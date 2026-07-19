// Package tablemanager is the per-instance registry of live table Actors: it
// decides, for any table ID, whether this instance owns it (acquire the
// lease, recover from the last snapshot + replay the log, start an Actor) or
// which instance does (read tableowner.Registry so the caller can proxy).
package tablemanager

import (
	"context"
	"fmt"
	"sync"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tableowner"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// Actor is table.Actor, re-exported so callers only need to import
// tablemanager.
type Actor = table.Actor

type Manager struct {
	leases       *tablelease.Service
	owners       *tableowner.Registry
	store        *tablestore.Store
	instanceAddr string
	broadcast    func(tableID, viewerID string, snap hand.Snapshot)

	mu       sync.Mutex
	actors   map[string]*Actor
	releases map[string]func()
}

func NewManager(leases *tablelease.Service, owners *tableowner.Registry, store *tablestore.Store, instanceAddr string, broadcast func(string, string, hand.Snapshot)) *Manager {
	return &Manager{
		leases:       leases,
		owners:       owners,
		store:        store,
		instanceAddr: instanceAddr,
		broadcast:    broadcast,
		actors:       make(map[string]*Actor),
		releases:     make(map[string]func()),
	}
}

// Acquire returns this instance's live Actor for tableID, acquiring the
// table's write lease and recovering state if this is the first request for
// it on this instance. seed is only invoked when no snapshot exists yet
// (the table has never been played).
func (m *Manager) Acquire(ctx context.Context, tableID string, seed func() *hand.Table) (*Actor, error) {
	m.mu.Lock()
	if a, ok := m.actors[tableID]; ok {
		m.mu.Unlock()
		return a, nil
	}
	m.mu.Unlock()

	release, ok, err := m.leases.Acquire(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("tablemanager: acquire lease: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("tablemanager: table %s is owned by another instance", tableID)
	}

	ht, err := m.recoverOrSeed(ctx, tableID, seed)
	if err != nil {
		release()
		return nil, err
	}

	actor := table.New(tableID, ht, m.store, m.broadcastFor(tableID))
	runCtx, cancel := context.WithCancel(context.Background())
	go actor.Run(runCtx)

	if err := m.owners.Advertise(ctx, tableID, m.instanceAddr); err != nil {
		// Advertising is routing-only, not authority — losing this write
		// just means a sibling instance can't find us to proxy yet; the
		// lease itself (already acquired above) is unaffected.
		_ = err
	}
	m.leases.StartHeartbeat(runCtx, tableID, func() {
		cancel() // lease lost — this Actor must stop mutating hand.Table immediately
		m.mu.Lock()
		delete(m.actors, tableID)
		delete(m.releases, tableID)
		m.mu.Unlock()
	})

	m.mu.Lock()
	m.actors[tableID] = actor
	m.releases[tableID] = release
	m.mu.Unlock()
	return actor, nil
}

// recoverOrSeed loads the latest durable snapshot and replays the action log
// recorded since it into a fresh hand.Table (Replay re-runs each entry
// through the table's own Act, so recovery can never drift from live
// behavior). seed is used only when no snapshot exists yet — a table played
// for the first time.
func (m *Manager) recoverOrSeed(ctx context.Context, tableID string, seed func() *hand.Table) (*hand.Table, error) {
	if m.store == nil {
		return seed(), nil
	}
	snap, err := m.store.LoadSnapshot(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("tablemanager: load snapshot: %w", err)
	}
	if snap == nil {
		return seed(), nil
	}
	entries, err := m.store.LoadActionsSince(ctx, tableID, snap.HandID, snap.Seq)
	if err != nil {
		return nil, fmt.Errorf("tablemanager: load actions since seq %d: %w", snap.Seq, err)
	}
	replayed := make([]hand.ReplayedAction, len(entries))
	for i, e := range entries {
		replayed[i] = hand.ReplayedAction{PlayerID: e.PlayerID, Action: betting.Action(e.Action), Amount: e.Amount}
	}
	ht := seed()
	if err := ht.Replay(snap.HandID, replayed); err != nil {
		return nil, fmt.Errorf("tablemanager: replay: %w", err)
	}
	return ht, nil
}

func (m *Manager) broadcastFor(tableID string) func(string, hand.Snapshot) {
	return func(viewerID string, snap hand.Snapshot) {
		if m.broadcast != nil {
			m.broadcast(tableID, viewerID, snap)
		}
	}
}

// Locate returns the local Actor for tableID if this instance owns it, or
// the remote owner's advertised address otherwise. Exactly one of the two
// return values is non-zero.
func (m *Manager) Locate(ctx context.Context, tableID string) (*Actor, string, error) {
	m.mu.Lock()
	if a, ok := m.actors[tableID]; ok {
		m.mu.Unlock()
		return a, "", nil
	}
	m.mu.Unlock()

	addr, ok, err := m.owners.Lookup(ctx, tableID)
	if err != nil {
		return nil, "", fmt.Errorf("tablemanager: lookup owner: %w", err)
	}
	if !ok {
		return nil, "", fmt.Errorf("tablemanager: table %s has no known owner", tableID)
	}
	return nil, addr, nil
}

// ReleaseForTest force-releases tableID's lease and drops its Actor without
// waiting for the heartbeat to lapse — exists only so integration tests can
// simulate a crash without sleeping out the real lease TTL.
func (m *Manager) ReleaseForTest(tableID string) {
	m.mu.Lock()
	release, ok := m.releases[tableID]
	delete(m.actors, tableID)
	delete(m.releases, tableID)
	m.mu.Unlock()
	if ok {
		release()
	}
}
