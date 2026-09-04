//go:build integration

package table

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// TestStaleInstanceTurnTimeoutIgnoresATurnAnotherInstanceAlreadyAdvanced
// reproduces the 2026-09-04 incident (docs/specs/2026-09-04-cross-instance-
// stale-turn-timer.md): two fleet instances run independent Actors for the
// same table, each trusting its own local cache (internal/tablelease is
// latency-only, never an exclusive lock). Instance B loads the table once
// (mirroring a player's WS landing there) and then goes quiet — no further
// command ever reaches it — while instance A processes every actual action.
// A's fold moves the turn from p1 to p2; B never learns that. A time.AfterFunc
// on B, armed before A's fold and still holding B's now-stale belief that p1
// is on the clock, then fires.
//
// Before this fix, handleTurnTimeout's ensureLoaded(ctx, false) on a
// trustCache instance skipped the reload entirely, so B's CurrentPlayerIDForActor
// check passed against its own stale cache and it proceeded to charge time
// bank and fold p1 a second time from data up to several actions old — the
// live session's logs showed exactly this: a "table time bank consumed" line
// for a stage/hand a *different* instance had already carried forward.
// Forcing the reload (this change) makes B observe the real current player
// (p2) before deciding, so it no-ops instead.
func TestStaleInstanceTurnTimeoutIgnoresATurnAnotherInstanceAlreadyAdvanced(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := seed.StartHand(); err != nil {
		t.Fatalf("start hand: %v", err)
	}
	firstToAct := seed.CurrentPlayerIDForActor()
	if err := store.SeedTable(context.Background(), tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Instance A: processes every real action for the rest of this test.
	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	ctxA, cancelA := context.WithCancel(context.Background())
	go a.Run(ctxA)
	stopActor(t, a, cancelA)

	// Instance B: loads the table exactly once (its one player's WS landing),
	// then never hears from it again — mirroring a quiet connection on a
	// different fleet instance while the table keeps moving on A.
	b := New(tableID, store, true, func(string, hand.Snapshot) {})
	ctxB, cancelB := context.WithCancel(context.Background())
	go b.Run(ctxB)
	stopActor(t, b, cancelB)
	snap := make(chan hand.Snapshot, 1)
	if err := b.Dispatch(SnapshotCmd{PlayerID: firstToAct, Snapshot: snap}); err != nil {
		t.Fatalf("prime instance B: %v", err)
	}
	<-snap
	if b.cached.CurrentPlayerIDForActor() != firstToAct {
		t.Fatalf("instance B did not observe %s on the clock", firstToAct)
	}

	// Real action lands on A only: firstToAct folds, handing the turn to the
	// next player at the same stage (3-max, so the hand does not complete).
	reply := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: firstToAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply}); err != nil {
		t.Fatalf("dispatch fold: %v", err)
	}
	if err := <-reply; err != nil {
		t.Fatalf("fold: %v", err)
	}
	stored, err := store.LoadTable(context.Background(), tableID)
	if err != nil {
		t.Fatalf("load after fold: %v", err)
	}
	versionAfterFold := stored.Version
	realCurrent := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	if realCurrent == firstToAct {
		t.Fatal("test setup did not advance the turn past firstToAct")
	}

	// B's stale timer for firstToAct fires now, long after the real turn moved
	// on — this is the exact shape of the live incident's late time.AfterFunc.
	timeoutReply := make(chan error, 1)
	if err := b.handleTurnTimeout(context.Background(),
		turnTimeoutCmd{PlayerID: firstToAct, Reply: timeoutReply}); err != nil {
		t.Fatalf("stale timeout on instance B: %v", err)
	}

	stored, err = store.LoadTable(context.Background(), tableID)
	if err != nil {
		t.Fatalf("load after stale timeout: %v", err)
	}
	if stored.Version != versionAfterFold {
		t.Fatalf("stale instance B mutated table state: version %d -> %d",
			versionAfterFold, stored.Version)
	}
	if got := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor(); got != realCurrent {
		t.Fatalf("stale instance B changed current player: %s -> %s", realCurrent, got)
	}
}
