package table

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestRaiseCancelsFixedCallWhenAmountChanges(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	current := game.CurrentPlayerIDForActor()
	var waitingPlayer string
	var selectedAmount int64
	for _, playerID := range []string{"p1", "p2", "p3"} {
		amount := game.ProspectiveCallAmountForActor(playerID)
		if playerID != current && amount > 0 {
			waitingPlayer, selectedAmount = playerID, amount
			break
		}
	}
	if waitingPlayer == "" {
		t.Fatal("test setup has no waiting player facing a call")
	}

	actor := New("preselection", nil, true, nil)
	actor.cached = game
	actor.handID = "hand-1"
	actor.version = 1
	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: waitingPlayer, ActionID: "preselect-call", Selection: "call", Amount: selectedAmount,
		ExpectedSnapshotVersion: 1, ExpectedHandID: "hand-1",
	}); err != nil {
		t.Fatalf("select fixed call: %v", err)
	}
	if got := actor.activity.Preselections[waitingPlayer].Amount; got != selectedAmount {
		t.Fatalf("persisted fixed call=%d, want %d", got, selectedAmount)
	}

	legal := game.ViewFor(current).LegalActions
	if legal == nil || legal.MinRaiseTo <= 0 {
		t.Fatal("current player cannot raise in test setup")
	}
	if _, err := actor.applyActAndCommit(context.Background(), ActCmd{
		PlayerID: current, ActionID: "raise-1", Action: betting.ActionRaise, Amount: legal.MinRaiseTo,
	}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if _, ok := actor.activity.Preselections[waitingPlayer]; ok {
		t.Fatal("fixed call survived after the amount to call increased")
	}
}
