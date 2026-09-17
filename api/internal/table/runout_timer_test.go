package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestRunoutTimerIdempotencyIncludesTheRunoutPhase(t *testing.T) {
	a := New("table", nil, true, nil)
	a.handID = "hand"
	a.runoutStreetDelay = time.Hour
	a.SetCachedForTest(hand.NewTableFromState(hand.State{Stage: hand.Flop, RunoutPhase: 1}))
	a.armRunoutTimer(true, hand.Flop)
	first := a.runoutTimer
	t.Cleanup(func() {
		if a.runoutTimer != nil {
			a.runoutTimer.Stop()
		}
	})

	a.SetCachedForTest(hand.NewTableFromState(hand.State{Stage: hand.Flop, RunoutPhase: 2}))
	a.armRunoutTimer(true, hand.Flop)
	if a.runoutTimer == first {
		t.Fatal("phase two at the same stage must arm a fresh runout timer")
	}
}

// awaitingRunoutTable builds a table parked mid all-in runout: heads-up,
// pre-flop shove and call, so the flop is dealt inside the call and the turn
// and river are still owed with nobody able to act.
func awaitingRunoutTable(t *testing.T) *hand.Table {
	t.Helper()
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 30, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	state := seed.ExportState()
	state.DealerSeat = 0
	state.DealerDrawn = true
	table := hand.NewTableFromState(state)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	first := table.CurrentPlayerIDForActor()
	if err := table.Act(first, betting.ActionRaise, 30); err != nil {
		t.Fatalf("%s shoves: %v", first, err)
	}
	if err := table.Act(table.CurrentPlayerIDForActor(), betting.ActionCall, 30); err != nil {
		t.Fatalf("call the shove: %v", err)
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatalf("test setup did not park the table mid-runout: stage %v", table.Stage())
	}
	return table
}

// TestAFiredRunoutStepLeavesNoArmSuppressionBehind is the 2026-09-17 freeze
// (docs/specs/2026-09-17-frozen-table-runout-and-sitout-fold.md). The
// (handID, stage, phase) key means "a timer is pending for this point in the
// runout", so a fire that dealt nothing — a rejected commit, a failed load,
// or the stale fire simulated here — must leave the next arm free to schedule
// a retry. Read as "this point was armed once, ever", the hand froze on the
// flop forever: nobody is on the clock mid-runout, so no turn timer and no
// player action can recover it, and every self-healing arm
// (rearmTimersFromCache, on every reload, from any instance) was suppressed.
func TestAFiredRunoutStepLeavesNoArmSuppressionBehind(t *testing.T) {
	a := New("table", nil, true, nil)
	a.handID = "hand"
	a.runoutStreetDelay = time.Hour
	t.Cleanup(func() {
		if a.runoutTimer != nil {
			a.runoutTimer.Stop()
		}
	})
	awaiting := awaitingRunoutTable(t)

	a.SetCachedForTest(awaiting)
	a.armRunoutTimer(true, awaiting.Stage())
	fired := a.runoutTimer
	if fired == nil {
		t.Fatal("expected a pending runout timer for a table awaiting a runout")
	}

	// That timer fires and the step reaches no verdict — modelled here by a
	// cache that no longer awaits a runout, the one no-op exit the handler
	// has that commits nothing.
	a.SetCachedForTest(hand.NewTableFromState(hand.State{Stage: hand.Flop, RunoutPhase: 1}))
	if err := a.handleRunoutStep(context.Background(), runoutStepCmd{}); err != nil {
		t.Fatalf("a stale runout fire must be a silent no-op, got %v", err)
	}

	// The street is still owed, so the next arm — this actor's own broadcast,
	// or any instance reloading the table — must schedule another attempt.
	a.SetCachedForTest(awaiting)
	a.armRunoutTimer(true, awaiting.Stage())
	if a.runoutTimer == fired {
		t.Fatal("the runout is still owed but no new timer was armed: the hand is frozen")
	}
}
