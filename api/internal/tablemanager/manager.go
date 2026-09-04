// Package tablemanager is the per-instance registry of live table Actors.
// There is no "owner" of a table under this revision (ARCHITECTURE.md §2):
// any instance may create an Actor for any table at any time. tablelease is
// consulted only to decide whether that Actor may trust its own in-memory
// cache between commits — never to gate whether it may be created at all.
package tablemanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"gopkg.aoctech.app/poker/api/internal/engine/equity"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type Actor = table.Actor

const (
	defaultActorIdleTimeout  = 5 * time.Minute
	defaultIdleCheckInterval = time.Minute
)

// ErrTableArchived means tableID was archived by cmd/tablecleanup for
// inactivity (StoredTable.Archived, api/internal/tablestore) — its seated
// players were already refunded, and no new actor may be created for it.
// buyin.Service wraps manager errors with %w so callers can errors.Is
// against this directly.
var ErrTableArchived = errors.New("tablemanager: table archived")

type Manager struct {
	env                    string
	leases                 *tablelease.Service
	store                  *tablestore.Store
	broadcast              func(tableID, viewerID string, snap hand.Snapshot)
	onHandComplete         func(tableID, handID string, outcome hand.HandOutcome, names map[string]string)
	onHandUpdated          func(tableID, handID string, outcome hand.HandOutcome, names map[string]string)
	onSeatsChanged         func(tableID string, seatsTaken int)
	onPlayerRemoved        func(tableID, playerID, reason, settlementNonce string, stack int64, holdID string)
	autoRebuySweep         func(tableID, handID string, outcome hand.HandOutcome)
	tableStreak            func(tableID, handID string, outcome hand.HandOutcome) map[string]int
	streakStore            table.StreakStore
	handHooks              table.HandHookClaimer
	connStore              table.ConnStore
	changeNotifier         table.ChangeNotifier
	systemSettlementIntent func(ctx context.Context, tableID, playerID, reason, settlementNonce string, stack int64, holdID string) (types.TransactWriteItem, error)
	roomLoader             func(tableID string) (*roomstore.Room, bool, error)
	reactionOwnership      func(ctx context.Context, playerID, reactionID string) (bool, error)
	reactionMarkUsed       func(ctx context.Context, playerID, reactionID string) (*types.TransactWriteItem, error)

	mu       sync.Mutex
	actors   map[string]*Actor
	releases map[string]func()
	cancels  map[string]context.CancelFunc
	locks    map[string]*tableLock

	actorIdleTimeout  time.Duration
	idleCheckInterval time.Duration

	// preRegisterHook, when set, runs once per actual actor creation, just
	// before the fresh actor is inserted into m.actors — after every
	// create-path network call and while tableID's per-table lock is still
	// held. It exists purely so tests can inject latency/instrumentation
	// into the create path without a real DynamoDB/Valkey dependency;
	// production code never sets it.
	preRegisterHook func(tableID string)

	// drainMu/draining/drainDone make DrainAndRelease idempotent (#33): the
	// SIGTERM/OnStop path and the proactive spot-termination poller
	// (internal/app's pollSpotTermination) can now both call it — sequentially
	// or concurrently, on this instance's single shared Manager — without
	// either one re-releasing a lease the other already released. The first
	// caller to observe draining==false does the real work and closes
	// drainDone when finished; every other caller (already in progress, or
	// arriving after this instance has already fully drained) just waits on
	// that same channel instead of touching m.actors again.
	drainMu   sync.Mutex
	draining  bool
	drainDone chan struct{}
}

// tableLock is a per-tableID mutex, refcounted so Manager.locks never
// retains an entry for a table nobody is currently creating/recreating an
// actor for. Guarded by Manager.mu: only the refs field and the locks map
// entry itself are protected by mu; mu is a bare struct{}-style mutex is
// held only long enough to look up/insert/evict the entry (see
// acquireTableLock/releaseTableLock), never across the network calls the
// per-table mu below serializes.
type tableLock struct {
	mu   sync.Mutex
	refs int
}

