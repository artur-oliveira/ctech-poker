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

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 30, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	state := seed.ExportState()
	state.DealerSeat = 0
	state.DealerDrawn = true
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var mu sync.Mutex
	firstSeenAt := map[int]time.Time{}
	a := New(tableID, store, true, func(_ string, snapshot hand.Snapshot) {
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

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil || stored == nil {
		t.Fatalf("expected hand to have started, got %+v err=%v", stored, err)
	}
	first := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	mustDispatch(t, a, ActCmd{PlayerID: first, ActionID: "shove", Action: betting.ActionRaise, Amount: 30, Reply: make(chan error, 1)})

	stored, _ = store.LoadTable(ctx, tableID)
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

	stored, _ = store.LoadTable(ctx, tableID)
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected the hand Complete once the runout finishes, got %v", stored.State.Stage)
	}
}

// TestRunoutSelfHealsAfterActorDiesMidPace reproduces a fleet failure mode
// found from production websocket traces: armRunoutTimer is a bare
// process-local time.AfterFunc with nothing persisted alongside it. If the
// Actor instance that armed it dies (crash, deploy, lease handoff) before it
// fires, the hand is stuck forever mid all-in-runout — the board never
// finishes dealing — even though the persisted table state is perfectly
// fine and playable. A brand new Actor instance (as if a different fleet
// node picked the table back up) must self-heal this the moment it loads
// state at all, even via something as passive as a Snapshot request.
func TestRunoutSelfHealsAfterActorDiesMidPace(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 30, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	state := seed.ExportState()
	state.DealerSeat = 0
	state.DealerDrawn = true
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// First "node": drives the shove + call (the flop is dealt synchronously
	// inside that Act call, no timer needed yet), then vanishes — its own
	// runout timer is set absurdly long so it can never fire during this
	// test, standing in for a node that died before its timer ever went off.
	gone := New(tableID, store, true, func(string, hand.Snapshot) {})
	gone.runoutStreetDelay = time.Hour
	goneCtx, cancelGone := context.WithCancel(ctx)
	go gone.Run(goneCtx)
	mustDispatch(t, gone, ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
	mustDispatch(t, gone, ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})
	stored, err := store.LoadTable(ctx, tableID)
	if err != nil || stored == nil {
		t.Fatalf("expected hand to have started, got %+v err=%v", stored, err)
	}
	first := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	mustDispatch(t, gone, ActCmd{PlayerID: first, ActionID: "shove", Action: betting.ActionRaise, Amount: 30, Reply: make(chan error, 1)})
	stored, _ = store.LoadTable(ctx, tableID)
	second := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	mustDispatch(t, gone, ActCmd{PlayerID: second, ActionID: "call", Action: betting.ActionCall, Reply: make(chan error, 1)})
	cancelGone()

	stored, _ = store.LoadTable(ctx, tableID)
	if stored.State.Stage != hand.Flop {
		t.Fatalf("expected the flop dealt and the hand stuck awaiting a runout, got stage %v", stored.State.Stage)
	}

	// Second "node": a fresh Actor for the same table, standing in for
	// whichever instance next picks this table up. It never sees an Act,
	// Ready, or any command that mutates game state — only a passive
	// Snapshot request (the same thing a WS connect issues). ensureLoaded
	// must still notice the stuck runout and re-arm it from durable state.
	var mu sync.Mutex
	firstSeenAt := map[int]time.Time{}
	revived := New(tableID, store, true, func(_ string, snapshot hand.Snapshot) {
		mu.Lock()
		if _, ok := firstSeenAt[len(snapshot.Board)]; !ok {
			firstSeenAt[len(snapshot.Board)] = time.Now()
		}
		mu.Unlock()
	})
	revived.runoutStreetDelay = 50 * time.Millisecond
	revivedCtx, cancelRevived := context.WithCancel(ctx)
	t.Cleanup(cancelRevived)
	go revived.Run(revivedCtx)

	mustDispatch(t, revived, SnapshotCmd{PlayerID: "p1", Snapshot: make(chan hand.Snapshot, 1), Reply: make(chan error, 1)})

	waitForBoardLen(t, &mu, firstSeenAt, 5, 3*time.Second)

	stored, _ = store.LoadTable(ctx, tableID)
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected the revived actor's re-armed runout to finish the hand, got stage %v", stored.State.Stage)
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
