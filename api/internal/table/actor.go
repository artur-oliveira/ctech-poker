// Package table drives one table's hand.Table from exactly one goroutine per
// instance — not because that instance owns write authority (it doesn't;
// ARCHITECTURE.md §2 makes DynamoDB's conditional writes the sole
// correctness mechanism), but because hand.Table has no internal lock, so
// two of this instance's own goroutines must still be serialized.
package table

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/equity"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/metrics"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

var timeNowFunc = time.Now

// Actor is the local serialization point for one table's hand.Table.
type Actor struct {
	id         string
	env        string
	store      *tablestore.Store
	trustCache bool // set once at construction — see New's doc comment
	broadcast  func(viewerID string, snap hand.Snapshot)

	cmds chan Command

	cached  *hand.Table // nil until first loaded; never trusted when !trustCache
	version int
	handID  string

	turnTimeout                  time.Duration
	disconnectGrace              time.Duration
	disconnectedSince            map[string]time.Time
	consecutiveDisconnectedHands map[string]int
	// activeConns counts live WS connections per playerID (a player can have
	// more than one open, e.g. two browser tabs). handleDisconnect only marks
	// a player disconnected once this hits zero, and handleConnect/
	// handleReconnect always clear that mark — so one tab dying while another
	// stays open never falsely flags the player as gone, and a disconnect
	// event racing a reconnect event from a different connection's goroutine
	// can never leave a live player stuck marked disconnected (the counter is
	// commutative regardless of which event the Run loop processes first).
	activeConns map[string]int
	// kickGrace bounds how long a disconnected player can occupy a seat
	// before being auto-removed (Leave, cashing them out same as a manual
	// exit). kickTimers holds one AfterFunc per currently-disconnected
	// player — unlike turnTimer/nextHandTimer there can be several at once,
	// one per seat.
	kickGrace             time.Duration
	kickTimers            map[string]*time.Timer
	playerNames           map[string]string
	turnTimer             *time.Timer
	turnDeadline          time.Time
	turnDeadlineFor       string
	turnDeadlineForStage  hand.Stage
	nextHandTimer         *time.Timer
	nextHandDeadline      time.Time
	nextHandArmedFor      string
	nextHandDelay         time.Duration
	lastBroadcastStage    hand.Stage
	runoutTimer           *time.Timer
	runoutTimerHandID     string
	runoutTimerStage      hand.Stage
	runoutStreetDelay     time.Duration
	escalationInterval    time.Duration
	escalationCfg         roomstore.BlindEscalation
	afkSweepTimer         *time.Timer
	afkSweepInterval      time.Duration
	done                  chan struct{}
	equityEnabled         atomic.Bool
	onHandComplete        func(string, hand.HandOutcome)
	completedHandNotified string
	onSeatsChanged        func(int)
	// onPlayerRemoved fires only for a system-initiated removal (AFK sweep,
	// disconnect kick timeout) — never for a player-requested LeaveCmd, which
	// the client already knows about and navigates away for itself. It lets
	// the gateway push an explicit "removed" message to that player's own
	// socket, so the client stops silently reconnecting into a seat it no
	// longer holds and instead redirects to the lobby (see tablews.go/
	// useTableRealtime.ts). stack/holdID are the same values a player-initiated
	// CashOut would have received on its Stack/HoldID channels — a system
	// removal never goes through buyin.Service.CashOut itself (it fires from
	// inside the Actor's own goroutine), so without forwarding them here the
	// removed player's chips are never credited back to any wallet and their
	// sessionlog entry is never closed (buyin.SettleSystemRemoval does both).
	onPlayerRemoved func(playerID, reason string, stack int64, holdID string)
}

// New returns an Actor for tableID. trustCache should be true only when the
// caller currently holds tableID's tablelease — it is read once here and
// never re-consulted; losing the lease later does not retroactively
// invalidate an in-flight Actor (ARCHITECTURE.md §2: the lease bounds
// latency, not correctness — a stale cache is always caught by
// CommitAction's version check regardless of trustCache).
func New(id string, store *tablestore.Store, trustCache bool, broadcast func(string, hand.Snapshot)) *Actor {
	a := &Actor{
		id: id, store: store, trustCache: trustCache, broadcast: broadcast, cmds: make(chan Command, 64),
		done:                         make(chan struct{}),
		turnTimeout:                  DefaultTurnTimeout,
		nextHandDelay:                NextHandDelay,
		runoutStreetDelay:            RunoutStreetDelay,
		disconnectGrace:              45 * time.Second,
		disconnectedSince:            make(map[string]time.Time),
		consecutiveDisconnectedHands: make(map[string]int),
		activeConns:                  make(map[string]int),
		kickGrace:                    5 * time.Minute,
		kickTimers:                   make(map[string]*time.Timer),
		playerNames:                  make(map[string]string),
		afkSweepInterval:             AFKSweepInterval,
	}
	a.equityEnabled.Store(true)
	a.armAFKSweepTimer()
	return a
}

// ErrActorStopped is returned by Dispatch when the actor has stopped serving
// (e.g. it lost its table lease and Run exited) and will never read the
// command. Callers re-resolve a live actor via the manager.
var ErrActorStopped = errors.New("table: actor stopped")

func (a *Actor) Dispatch(cmd Command) error {
	select {
	case a.cmds <- cmd:
		// Sent (channel is buffered). Wait for the reply, but bail if the
		// actor stops before Run reads/processes it — otherwise we'd block
		// forever on a dead actor.
		select {
		case err := <-cmd.reply():
			return err
		case <-a.done:
			return ErrActorStopped
		}
	case <-a.done:
		return ErrActorStopped
	}
}

