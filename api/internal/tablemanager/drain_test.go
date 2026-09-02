package tablemanager

import (
	"context"
	"sync"
	"sync/atomic"
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

// TestDrainAndReleaseIsIdempotentUnderConcurrentInvocation proves #33's core
// fix: firing DrainAndRelease from multiple call sites for the same instance
// (e.g. the OnStop SIGTERM handler racing the proactive spot-termination
// poller) must release each held lease exactly once, never double-release
// it, no matter how many times or how concurrently it is invoked.
func TestDrainAndReleaseIsIdempotentUnderConcurrentInvocation(t *testing.T) {
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

	// Wrap each real lease release so we can count invocations without
	// changing what actually happens when one fires.
	var releaseCount int32
	m.mu.Lock()
	for id, rel := range m.releases {
		real := rel
		m.releases[id] = func() {
			atomic.AddInt32(&releaseCount, 1)
			real()
		}
	}
	m.mu.Unlock()

	const concurrentCallers = 5
	var wg sync.WaitGroup
	for range concurrentCallers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.DrainAndRelease(ctx)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&releaseCount); got != 2 {
		t.Fatalf("expected exactly 2 lease releases (one per table) across %d concurrent DrainAndRelease calls, got %d", concurrentCallers, got)
	}

	// A later, fully-sequential re-call (e.g. the lifecycle hook's own path
	// arriving after the proactive poller already drained) must also be a
	// pure no-op.
	m.DrainAndRelease(ctx)
	if got := atomic.LoadInt32(&releaseCount); got != 2 {
		t.Fatalf("expected sequential re-call after drain to release nothing new, got %d total releases", got)
	}
}
