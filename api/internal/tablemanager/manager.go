// Package tablemanager is the per-instance registry of live table Actors.
// There is no "owner" of a table under this revision (ARCHITECTURE.md §2):
// any instance may create an Actor for any table at any time. tablelease is
// consulted only to decide whether that Actor may trust its own in-memory
// cache between commits — never to gate whether it may be created at all.
package tablemanager

import (
	"context"
	"fmt"
	"sync"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type Actor = table.Actor

type Manager struct {
	leases         *tablelease.Service
	store          *tablestore.Store
	broadcast      func(tableID, viewerID string, snap hand.Snapshot)
	onHandComplete func(tableID string, outcome hand.HandOutcome)
	roomLoader     func(tableID string) (*roomstore.BlindEscalation, bool, error)

	mu     sync.Mutex
	actors map[string]*Actor
}

func NewManager(leases *tablelease.Service, store *tablestore.Store, broadcast func(string, string, hand.Snapshot), roomLoader func(string) (*roomstore.BlindEscalation, bool, error), completion ...func(string, hand.HandOutcome)) *Manager {
	var onHandComplete func(string, hand.HandOutcome)
	if len(completion) > 0 {
		onHandComplete = completion[0]
	}
	return &Manager{leases: leases, store: store, broadcast: broadcast, onHandComplete: onHandComplete, roomLoader: roomLoader, actors: make(map[string]*Actor)}
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

	if m.store != nil {
		existing, err := m.store.LoadTable(ctx, tableID)
		if err != nil {
			return nil, fmt.Errorf("tablemanager: load table: %w", err)
		}
		if existing == nil {
			if err := m.store.SeedTable(ctx, tableID, seed().ExportState()); err != nil {
				return nil, fmt.Errorf("tablemanager: seed table: %w", err)
			}
		}
	}

	trustCache := false
	if m.leases != nil {
		if _, ok, err := m.leases.Acquire(ctx, tableID); err == nil && ok {
			trustCache = true
		}
	}

	actor := table.New(tableID, m.store, trustCache, m.broadcastFor(tableID))
	actor.SetOnHandCompleteForActor(func(outcome hand.HandOutcome) {
		if m.onHandComplete != nil {
			m.onHandComplete(tableID, outcome)
		}
	})
	if trustCache {
		// Only cancelable when there's a real cancellation trigger (losing
		// the lease); an Actor without cache-affinity runs for the process
		// lifetime regardless, same as this branch's counterpart below.
		runCtx, cancel := context.WithCancel(context.Background())
		m.leases.StartHeartbeat(runCtx, tableID, func() {
			cancel()
			m.removeActor(tableID)
		})
		go actor.Run(runCtx)
	} else {
		go actor.Run(context.Background())
	}

	m.actors[tableID] = actor

	// Re-arm blind escalation from the room's authoritative config so it
	// survives instance/lease moves (T6). Any instance creating the actor
	// loads the room and starts escalation.
	if m.roomLoader != nil {
		if cfg, ok, err := m.roomLoader(tableID); err == nil && ok && cfg != nil {
			actor.StartEscalation(*cfg)
		}
	}

	for _, hook := range onCreated {
		hook(actor)
	}
	return actor, nil
}

// removeActor drops a (dead) actor from the registry. Safe to call from the
// lease-loss callback (runs off the Run goroutine) — it takes m.mu.
func (m *Manager) removeActor(tableID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.actors[tableID]; ok && !a.IsAlive() {
		delete(m.actors, tableID)
	}
}

func (m *Manager) broadcastFor(tableID string) func(string, hand.Snapshot) {
	return func(viewerID string, snap hand.Snapshot) {
		if m.broadcast != nil {
			m.broadcast(tableID, viewerID, snap)
		}
	}
}