// Done exposes the actor's stop channel so the manager can detect a dead actor
// (after a lease loss) and recreate a live one.
func (a *Actor) Done() <-chan struct{} { return a.done }

// IsAlive reports whether Run is still serving commands.
func (a *Actor) IsAlive() bool {
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *Actor) Run(ctx context.Context) {
	defer close(a.done)
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-a.cmds:
			err := a.handle(ctx, cmd)
			cmd.reply() <- err
		}
	}
}

func (a *Actor) handle(ctx context.Context, cmd Command) error {
	switch c := cmd.(type) {
	case ReadyCmd:
		return a.handleReady(ctx, c)
	case ActCmd:
		return a.handleAct(ctx, c)
	case ConnectCmd:
		return a.handleConnect(c)
	case DisconnectCmd:
		return a.handleDisconnect(c)
	case ReconnectCmd:
		return a.handleReconnect(ctx, c)
	case SitOutCmd:
		return a.handleSitOut(ctx, c)
	case ShowCardsCmd:
		return a.handleShowCards(ctx, c)
	case JoinCmd:
		return a.handleJoin(ctx, c)
	case LeaveCmd:
		return a.handleLeave(ctx, c)
	case PostBigBlindCmd:
		return a.handlePostBigBlind(ctx, c)
	case SnapshotCmd:
		return a.handleSnapshot(ctx, c)
	case SetNameCmd:
		return a.handleSetName(c)
	case turnTimeoutCmd:
		return a.handleTurnTimeout(ctx, c)
	case nextHandCmd:
		return a.handleNextHand(ctx, c)
	case runoutStepCmd:
		return a.handleRunoutStep(ctx, c)
	case kickTimeoutCmd:
		return a.handleKickTimeout(ctx, c)
	case afkSweepCmd:
		return a.handleAFKSweep(ctx, c)
	case escalateCmd:
		return a.handleEscalate(ctx)
	default:
		return nil
	}
}

func (a *Actor) handlePostBigBlind(ctx context.Context, c PostBigBlindCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		a.cached.MarkReadyToPost(c.PlayerID)
		return a.commit(ctx, "", nil)
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

// handleSnapshot loads the table (seeding on first touch) and returns the
// viewer-specific snapshot. Built inside Run so it never races broadcastAll's
// concurrent ViewFor calls over a.cached.
func (a *Actor) handleSnapshot(ctx context.Context, c SnapshotCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	c.Snapshot <- a.cached.ViewFor(c.PlayerID)
	return nil
}

// handleSetName caches the persisted display name the WS gateway looked up at
// connect time. It never touches tablestore — the name is process-local
// broadcast metadata, not authoritative table state.
func (a *Actor) handleSetName(c SetNameCmd) error {
	if c.Name == "" {
		return nil
	}
	a.playerNames[c.PlayerID] = c.Name
	a.broadcastAll()
	return nil
}

func (a *Actor) handleEscalate(ctx context.Context) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		a.cached.EscalateBlindsForActor(a.escalationCfg.Multiplier, a.escalationCfg.Max)
		return a.commit(ctx, "", nil)
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

// ensureLoaded reads current state from the store the first time this Actor
// is used, or whenever force is true (a prior commit proved the local cache
// stale). force never mutates a.trustCache — trustCache reflects only
// whether this Actor's own lease-affinity was granted at construction; a
// version conflict is evidence about staleness at this moment, not a
// permanent downgrade or upgrade of that grant.
func (a *Actor) ensureLoaded(ctx context.Context, force bool) error {
	if a.cached != nil && a.trustCache && !force {
		return nil
	}
	if a.store == nil {
		return nil
	}
	stored, err := a.store.LoadTable(ctx, a.id)
	if err != nil {
		return err
	}
	if stored == nil {
		return errors.New("table: no state seeded for this table yet")
	}
	a.cached = hand.NewTableFromState(stored.State)
	a.version = stored.Version
	a.handID = stored.HandID
	a.rearmTimersFromCache()
	return nil
}

// rearmTimersFromCache re-derives and re-arms the turn/runout/next-hand
// timers from whatever is now cached. All three are idempotent per
// (handID, stage) — see armTurnTimer/armRunoutTimer/armNextHandTimer — so
// calling this on every fresh load is safe and cheap. This is what lets a
// hand self-heal from the fleet's core failure mode: these timers are bare
// in-process time.AfterFunc calls with no persisted counterpart, so if the
// actor instance that armed one dies before it fires (node restart, lease
// handoff, autoscale-in), nothing else ever resumes it — the hand (most
// visibly an all-in runout) stays frozen forever even though the table's
// persisted state is fine. ensureLoaded runs on every command a
// non-lease-holding actor handles and on first construction everywhere, so
// the next bit of traffic this table sees from ANY node — even a bare
// ping's ReconnectCmd — re-derives these timers from durable state instead
// of trusting whatever the previous instance's memory (now gone) intended.
// Reveal grace is a live-broadcast pacing nicety, not a correctness
// requirement, so this always arms with zero grace — broadcastAll still
// applies the real grace on its own next call for anyone actually watching.
func (a *Actor) rearmTimersFromCache() {
	if a.cached == nil {
		return
	}
	stage := a.cached.Stage()
	a.armTurnTimer(a.cached.CurrentPlayerIDForActor(), stage, 0)
	a.armRunoutTimer(a.cached.IsAwaitingRunoutForActor(), stage)
	a.armNextHandTimer(stage == hand.Complete)
}

func (a *Actor) handleReady(ctx context.Context, c ReadyCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	a.markLastAction(c.PlayerID)
	apply := func() error { return a.applyReadyAndCommit(ctx, c) }
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) applyReadyAndCommit(ctx context.Context, c ReadyCmd) error {
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == c.PlayerID {
			p.Ready = c.Ready
		}
	}
	if c.Ready {
		a.cached.RequestReturnFromSitOut(c.PlayerID)
	} else {
		a.cached.SitOutForActor(c.PlayerID)
	}
	a.tryStartHand(ctx)
	return a.commit(ctx, "", nil)
}

