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

func TestCheckFoldCancelledByNewRaise(t *testing.T) {
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
	for _, playerID := range []string{"p1", "p2", "p3"} {
		if playerID != current && game.ProspectiveCallAmountForActor(playerID) == 0 {
			waitingPlayer = playerID
			break
		}
	}
	if waitingPlayer == "" {
		t.Fatal("test setup has no waiting player facing a free check")
	}

	actor := New("preselection-checkfold-cancel", nil, true, nil)
	actor.cached = game
	actor.handID = "hand-1"
	actor.version = 1
	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: waitingPlayer, ActionID: "preselect-check-fold", Selection: "check_fold",
		ExpectedSnapshotVersion: 1, ExpectedHandID: "hand-1",
	}); err != nil {
		t.Fatalf("select check_fold: %v", err)
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
		t.Fatal("check_fold survived a new raise that changed the amount to call")
	}
}

func TestCheckFoldStillResolvesFoldWhenAmountUnchanged(t *testing.T) {
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
	for _, playerID := range []string{"p1", "p2", "p3"} {
		if playerID != current && game.ProspectiveCallAmountForActor(playerID) > 0 {
			waitingPlayer = playerID
			break
		}
	}
	if waitingPlayer == "" {
		t.Fatal("test setup has no waiting player already facing a call")
	}

	actor := New("preselection-checkfold-regression", nil, true, nil)
	actor.cached = game
	actor.handID = "hand-1"
	actor.version = 1
	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: waitingPlayer, ActionID: "preselect-check-fold", Selection: "check_fold",
		ExpectedSnapshotVersion: 1, ExpectedHandID: "hand-1",
	}); err != nil {
		t.Fatalf("select check_fold: %v", err)
	}

	for game.CurrentPlayerIDForActor() != waitingPlayer {
		acting := game.CurrentPlayerIDForActor()
		if acting == "" {
			t.Fatal("test setup: hand ended before the waiting player's turn")
		}
		callAmount := game.ProspectiveCallAmountForActor(acting)
		if _, err := actor.applyActAndCommit(context.Background(), ActCmd{
			PlayerID: acting, ActionID: "advance-" + acting, Action: betting.ActionCall, Amount: callAmount,
		}); err != nil {
			t.Fatalf("advance turn: %v", err)
		}
	}

	actor.processInlinePreselections(context.Background())

	for _, p := range game.PlayersForActor() {
		if p.ID == waitingPlayer {
			if p.State != hand.Folded {
				t.Fatalf("check_fold should still fold when the facing amount never changed, got state=%v", p.State)
			}
			return
		}
	}
	t.Fatal("waiting player not found")
}

func TestHandlePreselectAcceptsAllIn(t *testing.T) {
	actor, game, current := newTimeBankActor(t)
	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: current, ActionID: "preselect-all-in", Selection: "all_in",
		ExpectedHandID: actor.handID, ExpectedStage: game.ViewFor("").Stage,
	}); err != nil {
		t.Fatalf("all_in should be a valid selection: %v", err)
	}
	if got := actor.activity.Preselections[current].Selection; got != "all_in" {
		t.Fatalf("persisted selection=%q, want all_in", got)
	}

	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: current, ActionID: "preselect-bogus", Selection: "bogus",
		ExpectedHandID: actor.handID, ExpectedStage: game.ViewFor("").Stage,
	}); err == nil {
		t.Fatal("an unrecognized selection must still be rejected")
	}
}

func TestAllInPreselectionExecutesAsRaiseForFullStack(t *testing.T) {
	actor, game, current := newTimeBankActor(t)
	actor.version = 1
	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: current, ActionID: "preselect-all-in", Selection: "all_in",
		ExpectedSnapshotVersion: 1, ExpectedHandID: "hand-1",
	}); err != nil {
		t.Fatalf("set preselect all_in: %v", err)
	}

	actor.processInlinePreselections(context.Background())

	if !game.PlayerAllInForActor(current) {
		t.Fatal("preselected all_in did not push the player all-in")
	}
	if _, ok := actor.activity.Preselections[current]; ok {
		t.Fatal("all_in preselection should be consumed once executed")
	}
}

// A player whose total (contributed + remaining stack) no longer covers the
// bet facing them must go all-in via a call, not a raise past their own
// stack — mirroring NormalizedActionForActor's existing downgrade rule.
func TestAllInPreselectionDowngradesToCallWhenStackIsShort(t *testing.T) {
	// Heads-up so whichever player StartHand's random dealer pick makes act
	// first (the only one who can raise before "short" ever gets a turn) is
	// deterministic once found — retry until "short" isn't dealt that seat.
	var game *hand.Table
	for attempt := 0; ; attempt++ {
		g := hand.NewTable([]*hand.Player{
			{ID: "big", Stack: 1000, Ready: true},
			{ID: "short", Stack: 100, Ready: true},
		}, 10, 20)
		if err := g.StartHand(); err != nil {
			t.Fatal(err)
		}
		if g.CurrentPlayerIDForActor() != "short" {
			game = g
			break
		}
		if attempt > 20 {
			t.Fatal("test setup: short kept being dealt the first-to-act seat")
		}
	}

	actor := New("preselection-allin-short", nil, true, nil)
	actor.cached = game
	actor.handID = "hand-1"
	actor.version = 1

	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: "short", ActionID: "preselect-all-in", Selection: "all_in",
		ExpectedSnapshotVersion: 1, ExpectedHandID: "hand-1",
	}); err != nil {
		t.Fatalf("set preselect all_in: %v", err)
	}

	if _, err := actor.applyActAndCommit(context.Background(), ActCmd{
		PlayerID: "big", ActionID: "raise-1", Action: betting.ActionRaise, Amount: 500,
	}); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if game.CurrentPlayerIDForActor() != "short" {
		t.Fatal("test setup did not reach short's turn")
	}
	if callAmount := game.ProspectiveCallAmountForActor("short"); callAmount <= 100 {
		t.Fatalf("test setup: short's call amount %d does not exceed their stack", callAmount)
	}

	actor.processInlinePreselections(context.Background())

	if !game.PlayerAllInForActor("short") {
		t.Fatal("short did not go all-in on the downgraded call")
	}
	if _, ok := actor.activity.Preselections["short"]; ok {
		t.Fatal("all_in preselection should be consumed once executed")
	}
}

func TestAllInPreselectionUnaffectedByAnotherRaise(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}

	actor := New("preselection-allin-noprune", nil, true, nil)
	actor.cached = game
	actor.handID = "hand-1"
	actor.version = 1

	current := game.CurrentPlayerIDForActor()
	var waiting string
	for _, id := range []string{"p1", "p2", "p3"} {
		if id != current {
			waiting = id
			break
		}
	}

	if err := actor.handlePreselect(context.Background(), PreselectCmd{
		PlayerID: waiting, ActionID: "preselect-all-in", Selection: "all_in",
		ExpectedSnapshotVersion: 1, ExpectedHandID: "hand-1",
	}); err != nil {
		t.Fatalf("set preselect all_in: %v", err)
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

	if _, ok := actor.activity.Preselections[waiting]; !ok {
		t.Fatal("all_in preselection must survive another player's raise, unlike a fixed call")
	}
}