// acquireTableLock returns tableID's per-table lock, held, incrementing its
// refcount so a concurrent releaseTableLock can't evict the entry out from
// under a waiter. Only Manager.mu (briefly, for the map lookup/insert) is
// held here — never across the caller's subsequent network calls.
func (m *Manager) acquireTableLock(tableID string) *tableLock {
	m.mu.Lock()
	l, ok := m.locks[tableID]
	if !ok {
		l = &tableLock{}
		m.locks[tableID] = l
	}
	l.refs++
	m.mu.Unlock()

	l.mu.Lock()
	return l
}

// releaseTableLock unlocks l and, if no other caller is waiting on it,
// evicts it from m.locks so the registry doesn't grow unboundedly across
// this process's lifetime as distinct table IDs come and go.
func (m *Manager) releaseTableLock(tableID string, l *tableLock) {
	l.mu.Unlock()

	m.mu.Lock()
	l.refs--
	if l.refs == 0 {
		if cur, ok := m.locks[tableID]; ok && cur == l {
			delete(m.locks, tableID)
		}
	}
	m.mu.Unlock()
}

// lookupAliveActor returns the live actor cached for tableID, if any,
// dropping it first if it has stopped (lease lost) — a stale actor is never
// handed back so callers never dispatch to a dead one (T1). Guarded only by
// the short-lived m.mu, never held across a network call.
func (m *Manager) lookupAliveActor(tableID string) (*Actor, bool) {
	var release func()
	m.mu.Lock()
	if a, ok := m.actors[tableID]; ok {
		if a.IsAlive() {
			m.mu.Unlock()
			return a, true
		}
		delete(m.actors, tableID)
		delete(m.cancels, tableID)
		release = m.releases[tableID]
		delete(m.releases, tableID)
	}
	m.mu.Unlock()
	if release != nil {
		release()
	}
	equity.EvictTable(tableID)
	return nil, false
}

func NewManager(leases *tablelease.Service, store *tablestore.Store, broadcast func(string, string, hand.Snapshot), roomLoader func(string) (*roomstore.Room, bool, error), completion ...func(string, string, hand.HandOutcome, map[string]string)) *Manager {
	var onHandComplete func(string, string, hand.HandOutcome, map[string]string)
	if len(completion) > 0 {
		onHandComplete = completion[0]
	}
	return &Manager{
		leases:            leases,
		store:             store,
		broadcast:         broadcast,
		onHandComplete:    onHandComplete,
		roomLoader:        roomLoader,
		actors:            make(map[string]*Actor),
		releases:          make(map[string]func()),
		cancels:           make(map[string]context.CancelFunc),
		locks:             make(map[string]*tableLock),
		actorIdleTimeout:  defaultActorIdleTimeout,
		idleCheckInterval: defaultIdleCheckInterval,
	}
}

func (m *Manager) SetEnv(env string) { m.env = env }

func (m *Manager) SetOnHandUpdated(fn func(tableID, handID string, outcome hand.HandOutcome, names map[string]string)) {
	m.onHandUpdated = fn
}

// SetOnSeatsChanged installs the occupancy write-through hook, invoked with
// (tableID, seatsTaken) after every table actor's committed join/leave, for
// every actor this manager creates (including ones created before this call).
func (m *Manager) SetOnSeatsChanged(fn func(tableID string, seatsTaken int)) { m.onSeatsChanged = fn }

// SetOnPlayerRemoved installs the system-removal notification hook (AFK
// sweep / disconnect kick timeout only — never a player-requested leave),
// invoked with (tableID, playerID, reason, stack, holdID) for every actor
// this manager creates, including ones created before this call. stack/holdID
// are what buyin.SettleSystemRemoval needs to credit the removed player's
// wallet and close their sessionlog entry. settlementNonce makes this
// removal's poker_pending_cashouts key unique — SettleSystemRemoval must pass
// the same value so its credit resolves the row the seat removal co-wrote.
func (m *Manager) SetOnPlayerRemoved(fn func(tableID, playerID, reason, settlementNonce string, stack int64, holdID string)) {
	m.onPlayerRemoved = fn
}

// SetOnAutoRebuySweep installs the post-hand auto-rebuy hook, invoked with
// (tableID, handID, outcome) right after the achievements/history
// onHandComplete hook, for every actor this manager creates (including ones
// created before this call). The callback fires synchronously on the table
// actor's own goroutine — same as onHandComplete — so it must never call
// anything that dispatches back into the actor without detaching first (see
// app.wireAutoRebuyHook).
func (m *Manager) SetOnAutoRebuySweep(fn func(tableID, handID string, outcome hand.HandOutcome)) {
	m.autoRebuySweep = fn
}

