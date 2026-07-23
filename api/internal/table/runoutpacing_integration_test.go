//go:build integration

package table

import (
	"context"
	"sync"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// TestAllInRunoutPacesEachStreetBehindATimer drives a heads-up preflop
// all-in (p1 short-stacked) through the real Actor/store commit path and
// confirms the flop is dealt immediately (same broadcast as the call), then
// the turn and river each arrive roughly runoutStreetDelay apart -- not all
// five cards in a single broadcast.
func TestAllInRunoutPacesEachStreetBehindATimer(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 30, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	state := seed.ExportState()
	state.DealerSeat = 0
	state.DealerDrawn = true
	ctx := context.Background()
	if err := store.SeedTable(ctx, "table-runout-pacing", state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var mu sync.Mutex
	firstSeenAt := map[int]time.Time{}
	a := New("table-runout-pacing", store, true, func(_ string, snapshot hand.Snapshot) {
		mu.Lock()
		if _, ok := firstSeenAt[len(snapshot.Board)]; !ok {
			firstSeenAt[len(snapshot.Board)] = time.Now()
		}
		mu.Unlock()
	})
	a.runoutStreetDelay = 200 * time.Millisecond
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go a.Run(runCtx)

	mustDispatch(t, a, ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
	mustDispatch(t, a, ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})

	stored, err := store.LoadTable(ctx, "table-runout-pacing")
	if err != nil || stored == nil {
		t.Fatalf("expected hand to have started, got %+v err=%v", stored, err)
	}
	first := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	mustDispatch(t, a, ActCmd{PlayerID: first, ActionID: "shove", Action: betting.ActionRaise, Amount: 30, Reply: make(chan error, 1)})

	stored, _ = store.LoadTable(ctx, "table-runout-pacing")
	second := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	callStart := time.Now()
	mustDispatch(t, a, ActCmd{PlayerID: second, ActionID: "call", Action: betting.ActionCall, Reply: make(chan error, 1)})

	// The flop (3 cards) must already be visible in the broadcast triggered
	// by this very Act call -- no timer needed for the immediate street.
	mu.Lock()
	_, gotFlop := firstSeenAt[3]
	mu.Unlock()
	if !gotFlop {
		t.Fatal("expected the flop dealt immediately in the same broadcast as the call")
	}

	waitForBoardLen(t, &mu, firstSeenAt, 5, 3*time.Second)

	mu.Lock()
	turnAt, turnOK := firstSeenAt[4]
	riverAt, riverOK := firstSeenAt[5]
	mu.Unlock()
	if !turnOK || !riverOK {
		t.Fatalf("expected both a 4-card and 5-card broadcast, got %+v", firstSeenAt)
	}
	if elapsed := riverAt.Sub(turnAt); elapsed < 150*time.Millisecond {
		t.Fatalf("expected the river roughly %v after the turn, got %v", a.runoutStreetDelay, elapsed)
	}
	if elapsed := turnAt.Sub(callStart); elapsed < 150*time.Millisecond {
		t.Fatalf("expected the turn roughly %v after the all-in call, got %v", a.runoutStreetDelay, elapsed)
	}

	stored, _ = store.LoadTable(ctx, "table-runout-pacing")
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected the hand Complete once the runout finishes, got %v", stored.State.Stage)
	}
}

func mustDispatch(t *testing.T, a *Actor, cmd Command) {
	t.Helper()
	if err := a.Dispatch(cmd); err != nil {
		t.Fatalf("dispatch %T: %v", cmd, err)
	}
}

func waitForBoardLen(t *testing.T, mu *sync.Mutex, seen map[int]time.Time, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		_, ok := seen[n]
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for a %d-card board broadcast", n)
}
