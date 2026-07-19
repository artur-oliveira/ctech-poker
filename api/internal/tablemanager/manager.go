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
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

type Actor = table.Actor

type Manager struct {
	leases    *tablelease.Service
	store     *tablestore.Store
	broadcast func(tableID, viewerID string, snap hand.Snapshot)

	mu     sync.Mutex
	actors map[string]*Actor
}

func NewManager(leases *tablelease.Service, store *tablestore.Store, broadcast func(string, string, hand.Snapshot)) *Manager {
	return &Manager{leases: leases, store: store, broadcast: broadcast, actors: make(map[string]*Actor)}
}

// GetOrCreateActor returns this instance's Actor for tableID, seeding the
// table's very first DynamoDB state if it has never been played (seed is
// only invoked then). A failed best-effort lease acquire never blocks this —
// it only means the resulting Actor re-reads DynamoDB before every command
// instead of trusting its cache between commits.
func (m *Manager) GetOrCreateActor(ctx context.Context, tableID string, seed func() *hand.Table) (*Actor, error) {
	m.mu.Lock()
	if a, ok := m.actors[tableID]; ok {
		m.mu.Unlock()
		return a, nil
	}
	m.mu.Unlock()

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
	if trustCache {
		// Only cancelable when there's a real cancellation trigger (losing
		// the lease); an Actor without cache-affinity runs for the process
		// lifetime regardless, same as this branch's counterpart below.
		runCtx, cancel := context.WithCancel(context.Background())
		go actor.Run(runCtx)
		m.leases.StartHeartbeat(runCtx, tableID, func() { cancel() })
	} else {
		go actor.Run(context.Background())
	}

	m.mu.Lock()
	m.actors[tableID] = actor
	m.mu.Unlock()
	return actor, nil
}

func (m *Manager) broadcastFor(tableID string) func(string, hand.Snapshot) {
	return func(viewerID string, snap hand.Snapshot) {
		if m.broadcast != nil {
			m.broadcast(tableID, viewerID, snap)
		}
	}
}
