package tablemanager

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/equity"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
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

func TestActorIsEvictedAfterContinuousIdleWindow(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)
	m.actorIdleTimeout = 15 * time.Millisecond
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

func TestLeaseHoldingActorIsEvictedAfterContinuousIdleWindow(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	leases := tablelease.NewService(backend)
	m := NewManager(leases, nil, nil, nil)
	m.actorIdleTimeout = 15 * time.Millisecond
	m.idleCheckInterval = 5 * time.Millisecond
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	a, err := m.GetOrCreateActor(context.Background(), "leased-idle-table", seed)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-a.Done():
	case <-time.After(250 * time.Millisecond):
		t.Fatal("lease-holding actor was not stopped after its idle window")
	}

	deadline := time.Now().Add(250 * time.Millisecond)
	for m.LiveActorCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("stopped lease-holding actor remained in manager registry")
		}
		time.Sleep(time.Millisecond)
	}

	for {
		if release, ok, err := leases.Acquire(context.Background(), "leased-idle-table"); err != nil {
			t.Fatal(err)
		} else if ok {
			release()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("idle eviction did not release cache-affinity lease")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestReleaseEvictsTablesGlobalEquityEntries(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	if _, err := m.GetOrCreateActor(context.Background(), "equity-table", seed); err != nil {
		t.Fatal(err)
	}
	hole := [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.King, Suit: deck.Clubs}}
	if _, _, err := equity.EstimateForTableWithStats("equity-table", hole, nil, nil, 1, 10); err != nil {
		t.Fatal(err)
	}
	if _, stats, err := equity.EstimateForTableWithStats("equity-table", hole, nil, nil, 1, 10); err != nil || !stats.CacheHit {
		t.Fatalf("expected cached table equity before release: stats=%+v err=%v", stats, err)
	}

	m.Release("equity-table")
	if _, stats, err := equity.EstimateForTableWithStats("equity-table", hole, nil, nil, 1, 10); err != nil || stats.CacheHit {
		t.Fatalf("expected table equity miss after release: stats=%+v err=%v", stats, err)
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

func TestOnAutoRebuySweepFiresAfterHandCompletes(t *testing.T) {
	m := NewManager(nil, nil, nil, nil)

	var gotTableID, gotHandID string
	var gotOutcome hand.HandOutcome
	m.SetOnAutoRebuySweep(func(tableID, handID string, outcome hand.HandOutcome) {
		gotTableID, gotHandID, gotOutcome = tableID, handID, outcome
	})

	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{
			{ID: "p1", Stack: 1000, Ready: true},
			{ID: "p2", Stack: 1000, Ready: true},
		}, 10, 20)
	}
	actor, err := m.GetOrCreateActor(context.Background(), "table-1", seed)
	if err != nil {
		t.Fatalf("get or create actor: %v", err)
	}

	reply1 := make(chan error, 1)
	if err := actor.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply1}); err != nil {
		t.Fatalf("ready p1: %v", err)
	}
	reply2 := make(chan error, 1)
	if err := actor.Dispatch(table.ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ready p2: %v", err)
	}

	toAct := actor.TableForTest().CurrentPlayerIDForActor()
	reply3 := make(chan error, 1)
	if err := actor.Dispatch(table.ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply3}); err != nil {
		t.Fatalf("fold: %v", err)
	}

	if gotTableID != "table-1" || gotHandID == "" {
		t.Fatalf("expected sweep to fire with tableID=table-1 non-empty handID, got tableID=%q handID=%q", gotTableID, gotHandID)
	}
	if len(gotOutcome.Participants) == 0 {
		t.Fatal("expected a non-empty outcome.Participants")
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
