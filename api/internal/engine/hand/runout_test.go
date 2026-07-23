package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
)

// TestAllInPreflopDealsFlopImmediatelyThenAwaitsPacedRunout covers the
// 3-missing-streets case: an all-in accepted at PreFlop deals the flop
// synchronously (same as advanceStage always has), then stops — pacing the
// turn and river is table.Actor's job (see IsAwaitingRunoutForActor), one
// street at a time via AdvanceRunoutStreetForActor.
func TestAllInPreflopDealsFlopImmediatelyThenAwaitsPacedRunout(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 30, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}

	// Heads-up: p1 (dealer) posts the small blind and acts first preflop.
	// Shove the rest of their stack; p2 calls and remains Active with plenty
	// of chips left -- nobody else can bet against them, so this triggers
	// the runout path.
	first := table.currentPlayerToAct()
	if err := table.Act(first, betting.ActionRaise, 30); err != nil {
		t.Fatalf("p1 shove: %v", err)
	}
	second := table.currentPlayerToAct()
	if err := table.Act(second, betting.ActionCall, 0); err != nil {
		t.Fatalf("p2 call: %v", err)
	}

	if table.Stage() != Flop {
		t.Fatalf("expected the flop dealt immediately, got stage %v", table.Stage())
	}
	if len(table.board) != 3 {
		t.Fatalf("expected 3 board cards after the immediate flop deal, got %d", len(table.board))
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatal("expected a paced runout to still be pending (turn + river missing)")
	}

	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Turn || len(table.board) != 4 {
		t.Fatalf("expected the turn dealt next, got stage %v with %d board cards", table.Stage(), len(table.board))
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatal("expected the river still pending")
	}

	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Complete || len(table.board) != 5 {
		t.Fatalf("expected the river dealt and showdown run, got stage %v with %d board cards", table.Stage(), len(table.board))
	}
	if table.IsAwaitingRunoutForActor() {
		t.Fatal("expected no runout pending once showdown has run")
	}
	if table.LastOutcomeForActor() == nil {
		t.Fatal("expected showdown to have recorded an outcome")
	}
}

// TestAllInWithOnlyRiverMissingSkipsPacing covers the single-missing-street
// case: nothing to pace, the last street reveals and showdown runs in the
// same call, same as normal (non-all-in) play.
func TestAllInWithOnlyRiverMissingSkipsPacing(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}

	// Preflop: dealer (SB) calls up to the big blind, BB checks behind.
	preflopFirst := table.currentPlayerToAct()
	if err := table.Act(preflopFirst, betting.ActionCall, 0); err != nil {
		t.Fatalf("preflop call: %v", err)
	}
	preflopSecond := table.currentPlayerToAct()
	if err := table.Act(preflopSecond, betting.ActionCheck, 0); err != nil {
		t.Fatalf("preflop check: %v", err)
	}
	if table.Stage() != Flop {
		t.Fatalf("expected Flop after preflop, got %v", table.Stage())
	}

	// Flop: both check through, no chips committed.
	flopFirst := table.currentPlayerToAct()
	if err := table.Act(flopFirst, betting.ActionCheck, 0); err != nil {
		t.Fatalf("flop first check: %v", err)
	}
	flopSecond := table.currentPlayerToAct()
	if err := table.Act(flopSecond, betting.ActionCheck, 0); err != nil {
		t.Fatalf("flop second check: %v", err)
	}
	if table.Stage() != Turn {
		t.Fatalf("expected Turn after flop, got %v", table.Stage())
	}

	// Turn: first-to-act shoves their entire remaining stack, the other
	// calls -- only the river is left to deal.
	turnFirst := table.currentPlayerToAct()
	shoveAmount := table.playerByID(turnFirst).Stack
	if err := table.Act(turnFirst, betting.ActionBet, shoveAmount); err != nil {
		t.Fatalf("turn shove: %v", err)
	}
	turnSecond := table.currentPlayerToAct()
	if err := table.Act(turnSecond, betting.ActionCall, 0); err != nil {
		t.Fatalf("turn call: %v", err)
	}

	if table.Stage() != Complete {
		t.Fatalf("expected the river to reveal and showdown to run immediately (only one street was missing), got stage %v", table.Stage())
	}
	if len(table.board) != 5 {
		t.Fatalf("expected all 5 board cards dealt, got %d", len(table.board))
	}
	if table.LastOutcomeForActor() == nil {
		t.Fatal("expected showdown to have recorded an outcome")
	}
}
