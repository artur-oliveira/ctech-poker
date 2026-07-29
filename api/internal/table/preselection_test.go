package table

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
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

func TestInlinePreselectionExecution(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}

	actor := New("preselection-inline", nil, true, nil)
	actor.cached = game
	actor.handID = "hand-1"
	actor.version = 1

	// Current player is p1. Set preselection call_any for p2.
	p1 := game.CurrentPlayerIDForActor()
	var p2 string
	for _, id := range []string{"p1", "p2", "p3"} {
		if id != p1 {
			p2 = id
			break
		}
	}

	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: p2, ActionID: "preselect-call-any", Selection: "call_any", Amount: 0,
		ExpectedSnapshotVersion: 1, ExpectedHandID: "hand-1",
	}); err != nil {
		t.Fatalf("set preselect call_any: %v", err)
	}

	// Now p1 calls. broadcastAll will trigger processInlinePreselections for p2.
	callAmount := game.ProspectiveCallAmountForActor(p1)
	if _, err := actor.applyActAndCommit(context.Background(), ActCmd{
		PlayerID: p1, ActionID: "p1-call", Action: betting.ActionCall, Amount: callAmount,
	}); err != nil {
		t.Fatalf("p1 call: %v", err)
	}

	actor.processInlinePreselections(context.Background())

	// p2's preselection should have executed inline, moving the turn past p2.
	if current := game.CurrentPlayerIDForActor(); current == p2 {
		t.Fatalf("turn is still p2 after processInlinePreselections")
	}
}

func TestPreselectionAcceptsUnrelatedVersionChangeWithinSameStreet(t *testing.T) {
	actor, game, _ := newTimeBankActor(t)
	actor.version = 8
	waiting := "p1"
	if waiting == game.CurrentPlayerIDForActor() {
		waiting = "p2"
	}

	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: waiting, ActionID: "preselect-after-reaction", Selection: "check_fold",
		ExpectedSnapshotVersion: 7, ExpectedHandID: actor.handID, ExpectedStage: game.ViewFor("").Stage,
	}); err != nil {
		t.Fatalf("same-street preselection should ignore unrelated version change: %v", err)
	}
}

func TestPreselectionCannotCrossStreetOrHand(t *testing.T) {
	actor, game, current := newTimeBankActor(t)
	actor.activity.Preselections = map[string]tablestore.Preselection{
		current: {Selection: "fold", HandID: actor.handID, Stage: string(hand.Flop)},
	}

	actor.processInlinePreselections(context.Background())
	if _, ok := actor.activity.Preselections[current]; ok {
		t.Fatal("preselection from another street survived pruning")
	}
	if game.CurrentPlayerIDForActor() != current {
		t.Fatal("stale preselection executed on the current player")
	}

	actor.activity.Preselections[current] = tablestore.Preselection{
		Selection: "fold", HandID: "previous-hand", Stage: string(hand.PreFlop),
	}
	actor.processInlinePreselections(context.Background())
	if _, ok := actor.activity.Preselections[current]; ok {
		t.Fatal("preselection from another hand survived pruning")
	}
}
