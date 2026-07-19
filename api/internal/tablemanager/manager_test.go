package tablemanager

import (
	"context"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tableowner"
)

func TestAcquireCreatesActorOnFirstCall(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	m := NewManager(tablelease.NewService(backend), tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL), nil, "10.0.0.5:8010", nil)
	ctx := context.Background()

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20)
	}

	actor, err := m.Acquire(ctx, "table-1", seed)
	if err != nil || actor == nil {
		t.Fatalf("expected acquire to succeed, got actor=%v err=%v", actor, err)
	}

	local, remote, err := m.Locate(ctx, "table-1")
	if err != nil || local != actor {
		t.Fatalf("expected Locate to find the local actor, got local=%v remote=%q err=%v", local, remote, err)
	}
}

func TestLocateReturnsRemoteOwnerWhenNotHeldLocally(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	leases := tablelease.NewService(backend)
	owners := tableowner.NewRegistry(backend, tablelease.DefaultLeaseTTL)
	ctx := context.Background()

	// Simulate a different instance already owning table-2.
	release, ok, err := leases.Acquire(ctx, "table-2")
	if err != nil || !ok {
		t.Fatalf("seed acquire: ok=%v err=%v", ok, err)
	}
	defer release()
	if err := owners.Advertise(ctx, "table-2", "10.0.0.9:8010"); err != nil {
		t.Fatalf("advertise: %v", err)
	}

	m := NewManager(leases, owners, nil, "10.0.0.5:8010", nil)
	local, remote, err := m.Locate(ctx, "table-2")
	if err != nil || local != nil || remote != "10.0.0.9:8010" {
		t.Fatalf("expected remote owner 10.0.0.9:8010, got local=%v remote=%q err=%v", local, remote, err)
	}
}
