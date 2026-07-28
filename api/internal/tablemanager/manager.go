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
	"strings"
	"sync"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/metrics"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type Actor = table.Actor

const (
	defaultLeaseLessIdleTimeout = 5 * time.Minute
	defaultIdleCheckInterval    = time.Minute
)

// ErrTableArchived means tableID was archived by cmd/tablecleanup for
// inactivity (StoredTable.Archived, api/internal/tablestore) — its seated
// players were already refunded, and no new actor may be created for it.
// buyin.Service wraps manager errors with %w so callers can errors.Is
// against this directly.
var ErrTableArchived = errors.New("tablemanager: table archived")

type Manager struct {
	env             string
	leases          *tablelease.Service
	store           *tablestore.Store
	broadcast       func(tableID, viewerID string, snap hand.Snapshot)
	onHandComplete  func(tableID, handID string, outcome hand.HandOutcome, names map[string]string)
	onHandUpdated   func(tableID, handID string, outcome hand.HandOutcome, names map[string]string)
	onSeatsChanged  func(tableID string, seatsTaken int)
	onPlayerRemoved func(tableID, playerID, reason string, stack int64, holdID string)
	roomLoader      func(tableID string) (*roomstore.Room, bool, error)

	mu       sync.Mutex
	actors   map[string]*Actor
	releases map[string]func()
	cancels  map[string]context.CancelFunc

	leaseLessIdleTimeout time.Duration
	idleCheckInterval    time.Duration
}

func NewManager(leases *tablelease.Service, store *tablestore.Store, broadcast func(string, string, hand.Snapshot), roomLoader func(string) (*roomstore.Room, bool, error), completion ...func(string, string, hand.HandOutcome, map[string]string)) *Manager {
	var onHandComplete func(string, string, hand.HandOutcome, map[string]string)
	if len(completion) > 0 {
		onHandComplete = completion[0]
	}
	return &Manager{
		leases:               leases,
		store:                store,
		broadcast:            broadcast,
		onHandComplete:       onHandComplete,
		roomLoader:           roomLoader,
		actors:               make(map[string]*Actor),
		releases:             make(map[string]func()),
		cancels:              make(map[string]context.CancelFunc),
		leaseLessIdleTimeout: defaultLeaseLessIdleTimeout,
		idleCheckInterval:    defaultIdleCheckInterval,
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
// wallet and close their sessionlog entry.
func (m *Manager) SetOnPlayerRemoved(fn func(tableID, playerID, reason string, stack int64, holdID string)) {
	m.onPlayerRemoved = fn
}

// GetOrCreateActor returns this instance's Actor for tableID, seeding the
// table's very first DynamoDB state if it has never been played (seed is
// only invoked then). A failed best-effort lease acquire never blocks this —
// it only means the resulting Actor re-reads DynamoDB before every command
// instead of trusting its cache between commits.
//
// The whole create path is guarded by m.mu so two concurrent callers for the
// same tableID can never end up with two live Actors (T7). If the cached
// actor has stopped (it lost its lease and Run exited), it is dropped and a
// fresh one is created in its place so callers never dispatch to a dead actor
// (T1).
func (m *Manager) GetOrCreateActor(ctx context.Context, tableID string, seed func() *hand.Table, onCreated ...func(*Actor)) (*Actor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if a, ok := m.actors[tableID]; ok {
		if a.IsAlive() {
			return a, nil
		}
		// Stale/dead actor (lease lost) — drop it and recreate below.
		delete(m.actors, tableID)
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
	if m.leases != nil {
		if rel, ok, err := m.leases.Acquire(ctx, tableID); err == nil && ok {
			trustCache = true
			m.releases[tableID] = rel
		}
	}

	actor := table.New(tableID, m.store, trustCache, m.broadcastFor(tableID))
	if m.store == nil && seed != nil {
		actor.SetCachedForTest(seed())
	}
	actor.SetEnv(m.env)
	actor.SetOnHandCompleteForActor(func(handID string, outcome hand.HandOutcome, names map[string]string) {
		if m.onHandComplete != nil {
			m.onHandComplete(tableID, handID, outcome, names)
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
	actor.SetOnPlayerRemovedForActor(func(playerID, reason string, stack int64, holdID string) {
		if m.onPlayerRemoved != nil {
			m.onPlayerRemoved(tableID, playerID, reason, stack, holdID)
		}
	})
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancels[tableID] = cancel
	if trustCache {
		m.leases.StartHeartbeat(runCtx, tableID, func() {
			metrics.EmitTableMetric(m.env, "LeaseFailovers", 1, map[string]string{"table_id": tableID})
			cancel()
			<-actor.Done()
			m.removeActor(tableID, actor)
		})
	} else {
		go m.evictLeaseLessActorWhenIdle(runCtx, tableID, actor, cancel)
	}
	go actor.Run(runCtx)

	m.actors[tableID] = actor

	// Re-arm blind escalation and the per-turn action timeout from the room's
	// authoritative config so both survive instance/lease moves (T6). Any
	// instance creating the actor loads the room once and applies both.
	if m.roomLoader != nil {
		if room, ok, err := m.roomLoader(tableID); err == nil && ok && room != nil {
			if room.BlindEscalation != nil {
				actor.StartEscalation(*room.BlindEscalation)
			}
			actor.SetTurnTimeoutForActor(table.TurnTimeoutFor(room.TurnTimeoutSeconds))
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
	m.mu.Lock()
	if a, ok := m.actors[tableID]; ok && a == expected && !a.IsAlive() {
		delete(m.actors, tableID)
		delete(m.cancels, tableID)
		delete(m.releases, tableID)
	}
	m.mu.Unlock()
}

// evictLeaseLessActorWhenIdle bounds the per-instance registry even when
// this node only proxied a table and never acquired its cache-affinity lease.
// A continuous zero-connection window is required: seeing any live socket
// resets the clock, so a busy table is never evicted between polling ticks.
func (m *Manager) evictLeaseLessActorWhenIdle(ctx context.Context, tableID string, actor *Actor, cancel context.CancelFunc) {
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
			if now.Sub(idleSince) < m.leaseLessIdleTimeout {
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
	delete(m.actors, tableID)
	cancel := m.cancels[tableID]
	delete(m.cancels, tableID)
	rel, hasRel := m.releases[tableID]
	delete(m.releases, tableID)
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if hasRel && rel != nil {
		rel()
	}
}

// DrainAndRelease releases every table lease held by this instance on graceful shutdown
// and waits for all table actor goroutines to finish processing in-flight operations.
func (m *Manager) DrainAndRelease(ctx context.Context) {
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