// SetOnTableStreak installs the post-hand hot/cold streak hook. It returns
// the freshly persisted per-player streak values (achievements.Service is
// the durable writer), which the wrapper below hands to SetStreaksForActor —
// a direct call, not a Dispatch, since this too fires synchronously on the
// actor's own goroutine (same constraint as autoRebuySweep above).
func (m *Manager) SetOnTableStreak(fn func(tableID, handID string, outcome hand.HandOutcome) map[string]int) {
	m.tableStreak = fn
}

// SetStreakStore gives every actor this instance creates the shared badge
// store (internal/tablestreak). Without it the badge falls back to this
// process's own tally, which is exactly what made two instances publish two
// different numbers for one seat.
func (m *Manager) SetStreakStore(s table.StreakStore) {
	m.streakStore = s
}

// SetHandHookClaimer gives every actor this instance creates the shared
// once-per-hand claim for post-hand hooks (internal/handhook). Without it the
// only guard is each process's own completedHandNotified, which is what let a
// second instance re-credit a hand's achievements and streak.
func (m *Manager) SetHandHookClaimer(c table.HandHookClaimer) {
	m.handHooks = c
}

// SetConnStore gives every actor this instance creates the fleet-wide
// connection set (internal/tableconn) behind the seat's connection dot.
// Without it the dot reflects only sockets terminating on this process.
func (m *Manager) SetConnStore(s table.ConnStore) {
	m.connStore = s
}

// SetChangeNotifier gives every actor this instance creates the shared
// cross-process commit signal (internal/tablenotify). Without it, an actor
// only reloads and re-arms its real enforcement timers on its own next
// unrelated trigger — see ChangeNotifier's doc comment.
func (m *Manager) SetChangeNotifier(n table.ChangeNotifier) {
	m.changeNotifier = n
}

// ChangeListener is the subscribe side of internal/tablenotify — kept as a
// narrow interface here (rather than importing that package directly) so
// tablemanager depends only on the shape it actually calls.
type ChangeListener interface {
	Listen(ctx context.Context, onChange func(tableID string))
}

// ListenForExternalChanges blocks, dispatching table.ExternalChangeCmd to
// whichever local Actor is currently running each table the notifier
// reports as changed, until ctx is cancelled. A table with no local Actor
// (nothing here is currently serving it) is silently ignored — there is
// nothing local to refresh. Call once per process, alongside the notifier's
// own construction; see internal/app's fx wiring.
func (m *Manager) ListenForExternalChanges(ctx context.Context, notifier ChangeListener) {
	if notifier == nil {
		return
	}
	notifier.Listen(ctx, func(tableID string) {
		m.mu.Lock()
		actor := m.actors[tableID]
		m.mu.Unlock()
		if actor == nil {
			return
		}
		reply := make(chan error, 1)
		// TEMPORARY (2026-09-04 turn-ring staleness investigation): pins when
		// this process's pubsub callback fired, so a slow Dispatch (actor
		// command channel backpressure) shows up as a gap against
		// handleExternalChange's own "start" log. Remove once resolved.
		slog.Info("table external change received", "table_id", tableID, "received_unix_ms", time.Now().UnixMilli())
		if err := actor.Dispatch(table.ExternalChangeCmd{Reply: reply}); err != nil {
			slog.Warn("table external change dispatch failed", "table_id", tableID, "err", err)
		}
	})
}

