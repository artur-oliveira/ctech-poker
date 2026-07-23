//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func containsAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

// otherPlayer picks whichever of a/b isn't current — the dealer (and so who
// acts first each street) is drawn randomly by StartHand, so the test can't
// hardcode seat order.
func otherPlayer(current, a, b string) string {
	if current == a {
		return b
	}
	return a
}

// actionIDSeq hands out unique action_ids: ActIdempotent de-dupes by this
// value alone (not per-player), so every dispatched action needs its own.
func actionIDSeq() func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("full-hand-action-%d", n)
	}
}

// TestFullHandJoinBetRaiseFold drives one complete heads-up hand through the
// same Actor commands the WS gateway dispatches (tablews.go): join, ready
// (which starts the hand once both are ready), call, check, raise (both the
// opening bet and a re-raise), and fold — verifying at every decision point
// that it's genuinely that player's turn with that action legally available,
// i.e. both seats are actually able to act, not just one. Buy-in/wallet is
// out of scope here (mocked by JoinCmd's plain Stack field, no walletclient
// involved) per api/CLAUDE.md's "engine logic is unit-tested" split — this
// exercises the actor/engine wiring, not chip-accounting edge cases.
func TestFullHandJoinBetRaiseFold(t *testing.T) {
	backend := cache.NewMemoryBackend(16)
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")

	mgr := tablemanager.NewManager(tablelease.NewService(backend), store, nil, nil)
	seed := func() *hand.Table { return hand.NewTable(nil, 10, 20) }
	actor, err := mgr.GetOrCreateActor(context.Background(), "table-full-hand", seed)
	if err != nil {
		t.Fatalf("acquire actor: %v", err)
	}
	nextActionID := actionIDSeq()

	join := func(playerID string, stack int64) {
		t.Helper()
		reply := make(chan error, 1)
		if err := actor.Dispatch(table.JoinCmd{PlayerID: playerID, Stack: stack, Reply: reply}); err != nil {
			t.Fatalf("join %s: %v", playerID, err)
		}
	}
	ready := func(playerID string) {
		t.Helper()
		reply := make(chan error, 1)
		if err := actor.Dispatch(table.ReadyCmd{PlayerID: playerID, Ready: true, Reply: reply}); err != nil {
			t.Fatalf("ready %s: %v", playerID, err)
		}
	}
	act := func(playerID string, action betting.Action, amount int64) {
		t.Helper()
		reply := make(chan error, 1)
		cmd := table.ActCmd{PlayerID: playerID, ActionID: nextActionID(), Action: action, Amount: amount, Reply: reply}
		if err := actor.Dispatch(cmd); err != nil {
			t.Fatalf("%s %s %d: %v", playerID, action, amount, err)
		}
	}

	join("p1", 1000)
	join("p2", 1000)
	ready("p1")
	ready("p2")

	view := actor.TableForTest().ViewFor("p1")
	if view.Stage != "pre_flop" {
		t.Fatalf("expected the hand to start pre_flop once both players readied, got %s", view.Stage)
	}

	// Pre-flop: first-to-act (the dealer, heads-up) owes the blind
	// difference and calls; the other seat then has the check option and
	// closes the street.
	preflopActor := view.CurrentPlayerID
	bigBlind := otherPlayer(preflopActor, "p1", "p2")
	preflopLegal := actor.TableForTest().ViewFor(preflopActor).LegalActions
	if !containsAction(preflopLegal.Actions, "call") {
		t.Fatalf("expected %s to owe a call pre-flop, got %+v", preflopActor, preflopLegal.Actions)
	}
	act(preflopActor, betting.ActionCall, 0)

	bbLegal := actor.TableForTest().ViewFor(bigBlind).LegalActions
	if !containsAction(bbLegal.Actions, "check") {
		t.Fatalf("expected %s (big blind) to have check available after the limp, got %+v", bigBlind, bbLegal.Actions)
	}
	act(bigBlind, betting.ActionCheck, 0)

	flopView := actor.TableForTest().ViewFor("p1")
	if flopView.Stage != "flop" {
		t.Fatalf("expected the flop once pre-flop action closed, got %s", flopView.Stage)
	}

	// Flop: whoever acts first opens the betting (a "raise" from 0 is this
	// engine's only bet action, see legalActionsFor), the other seat
	// re-raises over it, and the original bettor folds to the re-raise.
	bettor := flopView.CurrentPlayerID
	raiser := otherPlayer(bettor, "p1", "p2")

	bettorLegal := actor.TableForTest().ViewFor(bettor).LegalActions
	if !containsAction(bettorLegal.Actions, "raise") {
		t.Fatalf("expected %s to be able to open the flop betting, got %+v", bettor, bettorLegal.Actions)
	}
	act(bettor, betting.ActionRaise, bettorLegal.MinRaiseTo+20)

	raiserLegal := actor.TableForTest().ViewFor(raiser).LegalActions
	if !containsAction(raiserLegal.Actions, "raise") {
		t.Fatalf("expected %s to be able to raise over %s's bet, got %+v", raiser, bettor, raiserLegal.Actions)
	}
	act(raiser, betting.ActionRaise, raiserLegal.MinRaiseTo)

	bettorFacingRaise := actor.TableForTest().ViewFor(bettor).LegalActions
	if !containsAction(bettorFacingRaise.Actions, "fold") {
		t.Fatalf("expected %s to be able to fold facing the re-raise, got %+v", bettor, bettorFacingRaise.Actions)
	}
	act(bettor, betting.ActionFold, 0)

	final := actor.TableForTest().ViewFor(raiser)
	if final.Stage != "complete" {
		t.Fatalf("expected the hand to end once the last opponent folded, got %s", final.Stage)
	}
	if final.Payouts[raiser] <= 0 {
		t.Fatalf("expected %s to be awarded the pot after %s folded, got payouts %+v", raiser, bettor, final.Payouts)
	}
}