// tryStartHand attempts to start a new hand if the table is between hands.
// "need at least 2 ready players" is not a caller error — the table just
// keeps waiting; StartHand's own error is swallowed here on purpose. Called
// from both a Ready toggle and a fresh Join, since a join alone can now bring
// the table to 2+ ready players (auto-ready on join). When the table is
// Complete, the just-finished hand's final state is snapshotted to the audit
// history table before StartHand() resets it to just players/chips.
func (a *Actor) tryStartHand(ctx context.Context) {
	if a.cached.Stage() == hand.WaitingForPlayers || a.cached.Stage() == hand.Complete {
		if a.cached.Stage() == hand.Complete {
			a.saveHandHistorySnapshot(ctx)
		}
		if err := a.cached.StartHand(); err == nil {
			a.handID = newHandID()
		}
	}
}

// saveHandHistorySnapshot persists the table's current (pre-reset) state to
// poker_table_state_history for audit purposes. Best-effort: this is an
// append-only audit copy, not the authoritative item, so a failure here never
// blocks the hand transition — it only emits a metric. A version-conflict
// retry re-running tryStartHand is harmless: once another instance has
// already advanced the stage past Complete, the reloaded cache no longer
// satisfies the Complete guard above, so the snapshot is not repeated.
func (a *Actor) saveHandHistorySnapshot(ctx context.Context) {
	if a.store == nil {
		return
	}
	if err := a.store.SaveTableStateHistory(ctx, a.id, timeNowFunc().Unix(), a.cached.ExportState()); err != nil {
		metrics.EmitTableMetric(a.env, "TableStateHistorySaveError", 1, map[string]string{"table_id": a.id})
	}
}

func (a *Actor) handleAct(ctx context.Context, c ActCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	a.markLastAction(c.PlayerID)
	start := timeNowFunc()
	_, err := a.applyActAndCommit(ctx, c)
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		// Two distinct reasons to reload and retry exactly once:
		//   - ErrVersionConflict: another instance committed first: definite
		//     staleness.
		//   - trustCache==true and any other error: this instance normally
		//     trusts a.cached between commits (ensureLoaded(ctx,false) is a
		//     no-op once cached is set) and only re-reads on a version
		//     conflict from ITS OWN write attempts. But ARCHITECTURE.md §2 /
		//     tablews.go's RegisterTableWS explicitly allow any instance to
		//     accept any table's connections directly, with no proxying to
		//     the lease holder — so another instance can commit actions this
		//     one never observes, and its next local Act() call (e.g. a
		//     turn-order check) can reject a genuinely legal action against
		//     stale data without ever reaching a conditional write to
		//     conflict on. Retrying here can't mask a truly invalid action:
		//     if it's genuinely invalid, re-running it against freshly
		//     loaded state reproduces the identical rejection.
		if errors.Is(err, tablestore.ErrVersionConflict) || a.trustCache {
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				return reloadErr
			}
			_, err = a.applyActAndCommit(ctx, c)
		}
	}
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		return err
	}
	metrics.EmitTableMetric(a.env, "ActionLatencyMs", float64(timeNowFunc().Sub(start).Milliseconds()), map[string]string{"table_id": a.id})
	a.broadcastAll()
	return nil
}

func (a *Actor) notifyHandComplete() {
	if a.cached == nil || a.cached.Stage() != hand.Complete || a.handID == "" || a.completedHandNotified == a.handID {
		return
	}
	if outcome := a.cached.LastOutcomeForActor(); outcome != nil {
		a.completedHandNotified = a.handID
		metrics.EmitTableMetric(a.env, "HandsCompleted", 1, map[string]string{"table_id": a.id})
		if a.onHandComplete != nil {
			a.onHandComplete(a.handID, *outcome)
		}
	}
}

// SetOnHandCompleteForActor installs the post-commit gamification hook.
// The actor invokes it at most once per local hand ID.
func (a *Actor) SetOnHandCompleteForActor(fn func(string, hand.HandOutcome)) { a.onHandComplete = fn }

// SetOnSeatsChangedForActor installs the post-commit occupancy write-through
// hook, invoked with the new occupied-seat count after every committed join
// or leave.
func (a *Actor) SetOnSeatsChangedForActor(fn func(int)) { a.onSeatsChanged = fn }

// SetOnPlayerRemovedForActor installs the system-removal notification hook —
// see onPlayerRemoved's doc comment.
func (a *Actor) SetOnPlayerRemovedForActor(fn func(playerID, reason string, stack int64, holdID string)) {
	a.onPlayerRemoved = fn
}