// GetOrCreateActor returns this instance's Actor for tableID, seeding the
// table's very first DynamoDB state if it has never been played (seed is
// only invoked then). A failed best-effort lease acquire never blocks this —
// it only means the resulting Actor re-reads DynamoDB before every command
// instead of trusting its cache between commits.
//
// The create path is guarded per-tableID (not by a single process-global
// mutex) so two concurrent callers for the same tableID can never end up
// with two live Actors (T7), while callers for different tableIDs never
// block on each other's DynamoDB/Valkey round trips — the whole point of
// #31. m.mu itself is only ever held long enough to read or mutate the
// actors/cancels/releases/locks maps; it is never held across a network
// call. If the cached actor has stopped (it lost its lease and Run exited),
// it is dropped and a fresh one is created in its place so callers never
// dispatch to a dead actor (T1).
//
// This lock is deliberately process-local, not a Valkey/distributed lock:
// two *different* instances each running an Actor for the same table is an
// explicitly supported state (see the package doc + ARCHITECTURE.md §2).
// Cross-instance correctness is enforced by DynamoDB conditional writes
// (`version` + ConditionExpression in tablestore.CommitAction) — the second
// writer fails its condition and reloads — never by there being exactly one
// Actor fleet-wide. tablelease already keeps cross-instance duplication rare
// (cache-affinity only). The one race a lock here must prevent is two
// goroutines *in this process* creating two Actor goroutines for the same
// table, which would race on the same in-memory cache; that is purely a
// local concern and a sync.Mutex is the right tool for it.
// IsArchived reports whether tableID's stored table has been archived by
// cmd/tablecleanup. It is a single DynamoDB read: no actor goroutine, no
// timers, no lease acquisition and no per-table lock — for callers that only
// need to know whether a table they are NOT joining is still alive (issue
// #57). A table with no stored state at all is not archived (it is simply
// cold and would be seeded on first join).
func (m *Manager) IsArchived(ctx context.Context, tableID string) (bool, error) {
	if m == nil || m.store == nil {
		return false, nil
	}
	stored, err := m.store.LoadTable(ctx, tableID)
	if err != nil {
		return false, fmt.Errorf("tablemanager: load table: %w", err)
	}
	return stored != nil && stored.Archived, nil
}

