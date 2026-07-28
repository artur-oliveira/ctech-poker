package tablemanager

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
)

func TestGetOrCreateActorReturnsSameActorOnSecondCall(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	m := NewManager(tablelease.NewService(backend), nil, nil, nil)
	ctx := context.Background()

	seed := func() *hand.Table { return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20) }

	a1, err := m.GetOrCreateActor(ctx, "table-1", seed)
	if err != nil || a1 == nil {
		t.Fatalf("expected first call to succeed, got actor=%v err=%v", a1, err)
	}
	a2, err := m.GetOrCreateActor(ctx, "table-1", seed)
	if err != nil || a2 != a1 {
		t.Fatalf("expected the same Actor on the second call, got a1=%p a2=%p err=%v", a1, a2, err)
	}
}

func TestLeaseLessActorIsEvictedAfterContinuousIdleWindow(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)
	m.leaseLessIdleTimeout = 15 * time.Millisecond
	m.idleCheckInterval = 5 * time.Millisecond
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	a, err := m.GetOrCreateActor(context.Background(), "idle-table", seed)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-a.Done():
	case <-time.After(250 * time.Millisecond):
		t.Fatal("lease-less actor was not stopped after its idle window")
	}

	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		m.mu.Lock()
		_, retained := m.actors["idle-table"]
		m.mu.Unlock()
		if !retained {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stopped lease-less actor remained in manager registry")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGetOrCreateActorSucceedsEvenWhenLeaseIsHeldElsewhere(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	leases := tablelease.NewService(backend)
	ctx := context.Background()

	release, ok, err := leases.Acquire(ctx, "table-2")
	if err != nil || !ok {
		t.Fatalf("seed acquire: ok=%v err=%v", ok, err)
	}
	defer release()

	m := NewManager(leases, nil, nil, nil)
	seed := func() *hand.Table { return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}}, 10, 20) }
	a, err := m.GetOrCreateActor(ctx, "table-2", seed)
	if err != nil || a == nil {
		t.Fatalf("expected GetOrCreateActor to still succeed without the lease, got actor=%v err=%v", a, err)
	}
}

func TestGetOrCreateActorRunsHookOnlyForNewActor(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	created := 0
	if _, err := m.GetOrCreateActor(context.Background(), "table-hook", seed, func(*Actor) { created++ }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetOrCreateActor(context.Background(), "table-hook", seed, func(*Actor) { created++ }); err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("creation hook ran %d times, want 1", created)
	}
}