func (a *Actor) notifySeatsChanged() {
	if a.onSeatsChanged != nil && a.cached != nil {
		a.onSeatsChanged(len(a.cached.PlayersForActor()))
	}
}

// applyActAndCommit returns completed=true only when this Actor successfully
// committed the transition to Complete. A duplicate observed after another
// instance won the conditional write therefore cannot emit gamification a
// second time from this process.

func (a *Actor) applyActAndCommit(ctx context.Context, c ActCmd) (bool, error) {
	applied, err := a.cached.ActIdempotent(c.ActionID, c.PlayerID, c.Action, c.Amount)
	if err != nil {
		return false, err
	}
	if !applied {
		return false, nil
	}
	entry := tablestore.ActionLogEntry{PlayerID: c.PlayerID, ActionID: c.ActionID, Action: string(c.Action), Amount: c.Amount}
	if err := a.commit(ctx, c.ActionID, &entry); err != nil {
		return false, err
	}
	return a.cached.Stage() == hand.Complete, nil
}

func (a *Actor) commit(ctx context.Context, actionID string, entry *tablestore.ActionLogEntry) error {
	if a.store == nil {
		// Mirrors ensureLoaded's nil-store no-op: unit tests construct an
		// Actor with a nil store to exercise engine-level handler logic
		// without a real (DynamoDB Local) backing store. Never nil in
		// production — the manager always supplies a real *tablestore.Store.
		a.version++
		return nil
	}
	newState := a.cached.ExportState()
	if entry == nil {
		entry = &tablestore.ActionLogEntry{}
	}
	entry.TableID, entry.HandID, entry.Version = a.id, a.handID, a.version+1
	if err := a.store.CommitAction(ctx, a.id, a.handID, actionID, a.version, newState, *entry); err != nil {
		return err
	}
	a.version++
	return nil
}

// retryOnConflict runs apply once. If a version conflict is detected (another
// instance committed first), it reloads fresh state and applies once more.
// Handlers whose apply needs a return value beyond error (Act, Leave) keep
// their specialized retry; this covers the simple mutating handlers.
func (a *Actor) retryOnConflict(ctx context.Context, apply func() error) error {
	if err := apply(); err == nil {
		return nil
	} else if !errors.Is(err, tablestore.ErrVersionConflict) {
		return err
	}
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	return apply()
}

func (a *Actor) SetEnv(env string) { a.env = env }

// handleConnect fires exactly once per physical WS connection, right after
// the gateway registers it — unlike ReconnectCmd (fired on every inbound
// frame from every connection), this is the only place safe to count
// connections. A player with a second tab already open bumps this to 2; only
// the LAST connection to close (handleDisconnect dropping the count to 0)
// ever marks the player disconnected.
func (a *Actor) handleConnect(c ConnectCmd) error {
	a.activeConns[c.PlayerID]++
	a.clearDisconnectMark(c.PlayerID)
	return nil
}

func (a *Actor) handleDisconnect(c DisconnectCmd) error {
	if a.activeConns[c.PlayerID] > 0 {
		a.activeConns[c.PlayerID]--
	}
	if a.activeConns[c.PlayerID] > 0 {
		return nil // another connection (another tab) for this player is still live
	}
	metrics.EmitTableMetric(a.env, "Disconnects", 1, map[string]string{"table_id": a.id})
	a.disconnectedSince[c.PlayerID] = timeNowFunc()
	a.armKickTimer(c.PlayerID)
	a.broadcastAll()
	return nil
}

func (a *Actor) handleReconnect(ctx context.Context, c ReconnectCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	// This runs on EVERY inbound frame (tablews.go's read loop dispatches it
	// ahead of every message, including plain keepalive pings) so any traffic
	// clears a stale disconnect mark. Broadcasting unconditionally here means
	// every ping from every seat re-pushes the snapshot to the whole table —
	// with N seats pinging independently that's an O(N) snapshot flood with
	// no state change behind it. Only broadcast when this player was actually
	// marked disconnected.
	if !a.clearDisconnectMark(c.PlayerID) {
		return nil
	}
	a.broadcastAll()
	return nil
}

// clearDisconnectMark deletes playerID's stale disconnect bookkeeping and
// reports whether anything was actually cleared, so callers only broadcast
// (or otherwise react) when this genuinely changed something.
func (a *Actor) clearDisconnectMark(playerID string) bool {
	delete(a.consecutiveDisconnectedHands, playerID)
	if t, armed := a.kickTimers[playerID]; armed {
		t.Stop()
		delete(a.kickTimers, playerID)
	}
	if _, wasDisconnected := a.disconnectedSince[playerID]; !wasDisconnected {
		return false
	}
	delete(a.disconnectedSince, playerID)
	return true
}

// armKickTimer (re-)arms the auto-kick clock for a just-disconnected player.
// Only handleDisconnect calls this, exactly once per disconnect episode (the
// same invariant handleConnect/handleReconnect's clearDisconnectMark relies
// on), so unlike armTurnTimer there's no same-player no-op check needed.
func (a *Actor) armKickTimer(playerID string) {
	if t, armed := a.kickTimers[playerID]; armed {
		t.Stop()
	}
	a.kickTimers[playerID] = time.AfterFunc(a.kickGrace, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(kickTimeoutCmd{PlayerID: playerID, Reply: reply})
	})
}

