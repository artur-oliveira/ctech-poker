package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

// The three tests below all come from one production incident (table
// 01M2QYH151BYZJEYF3BAWZWYQK, hand 01M2R0WT7EXVB8TB4GF8W9JWB5, 2026-09-17 —
// docs/specs/2026-09-17-frozen-table-runout-and-sitout-fold.md): the small
// blind paused mid-hand while another seat was on the clock, which folded her
// out of the live betting round from off-turn and then lost that fold to a
// SittingOut overwrite on her very next command.

// TestSitOutOffTurnDoesNotFoldOrMoveTheActionAnchor pins the first half:
// betting.Round.Act has no turn-order check, so folding any Active player
// from SitOutForActor also moved Round.LastActorID — and actionScanOrder
// anchors the next decision at the seat after the last actor, so the scan
// jumped past two seats that had not acted yet (production frames show
// current_player_id going straight from UTG to the big blind).
func TestSitOutOffTurnDoesNotFoldOrMoveTheActionAnchor(t *testing.T) {
	players := []*Player{
		{ID: "utg", Stack: 1000, Ready: true},
		{ID: "button", Stack: 1000, Ready: true},
		{ID: "sb", Stack: 1000, Ready: true},
		{ID: "bb", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerSeat = 1
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if current := table.CurrentPlayerIDForActor(); current != "utg" {
		t.Fatalf("expected utg first to act pre-flop, got %q", current)
	}

	// The small blind pauses while utg is on the clock.
	table.SitOutForActor("sb")

	if sb := table.playerByID("sb"); sb.State != SittingOut {
		t.Fatalf("expected the off-turn sit-out to leave the seat SittingOut, got %v", sb.State)
	}
	if idx, ok := table.roundIdx["sb"]; ok && table.round.Players[idx].Folded {
		t.Fatal("a player who is not on the clock must not be folded out of the live betting round")
	}
	if table.round.LastActorID != "" {
		t.Fatalf("an off-turn sit-out must not move the action anchor, LastActorID = %q", table.round.LastActorID)
	}
	if current := table.CurrentPlayerIDForActor(); current != "utg" {
		t.Fatalf("expected utg still on the clock after another seat paused, got %q", current)
	}

	// The round still owes the paused seat a decision; the actor's auto-fold
	// sweep is what takes it, and only once it is actually their turn.
	if table.CurrentPlayerShouldAutoFoldForActor() {
		t.Fatal("utg is on the clock and has not paused — nothing to auto-fold yet")
	}
	if err := table.Act("utg", betting.ActionFold, 0); err != nil {
		t.Fatalf("utg folds: %v", err)
	}
	if err := table.Act("button", betting.ActionFold, 0); err != nil {
		t.Fatalf("button folds: %v", err)
	}
	if current := table.CurrentPlayerIDForActor(); current != "sb" {
		t.Fatalf("expected the paused small blind on the clock, got %q", current)
	}
	if !table.CurrentPlayerShouldAutoFoldForActor() {
		t.Fatal("a paused seat that reaches its own turn must be auto-folded by the sweep")
	}
	// That is the sweep's fold, now legitimately on-turn. A seat paused
	// earlier in the hand is SittingOut, not Active, so this is also what
	// pins SitOutForActor gating the fold on "is on the clock" rather than on
	// the seat's own state.
	table.SitOutForActor("sb")
	if sb := table.playerByID("sb"); sb.State != Folded {
		t.Fatalf("expected the paused seat folded once on the clock, got %v", sb.State)
	}
	if table.Stage() != Complete {
		t.Fatalf("expected the hand to end uncontested for the big blind, stage is %v", table.Stage())
	}
	if payout := table.Payouts()["bb"]; payout == 0 {
		t.Fatalf("expected the big blind paid the uncontested pot, payouts = %+v", table.Payouts())
	}
}

// TestPauseOrExitNeverResurrectsAFoldedPlayer pins the second half: a fold
// already made in the live hand outlived neither RequestExit nor
// SitOutForActor, both of which overwrote State with SittingOut. runShowdown
// reads State==Folded to decide whose chips are dead money, so the folded
// player was dealt back into the showdown and paid a side pot built from the
// very chips they had folded.
func TestPauseOrExitNeverResurrectsAFoldedPlayer(t *testing.T) {
	players := []*Player{
		{ID: "utg", Stack: 1000, Ready: true},
		{ID: "sb", Stack: 1000, Ready: true},
		{ID: "bb", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Rig the showdown so the folded seat would win outright if it were ever
	// treated as live: quad aces against two pair and one pair.
	table.playerByID("utg").HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}
	table.playerByID("sb").HoleCards = [2]deck.Card{{Rank: deck.Two, Suit: deck.Clubs}, {Rank: deck.Three, Suit: deck.Diamonds}}
	table.playerByID("bb").HoleCards = [2]deck.Card{{Rank: deck.Seven, Suit: deck.Hearts}, {Rank: deck.Eight, Suit: deck.Hearts}}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Three, Suit: deck.Spades}
	table.shuffle.Cards[13] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	if err := table.Act("utg", betting.ActionFold, 0); err != nil {
		t.Fatalf("utg folds: %v", err)
	}
	// Everything the paused/exiting seat can do afterwards must leave that
	// fold alone for the rest of the hand.
	if err := table.RequestExit("utg"); err != nil {
		t.Fatalf("RequestExit: %v", err)
	}
	if state := table.playerByID("utg").State; state != Folded {
		t.Fatalf("request_exit must not resurrect a folded seat, got %v", state)
	}
	table.SitOutForActor("utg")
	if state := table.playerByID("utg").State; state != Folded {
		t.Fatalf("sit-out must not resurrect a folded seat, got %v", state)
	}
	if err := table.CancelExit("utg"); err != nil {
		t.Fatalf("CancelExit: %v", err)
	}
	if state := table.playerByID("utg").State; state != Folded {
		t.Fatalf("cancel_exit must not resurrect a folded seat, got %v", state)
	}

	// Check the hand down to a real showdown.
	if err := table.Act("sb", betting.ActionCall, 20); err != nil {
		t.Fatalf("sb calls: %v", err)
	}
	if err := table.Act("bb", betting.ActionCheck, 0); err != nil {
		t.Fatalf("bb checks: %v", err)
	}
	for _, street := range []string{"flop", "turn", "river"} {
		if err := table.Act("sb", betting.ActionCheck, 0); err != nil {
			t.Fatalf("sb checks %s: %v", street, err)
		}
		if err := table.Act("bb", betting.ActionCheck, 0); err != nil {
			t.Fatalf("bb checks %s: %v", street, err)
		}
	}
	if table.Stage() != Complete {
		t.Fatalf("expected Complete after checking down, got %v", table.Stage())
	}
	if payout, ok := table.Payouts()["utg"]; ok && payout != 0 {
		t.Fatalf("folded chips are dead money — the folded seat was paid %d (payouts = %+v)", payout, table.Payouts())
	}
	if table.Payouts()["sb"] == 0 {
		t.Fatalf("expected the best live hand paid, payouts = %+v", table.Payouts())
	}
}

// TestPausedSeatDoesNotForceAShowdownOnAnUndealtBoard replays the incident's
// exact shape: the paused small blind is folded out by the sweep, the big
// blind shoves, one seat folds and the button calls the all-in. The hand must
// then be awaiting a paced runout — two streets still owed — with nobody on
// the clock, and the seat that called the all-in must never be foldable out
// of that contested pot.
func TestPausedSeatDoesNotForceAShowdownOnAnUndealtBoard(t *testing.T) {
	players := []*Player{
		{ID: "utg", Stack: 529625, Ready: true},
		{ID: "button", Stack: 75000, Ready: true},
		{ID: "sb", Stack: 95900, Ready: true},
		{ID: "bb", Stack: 10275, Ready: true},
	}
	table := NewTable(players, 500, 1000)
	table.dealerSeat = 1
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	table.SitOutForActor("sb") // pauses off-turn: no fold, no anchor move
	if err := table.RequestExit("sb"); err != nil {
		t.Fatalf("RequestExit: %v", err)
	}
	if err := table.Act("utg", betting.ActionFold, 0); err != nil {
		t.Fatalf("utg folds: %v", err)
	}
	if err := table.Act("button", betting.ActionCall, 1000); err != nil {
		t.Fatalf("button calls the big blind: %v", err)
	}
	if current := table.CurrentPlayerIDForActor(); current != "sb" {
		t.Fatalf("expected the paused small blind on the clock, got %q", current)
	}
	if !table.CurrentPlayerShouldAutoFoldForActor() {
		t.Fatal("the paused, exiting seat must be auto-folded on its own turn")
	}
	table.SitOutForActor("sb") // the sweep's fold, now legitimately on-turn
	if state := table.playerByID("sb").State; state != Folded {
		t.Fatalf("expected the paused seat folded once the action reached it, got %v", state)
	}
	if err := table.Act("bb", betting.ActionRaise, 10275); err != nil {
		t.Fatalf("bb shoves all-in: %v", err)
	}
	if current := table.CurrentPlayerIDForActor(); current != "button" {
		t.Fatalf("the shove must reopen action for the caller, got %q", current)
	}
	if err := table.Act("button", betting.ActionCall, 10275); err != nil {
		t.Fatalf("button calls the all-in: %v", err)
	}

	if table.Stage() != Flop {
		t.Fatalf("expected the flop dealt inside the call, got stage %v", table.Stage())
	}
	if current := table.CurrentPlayerIDForActor(); current != "" {
		t.Fatalf("no betting round can complete again, yet %q is on the clock", current)
	}
	if !table.IsAwaitingRunoutForActor() {
		t.Fatal("turn and river are still owed — the table must report an awaiting runout")
	}
	// The caller closed the action: nothing may fold them out of a contested
	// pot, which is what ended the real hand on a three-card board.
	table.SitOutForActor("button")
	if state := table.playerByID("button").State; state == Folded {
		t.Fatal("a seat that already called an all-in must not be folded out of the pot")
	}
	if table.Stage() != Flop || !table.IsAwaitingRunoutForActor() {
		t.Fatalf("pausing the all-in caller must not resolve the hand: stage %v, awaiting %v",
			table.Stage(), table.IsAwaitingRunoutForActor())
	}

	table.AdvanceRunoutStreetForActor()
	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Complete {
		t.Fatalf("expected the paced runout to reach Complete, got %v", table.Stage())
	}
	if len(table.board) != 5 {
		t.Fatalf("a contested all-in must see a full board, got %d cards", len(table.board))
	}
	if payout, ok := table.Payouts()["sb"]; ok && payout != 0 {
		t.Fatalf("the paused seat folded pre-flop and must be paid nothing, got %d", payout)
	}
}