func (m *Manager) GetOrCreateActor(ctx context.Context, tableID string, seed func() *hand.Table, onCreated ...func(*Actor)) (*Actor, error) {
	if a, ok := m.lookupAliveActor(tableID); ok {
		return a, nil
	}

	// Past this point tableID is retained by state that outlives the caller:
	// the Actor's own id, this registry's map key, the broadcast/metric/lease
	// closures below. Callers arrive from Fiber handlers where c.Params gives
	// back a string pointing into fasthttp's recycled request buffer, so the
	// bytes mutate as soon as that request is done. GET /rooms/:id/seated
	// created the actor for a live table and its id later read
	// "/01KYNA8GAXYP5TF8K71XB1DMT", making every LoadTable miss and every
	// action fail with "table: no state seeded for this table yet".
	tableID = strings.Clone(tableID)

	// Everything from here down — the DynamoDB/Valkey round trips and the
	// actor construction they gate — runs under tableID's own lock, never
	// under m.mu, so an unrelated table's cold start never waits behind this
	// one's. The lock still guarantees at most one Actor is ever created for
	// this tableID (T7): a second concurrent caller blocks here, then finds
	// the freshly-registered actor via the re-check below instead of racing
	// this one's creation.
	tl := m.acquireTableLock(tableID)
	defer m.releaseTableLock(tableID, tl)

	if a, ok := m.lookupAliveActor(tableID); ok {
		return a, nil
	}

	if m.store != nil {
		existing, err := m.store.LoadTable(ctx, tableID)
		if err != nil {
			return nil, fmt.Errorf("tablemanager: load table: %w", err)
		}
		if existing != nil && existing.Archived {
			return nil, ErrTableArchived
		}
		if existing == nil {
			if err := m.store.SeedTable(ctx, tableID, seed().ExportState()); err != nil {
				return nil, fmt.Errorf("tablemanager: seed table: %w", err)
			}
		}
	}

	trustCache := false
	var release func()
	if m.leases != nil {
		if rel, ok, err := m.leases.Acquire(ctx, tableID); err == nil && ok {
			trustCache = true
			release = rel
		}
	}

	actor := table.New(tableID, m.store, trustCache, m.broadcastFor(tableID))
	if m.streakStore != nil {
		actor.SetStreakStoreForActor(m.streakStore)
	}
	if m.handHooks != nil {
		actor.SetHandHookClaimerForActor(m.handHooks)
	}
	if m.connStore != nil {
		actor.SetConnStoreForActor(m.connStore)
	}
	if m.changeNotifier != nil {
		actor.SetChangeNotifierForActor(m.changeNotifier)
	}
	if m.store == nil && seed != nil {
		actor.SetCachedForTest(seed())
	}
	actor.SetEnv(m.env)
	actor.SetOnHandCompleteForActor(func(handID string, outcome hand.HandOutcome, names map[string]string) {
		if m.onHandComplete != nil {
			m.onHandComplete(tableID, handID, outcome, names)
		}
		if m.autoRebuySweep != nil {
			m.autoRebuySweep(tableID, handID, outcome)
		}
		if m.tableStreak != nil {
			actor.SetStreaksForActor(m.tableStreak(tableID, handID, outcome))
		}
	})
	actor.SetOnHandUpdatedForActor(func(handID string, outcome hand.HandOutcome, names map[string]string) {
		if m.onHandUpdated != nil {
			m.onHandUpdated(tableID, handID, outcome, names)
		}
	})
	actor.SetOnSeatsChangedForActor(func(seatsTaken int) {
		if m.onSeatsChanged != nil {
			m.onSeatsChanged(tableID, seatsTaken)
		}
	})
	actor.SetOnPlayerRemovedForActor(func(playerID, reason, settlementNonce string, stack int64, holdID string) {
		if m.onPlayerRemoved != nil {
			m.onPlayerRemoved(tableID, playerID, reason, settlementNonce, stack, holdID)
		}
	})
	actor.SetSystemSettlementIntentForActor(func(ctx context.Context, playerID, reason, settlementNonce string, stack int64, holdID string) (types.TransactWriteItem, error) {
		if m.systemSettlementIntent == nil {
			return types.TransactWriteItem{}, errors.New("tablemanager: system settlement intent builder unavailable")
		}
		return m.systemSettlementIntent(ctx, tableID, playerID, reason, settlementNonce, stack, holdID)
	})
	actor.SetReactionOwnershipForActor(func(ctx context.Context, playerID, reactionID string) (bool, error) {
		if m.reactionOwnership == nil {
			return false, errors.New("tablemanager: reaction ownership check unavailable")
		}
		return m.reactionOwnership(ctx, playerID, reactionID)
	})
	actor.SetReactionMarkUsedForActor(func(ctx context.Context, playerID, reactionID string) (*types.TransactWriteItem, error) {
		if m.reactionMarkUsed == nil {
			return nil, errors.New("tablemanager: reaction usage intent builder unavailable")
		}
		return m.reactionMarkUsed(ctx, playerID, reactionID)
	})
	runCtx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	m.cancels[tableID] = cancel
	if release != nil {
		m.releases[tableID] = release
	}
	m.mu.Unlock()

	if trustCache {
		m.leases.StartHeartbeat(runCtx, tableID, func() {
			cancel()
			<-actor.Done()
			m.removeActor(tableID, actor)
		})
	}
	// tablelease is cache affinity only. A lease must not pin an otherwise
	// unused actor, its timers, and its equity entries in memory forever.
	go m.evictActorWhenIdle(runCtx, tableID, actor, cancel)
	go actor.Run(runCtx)

	if m.preRegisterHook != nil {
		m.preRegisterHook(tableID)
	}

	m.mu.Lock()
	m.actors[tableID] = actor
	m.mu.Unlock()

	// Re-arm blind escalation and the per-turn action timeout from the room's
	// authoritative config so both survive instance/lease moves (T6). Any
	// instance creating the actor loads the room once and applies both.
	if m.roomLoader != nil {
		if room, ok, err := m.roomLoader(tableID); err == nil && ok && room != nil {
			if room.BlindEscalation != nil {
				actor.StartEscalation(*room.BlindEscalation)
			}
			actor.SetTurnTimeoutForActor(table.TurnTimeoutFor(room.TurnTimeoutSeconds))
			actor.SetRunItTwiceEnabledForActor(room.RunItTwiceEnabled)
		}
	}

	for _, hook := range onCreated {
		hook(actor)
	}
	return actor, nil
}

// removeActor drops a (dead) actor from the registry. Safe to call from the
// lease-loss callback (runs off the Run goroutine) — it takes m.mu.
func (m *Manager) removeActor(tableID string, expected *Actor) {
	var release func()
	removed := false
	m.mu.Lock()
	if a, ok := m.actors[tableID]; ok && a == expected && !a.IsAlive() {
		delete(m.actors, tableID)
		delete(m.cancels, tableID)
		release = m.releases[tableID]
		delete(m.releases, tableID)
		removed = true
	}
	m.mu.Unlock()
	if release != nil {
		release()
	}
	if removed {
		equity.EvictTable(tableID)
	}
}