// handleKickTimeout fires 5 minutes after a player disconnects and removes
// them from the table (same cash-out path as a manual Leave), freeing the
// seat for someone else. Stale if they reconnected since (clearDisconnectMark
// already stopped this timer, but a fire can still be in flight on the cmds
// channel when that happens) or already left.
func (a *Actor) handleKickTimeout(ctx context.Context, c kickTimeoutCmd) error {
	if _, disconnected := a.disconnectedSince[c.PlayerID]; !disconnected {
		return nil
	}
	delete(a.kickTimers, c.PlayerID)
	// ponytail: RemovePlayerForActor rejects removal while the player is
	// still dealt into a hand in progress (any state, including Folded — see
	// RemovePlayerForActor's doc comment). In practice that can't coincide
	// with 5 minutes disconnected — handleTurnTimeout's 45s/3-hand disconnect
	// grace already forces them to SittingOut long before this fires. If it
	// ever races anyway, skip silently; nothing to retry from here.
	stackCh := make(chan int64, 1)
	holdIDCh := make(chan string, 1)
	if err := a.handleLeave(ctx, LeaveCmd{PlayerID: c.PlayerID, Stack: stackCh, HoldID: holdIDCh}); err != nil {
		return err
	}
	if a.onPlayerRemoved != nil {
		a.onPlayerRemoved(c.PlayerID, "disconnected", <-stackCh, <-holdIDCh)
	}
	return nil
}

// markLastAction stamps playerID's LastActionAt with now — called only from
// handlers driven by a genuine inbound client command (Act, Ready, SitOut),
// never from a server-synthesized one (turn-timeout fold, disconnect
// auto-sit-out, the AFK sweep's own removal), so it reflects actual human
// presence rather than the system acting on the player's behalf. A no-op if
// the player isn't seated (defensive; every caller already resolved them).
func (a *Actor) markLastAction(playerID string) {
	for _, p := range a.cached.PlayersForActor() {
		if p.ID == playerID {
			p.LastActionAt = timeNowFunc().UnixMilli()
			return
		}
	}
}

// armAFKSweepTimer (re-)arms the periodic AFK sweep. Unlike every other timer
// in this file, it isn't tied to a game-state transition — it re-arms itself
// unconditionally after every fire (from handleAFKSweep), so it keeps
// checking every AFKSweepInterval for as long as the actor is alive. Once the
// actor dies (Run's ctx cancelled), the next fire's Dispatch hits a.done and
// returns ErrActorStopped without re-arming, so this doesn't outlive the
// actor.
func (a *Actor) armAFKSweepTimer() {
	a.afkSweepTimer = time.AfterFunc(a.afkSweepInterval, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(afkSweepCmd{Reply: reply})
	})
}

// handleAFKSweep exists because the per-turn timer only ever checks whoever
// the engine currently prompts (armTurnTimer/handleTurnTimeout) — a player
// who never becomes "current" for a long stretch, or one whose disconnect
// bookkeeping was lost to an actor restart (disconnectedSince/kickTimers are
// in-memory only; LastActionAt is persisted specifically so this check
// survives that), would otherwise occupy a seat forever. Reuses kickGrace as
// the staleness threshold — same "gone" semantics whether detected via
// transport disconnect or via activity silence, both flowing into the exact
// same removal path (handleLeave) as the disconnect-driven kick, including
// its existing has-a-live-hand guard (a stale player mid-hand is retried on
// the next sweep instead of being force-removed there).
func (a *Actor) handleAFKSweep(ctx context.Context, c afkSweepCmd) error {
	defer a.armAFKSweepTimer()
	if err := a.ensureLoaded(ctx, false); err != nil {
		return nil // transient load failure; next sweep retries
	}
	now := timeNowFunc()
	var stale []string
	for _, p := range a.cached.PlayersForActor() {
		if p.LastActionAt == 0 {
			continue
		}
		if now.Sub(time.UnixMilli(p.LastActionAt)) >= a.kickGrace {
			stale = append(stale, p.ID)
		}
	}
	for _, id := range stale {
		stackCh := make(chan int64, 1)
		holdIDCh := make(chan string, 1)
		if err := a.handleLeave(ctx, LeaveCmd{PlayerID: id, Stack: stackCh, HoldID: holdIDCh}); err != nil {
			continue // still dealt into a hand in progress; next sweep retries
		}
		if a.onPlayerRemoved != nil {
			a.onPlayerRemoved(id, "idle", <-stackCh, <-holdIDCh)
		}
	}
	return nil
}

