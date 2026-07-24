package tablemanager

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
)

func TestDrainAndReleaseFreesEveryLocallyOwnedTable(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	leases := tablelease.NewService(backend)
	m := NewManager(leases, nil, nil, nil)
	ctx := context.Background()
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }

	if _, err := m.GetOrCreateActor(ctx, "table-1", seed); err != nil {
		t.Fatalf("get or create table-1: %v", err)
	}
	if _, err := m.GetOrCreateActor(ctx, "table-2", seed); err != nil {
		t.Fatalf("get or create table-2: %v", err)
	}

	m.DrainAndRelease(ctx)

	leases2 := tablelease.NewService(backend)
	m2 := NewManager(leases2, nil, nil, nil)
	if _, err := m2.GetOrCreateActor(ctx, "table-1", seed); err != nil {
		t.Fatalf("expected sibling instance to acquire table-1 after drain, got %v", err)
	}
	if _, err := m2.GetOrCreateActor(ctx, "table-2", seed); err != nil {
		t.Fatalf("expected sibling instance to acquire table-2 after drain, got %v", err)
	}
}