// evictActorWhenIdle bounds the per-instance registry whether or not this
// actor acquired the cache-affinity lease. A continuous zero-connection
// window is required: seeing any live socket resets the clock, so a busy table
// is never evicted between polling ticks.
func (m *Manager) evictActorWhenIdle(ctx context.Context, tableID string, actor *Actor, cancel context.CancelFunc) {
	ticker := time.NewTicker(m.idleCheckInterval)
	defer ticker.Stop()
	idleSince := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if actor.ActiveConnCount() > 0 {
				idleSince = time.Time{}
				continue
			}
			if idleSince.IsZero() {
				idleSince = now
				continue
			}
			if now.Sub(idleSince) < m.actorIdleTimeout {
				continue
			}
			cancel()
			<-actor.Done()
			m.removeActor(tableID, actor)
			return
		}
	}
}

func (m *Manager) broadcastFor(tableID string) func(string, hand.Snapshot) {
	return func(viewerID string, snap hand.Snapshot) {
		if m.broadcast != nil {
			m.broadcast(tableID, viewerID, snap)
		}
	}
}

// Release releases tableID's lease and removes the actor from local registry.
func (m *Manager) Release(tableID string) {
	m.mu.Lock()
	actor := m.actors[tableID]
	delete(m.actors, tableID)
	cancel := m.cancels[tableID]
	delete(m.cancels, tableID)
	rel, hasRel := m.releases[tableID]
	delete(m.releases, tableID)
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if actor != nil {
		<-actor.Done()
	}
	if hasRel && rel != nil {
		rel()
	}
	equity.EvictTable(tableID)
}

// LiveActorCount reports this process's actor count for the load harness. It
// is operational telemetry, not a fleet-wide ownership assertion.
func (m *Manager) LiveActorCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.actors)
}

// DrainAndRelease releases every table lease held by this instance on
// graceful shutdown and waits for all table actor goroutines to finish
// processing in-flight operations.
//
// It is idempotent and safe to call concurrently or repeatedly for the same
// Manager (#33): only the first call actually walks m.actors and releases
// anything; every other call — whether it arrives while the first is still
// running or after this instance has already fully drained — blocks only
// until that first call's work (or ctx) is done, then returns without
// touching a single lease itself. This is what lets both the OnStop
// SIGTERM handler and the proactive EC2 spot-termination-notice poller
// (internal/app's pollSpotTermination) call this same path without either
// one double-releasing a seat/lease the other already tore down.
func (m *Manager) DrainAndRelease(ctx context.Context) {
	m.drainMu.Lock()
	if m.draining {
		done := m.drainDone
		m.drainMu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
		}
		return
	}
	m.draining = true
	done := make(chan struct{})
	m.drainDone = done
	m.drainMu.Unlock()
	defer close(done)

	m.mu.Lock()
	ids := make([]string, 0, len(m.actors))
	actors := make([]*table.Actor, 0, len(m.actors))
	for id, a := range m.actors {
		ids = append(ids, id)
		actors = append(actors, a)
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.Release(id)
	}

	for _, a := range actors {
		select {
		case <-a.Done():
		case <-ctx.Done():
			return
		}
	}
}

// SetSystemSettlementIntent installs the pre-commit builder used by AFK and
// disconnect removals. A system removal fails closed if this dependency is
// absent or cannot build its durable recovery obligation.
func (m *Manager) SetSystemSettlementIntent(fn func(ctx context.Context, tableID, playerID, reason, settlementNonce string, stack int64, holdID string) (types.TransactWriteItem, error)) {
	m.systemSettlementIntent = fn
}

func (m *Manager) SetReactionOwnership(fn func(ctx context.Context, playerID, reactionID string) (bool, error)) {
	m.reactionOwnership = fn
}

func (m *Manager) SetReactionMarkUsed(fn func(ctx context.Context, playerID, reactionID string) (*types.TransactWriteItem, error)) {
	m.reactionMarkUsed = fn
}