func (a *Actor) handleSitOut(ctx context.Context, c SitOutCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	a.markLastAction(c.PlayerID)
	apply := func() error {
		a.cached.SitOutForActor(c.PlayerID)
		return a.commit(ctx, "", nil)
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleShowCards(ctx context.Context, c ShowCardsCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		if err := a.cached.RevealHoleCards(c.PlayerID); err != nil {
			return err
		}
		return a.commit(ctx, "", nil)
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

// handleTurnTimeout runs inside Run (dispatched by the universal per-turn
// timer) so it can safely read/write the actor's disconnect bookkeeping maps.
// It fires for whoever currently must act, regardless of connection state. A
// stale timer (the player already acted through the normal path before this
// fired) is a silent no-op — CurrentPlayerCanActForActor is false by then.
func (a *Actor) handleTurnTimeout(ctx context.Context, c turnTimeoutCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if !a.cached.CurrentPlayerCanActForActor(c.PlayerID) {
		return nil
	}
	if since, disconnected := a.disconnectedSince[c.PlayerID]; disconnected {
		a.consecutiveDisconnectedHands[c.PlayerID]++ // safe: runs in Run goroutine
		if timeNowFunc().Sub(since) >= a.disconnectGrace || a.consecutiveDisconnectedHands[c.PlayerID] >= 3 {
			// SitOutForActor folds the player out of the live round itself
			// (not just a bare state flip), so the round can actually
			// complete and, if this was the last decision pending, advance
			// the hand to Complete — broadcastAll's notifyHandComplete call
			// picks that up same as a normal Act would.
			a.cached.SitOutForActor(c.PlayerID)
			if err := a.commit(ctx, "", nil); err != nil {
				if !errors.Is(err, tablestore.ErrVersionConflict) {
					return err
				}
				// a.cached now holds an uncommitted SitOutForActor mutation
				// layered on stale state -- discard it by reloading fresh,
				// authoritative state instead of leaving this fabricated,
				// never-persisted table in memory for whatever this actor
				// does next (e.g. a later kick-timeout removal computing
				// handInProgress/dealtIntoCurrentHand off of it, which could
				// wrongly allow removing a player still dealt into the REAL
				// hand and leave a stale handOrder entry for a since-removed
				// player — runShowdown's playerByID lookup on that entry
				// would then panic).
				if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
					return reloadErr
				}
			}
			a.broadcastAll()
			return nil
		}
	}
	_, err := a.applyActAndCommit(ctx, ActCmd{
		PlayerID: c.PlayerID, ActionID: "turn-timeout-" + c.PlayerID, Action: betting.ActionFold, Amount: 0, Reply: c.Reply,
	})
	if errors.Is(err, tablestore.ErrVersionConflict) {
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
		// Re-check after the reload rather than blindly retrying the same
		// fold: whatever committed first may have already resolved this
		// exact turn (folded/removed the player through another path
		// entirely — e.g. the other server's own disconnect-kick), and
		// c.PlayerID's leftover roundIdx entry from a now-stale round must
		// not be trusted (see currentPlayerCanAct's doc comment).
		if !a.cached.CurrentPlayerCanActForActor(c.PlayerID) {
			a.broadcastAll()
			return nil
		}
		_, err = a.applyActAndCommit(ctx, ActCmd{
			PlayerID: c.PlayerID, ActionID: "turn-timeout-" + c.PlayerID, Action: betting.ActionFold, Amount: 0, Reply: c.Reply,
		})
	}
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleJoin(ctx context.Context, c JoinCmd) error {
	// Force a fresh reload here (unlike every other handler's ensureLoaded(ctx,
	// false)): a join is the highest-risk moment for a new viewer to be folded
	// into a stale in-memory cache — e.g. a lease-holding actor whose local
	// state is behind because it never observed another instance's commit (see
	// ensureLoaded's doc comment). A fresh read costs one extra DynamoDB read
	// per join and guarantees a joining player never sees leftover
	// LastOutcome/board/deadlines from before they existed.
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	apply := func() error { return a.applyJoinAndCommit(ctx, c) }
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.notifySeatsChanged()
	a.broadcastAll()
	return nil
}

func (a *Actor) applyJoinAndCommit(ctx context.Context, c JoinCmd) error {
	if c.MaxSeats > 0 && len(a.cached.PlayersForActor()) >= c.MaxSeats {
		return errors.New("table: no seats available")
	}
	p := &hand.Player{ID: c.PlayerID, Stack: c.Stack, HoldID: c.HoldID, LastActionAt: timeNowFunc().UnixMilli()}
	stage := a.cached.Stage()
	if stage != hand.WaitingForPlayers && stage != hand.Complete {
		if err := a.cached.AddMidHandJoiner(p); err != nil {
			return err
		}
	} else if err := a.cached.AddWaitingPlayer(p); err != nil {
		return err
	}
	a.tryStartHand(ctx)
	return a.commit(ctx, "", nil)
}

// handleLeave removes the player and reports their final stack on c.Stack —
// but only after the removal has actually committed, so a caller (buyin's
// CashOut) never credits a wallet for a leave that a version conflict or
// store error ultimately rolled back. The stack is recomputed from the
// freshly-reloaded a.cached on the retry (see applyLeaveAndCommit), never
// carried over from the stale pre-conflict attempt.
func (a *Actor) handleLeave(ctx context.Context, c LeaveCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	stack, holdID, err := a.applyLeaveAndCommit(ctx, c)
	if errors.Is(err, tablestore.ErrVersionConflict) {
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
		stack, holdID, err = a.applyLeaveAndCommit(ctx, c)
	}
	if err != nil {
		return err
	}
	delete(a.disconnectedSince, c.PlayerID)
	delete(a.consecutiveDisconnectedHands, c.PlayerID)
	delete(a.activeConns, c.PlayerID)
	if t, armed := a.kickTimers[c.PlayerID]; armed {
		t.Stop()
		delete(a.kickTimers, c.PlayerID)
	}
	if c.Stack != nil {
		c.Stack <- stack
	}
	if c.HoldID != nil {
		c.HoldID <- holdID
	}
	a.notifySeatsChanged()
	a.broadcastAll()
	return nil
}

func (a *Actor) applyLeaveAndCommit(ctx context.Context, c LeaveCmd) (int64, string, error) {
	stack, holdID, err := a.cached.RemovePlayerForActor(c.PlayerID)
	if err != nil {
		return 0, "", err
	}
	if err := a.commit(ctx, "", nil); err != nil {
		return 0, "", err
	}
	return stack, holdID, nil
}

// armTurnTimer (re-)arms the universal per-turn timer for current — the
// player who must act right now, connected or not (empty string when no
// decision is pending). Idempotent per (current, stage) pair: re-arming for
// the SAME current player on the SAME street is a no-op (does not restart
// their clock), matching "the timer counts down from when the turn actually
// began," not from every subsequent broadcast. stage is part of the key
// (not just current) because currentPlayerToAct always resolves to the
// earliest non-folded active player in table order at the start of a fresh
// betting round — the very same player ID can easily be "current" again on
// the next street (trivially so in heads-up), which must still count as a
// brand new turn. grace is added on top of the normal turnTimeout —
// broadcastAll passes RevealGrace for the first arm after a stage transition
// into Flop/Turn/River, and 0 otherwise.
func (a *Actor) armTurnTimer(current string, stage hand.Stage, grace time.Duration) {
	if current == a.turnDeadlineFor && stage == a.turnDeadlineForStage {
		return
	}
	if a.turnTimer != nil {
		a.turnTimer.Stop()
	}
	a.turnDeadlineFor = current
	a.turnDeadlineForStage = stage
	if current == "" {
		return
	}
	duration := a.turnTimeout + grace
	a.turnDeadline = timeNowFunc().Add(duration)
	// The timer only dispatches a command; all map/state mutations happen
	// inside Run (handleTurnTimeout), so there is no data race with the Run
	// goroutine.
	a.turnTimer = time.AfterFunc(duration, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(turnTimeoutCmd{PlayerID: current, Reply: reply})
	})
}

// isRevealStreet reports whether stage is one of the three streets whose
// arrival deals new board cards and therefore plays a reveal animation —
// PreFlop's hole cards use a different (faster) animation and are excluded.
func isRevealStreet(stage hand.Stage) bool {
	return stage == hand.Flop || stage == hand.Turn || stage == hand.River
}

// armNextHandTimer (re-)arms the 8s post-hand countdown when the table is
// Complete. Idempotent per handID: re-arming for the SAME hand does not
// restart the countdown (matches armTurnTimer's convention). complete is
// passed in by broadcastAll (already knows the current stage) so this stays
// a plain bool check, no engine dependency beyond what's already cached.
func (a *Actor) armNextHandTimer(complete bool) {
	if !complete {
		if a.nextHandTimer != nil {
			a.nextHandTimer.Stop()
		}
		a.nextHandArmedFor = ""
		return
	}
	if a.handID == a.nextHandArmedFor {
		return
	}
	if a.nextHandTimer != nil {
		a.nextHandTimer.Stop()
	}
	a.nextHandArmedFor = a.handID
	a.nextHandDeadline = timeNowFunc().Add(a.nextHandDelay)
	a.nextHandTimer = time.AfterFunc(a.nextHandDelay, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(nextHandCmd{Reply: reply})
	})
}

// handleNextHand attempts to start the next hand once the 5s post-hand
// countdown expires. A stale timer (a client already returned from sitting
// out and tryStartHand already ran, or the table isn't Complete anymore) is a
// silent no-op. tryStartHand itself already swallows "fewer than 2 ready
// players" — StartHand falls the table back to WaitingForPlayers in that
// case, so it doesn't stay stuck on Complete; a ReadyCmd(true) later starts
// the next hand normally.
func (a *Actor) handleNextHand(ctx context.Context, c nextHandCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if a.cached.Stage() != hand.Complete {
		return nil
	}
	a.tryStartHand(ctx)
	if err := a.commit(ctx, "", nil); err != nil {
		if !errors.Is(err, tablestore.ErrVersionConflict) {
			return err
		}
		// a.cached now holds an uncommitted tryStartHand mutation (possibly a
		// whole fabricated next hand, dealt from a stale player roster) layered
		// on stale state -- discard it by reloading fresh, authoritative state
		// instead of leaving it in memory for this actor's next command to
		// trust (see handleTurnTimeout's identical fix for the full story).
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
	}
	a.broadcastAll()
	return nil
}

// armRunoutTimer (re-)arms the paced all-in-runout timer while
// IsAwaitingRunoutForActor is true. Idempotent per (handID, stage) pair —
// re-arming for the same point in the runout does not restart the delay,
// matching armTurnTimer/armNextHandTimer's convention. stage is passed in by
// broadcastAll (already knows the current stage) so this stays a plain
// comparison, no extra engine call.
func (a *Actor) armRunoutTimer(awaiting bool, stage hand.Stage) {
	if !awaiting {
		if a.runoutTimer != nil {
			a.runoutTimer.Stop()
		}
		a.runoutTimerHandID = ""
		return
	}
	if a.handID == a.runoutTimerHandID && stage == a.runoutTimerStage {
		return
	}
	if a.runoutTimer != nil {
		a.runoutTimer.Stop()
	}
	a.runoutTimerHandID = a.handID
	a.runoutTimerStage = stage
	a.runoutTimer = time.AfterFunc(a.runoutStreetDelay, func() {
		reply := make(chan error, 1)
		_ = a.Dispatch(runoutStepCmd{Reply: reply})
	})
}

// handleRunoutStep fires runoutStreetDelay after the previous runout street
// was dealt and deals exactly the next one, pacing an all-in board reveal
// instead of showing the whole runout in a single broadcast. A stale fire
// (the awaited state no longer holds, e.g. this table already finished the
// runout through another path) is a silent no-op.
func (a *Actor) handleRunoutStep(ctx context.Context, c runoutStepCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if !a.cached.IsAwaitingRunoutForActor() {
		return nil
	}
	a.cached.AdvanceRunoutStreetForActor()
	if err := a.commit(ctx, "", nil); err != nil {
		if !errors.Is(err, tablestore.ErrVersionConflict) {
			return err
		}
		// Same discard-and-reload fix as handleTurnTimeout/handleNextHand:
		// a.cached currently holds an uncommitted AdvanceRunoutStreetForActor
		// mutation layered on stale state.
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) broadcastAll() {
	if a.broadcast == nil || a.cached == nil {
		return
	}
	stage := a.cached.Stage()
	current := a.cached.CurrentPlayerIDForActor()
	grace := time.Duration(0)
	if stage != a.lastBroadcastStage && isRevealStreet(stage) {
		grace = RevealGrace
	}
	a.armTurnTimer(current, stage, grace)
	a.armRunoutTimer(a.cached.IsAwaitingRunoutForActor(), stage)
	a.armNextHandTimer(stage == hand.Complete)
	a.lastBroadcastStage = stage
	doEquity := a.equityEnabled.Load() && equityStage(stage)
	for _, p := range a.cached.PlayersForActor() {
		snapshot := a.cached.ViewFor(p.ID)
		if current != "" && current == a.turnDeadlineFor {
			snapshot.ActionDeadlineUnixMs = a.turnDeadline.UnixMilli()
		}
		if stage == hand.Complete && a.handID == a.nextHandArmedFor {
			snapshot.NextHandUnixMs = a.nextHandDeadline.UnixMilli()
		}
		a.applyPlayerNames(snapshot.Seats)
		if doEquity {
			if hole, board, ok := a.cached.HoleAndBoardForActor(p.ID); ok {
				opponents := 0
				for _, seat := range snapshot.Seats {
					if seat.PlayerID != p.ID && (seat.State == "active" || seat.State == "all_in") {
						opponents++
					}
				}
				if opponents > 0 {
					// Offload equity from the Run goroutine: compute in a
					// goroutine over captured values and push a follow-up
					// state update when ready.
					go a.computeAndSendEquity(p.ID, snapshot, hole, board, opponents)
				}
			}
		}
		a.broadcast(p.ID, snapshot)
	}
	a.notifyHandComplete()
}

// applyPlayerNames fills in each seat's cached display name in place. Safe to
// mutate directly: seats is a freshly built slice from this ViewFor call,
// not yet shared with any other goroutine (unlike the equity copy below).
func (a *Actor) applyPlayerNames(seats []hand.SeatView) {
	for i := range seats {
		if name, ok := a.playerNames[seats[i].PlayerID]; ok {
			seats[i].Name = name
		}
	}
}

func equityStage(stage hand.Stage) bool {
	return stage == hand.PreFlop || stage == hand.Flop || stage == hand.Turn || stage == hand.River
}

// computeAndSendEquity runs off the Run goroutine. It never touches a.cached;
// it works on a copy of the captured snapshot so there is no race with Run or
// with the synchronous broadcast that already sent the same Snapshot. When
// ready it pushes a follow-up state update carrying the equity.
func (a *Actor) computeAndSendEquity(viewerID string, snapshot hand.Snapshot, hole [2]deck.Card, board []deck.Card, opponents int) {
	estimate, err := equity.Estimate(hole, board, nil, opponents, 500)
	if err != nil {
		return
	}
	// Copy Seats: the captured snapshot shares a backing array with the one the
	// synchronous broadcast already sent, so mutating it in place would race.
	out := snapshot
	out.Seats = make([]hand.SeatView, len(snapshot.Seats))
	copy(out.Seats, snapshot.Seats)
	for i := range out.Seats {
		if out.Seats[i].PlayerID == viewerID {
			out.Seats[i].Equity = &estimate
			break
		}
	}
	a.broadcast(viewerID, out)
}

func (a *Actor) SetEquityEnabledForActor(enabled bool) { a.equityEnabled.Store(enabled) }

// SetTurnTimeoutForActor sets the per-turn action deadline from the room's
// configured turn_timeout_seconds (0 handled by table.TurnTimeoutFor before
// this is called).
func (a *Actor) SetTurnTimeoutForActor(d time.Duration) {
	if d > 0 {
		a.turnTimeout = d
	}
}

// SetDisconnectGraceForActor overrides how long a disconnected player is
// given before their turn-timeout auto-fold escalates to auto-sit-out
// (handleTurnTimeout). Test-only knob — no room config exposes this today.
func (a *Actor) SetDisconnectGraceForActor(d time.Duration) {
	if d > 0 {
		a.disconnectGrace = d
	}
}

// SetKickGraceForActor overrides how long a disconnected player can occupy a
// seat before armKickTimer auto-removes them. Test-only knob — no room
// config exposes this today.
func (a *Actor) SetKickGraceForActor(d time.Duration) {
	if d > 0 {
		a.kickGrace = d
	}
}

func newHandID() string {
	return timeNowFunc().Format("20060102T150405.000000000")
}

// TableForTest exposes the cached hand.Table for integration-test assertions.
func (a *Actor) TableForTest() *hand.Table { return a.cached }

// SetCachedForTest seeds the cached hand.Table when running without a store.
func (a *Actor) SetCachedForTest(t *hand.Table) { a.cached = t }
