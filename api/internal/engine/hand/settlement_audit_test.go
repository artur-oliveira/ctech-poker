package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

// Focused settlement-audit regression tests for issue #121:
//
//	(a) multi-way all-in with 3+ distinct all-in amounts + a folded partial
//	    bet builds the side-pot layers correctly through the real Act path
//	    (not just sidepots.ComputeSidePots' own unit tests);
//	(b) run-it-twice halves each layer's pot per board, keeps the halving odd
//	    chip on board one, and does it for a side pot as well as the main pot;
//	(c) the odd chip on a chopped pot goes to the first live seat left of the
//	    button, exercised through a genuine showdown award (not just the
//	    oddChipWinner unit test);
//	(d) the runShowdown "len(winners) == 0" branch never refunds folded/dead
//	    money to its contributors — it rolls to a live winner.

// (a) -----------------------------------------------------------------------

// TestMultiWayAllInThreeDistinctAmountsWithFoldedPartialBuildsSidePots pins a
// five-handed hand where three players are all-in for three distinct amounts
// (100 / 250 / 500), a fourth (D) builds the pot far past them and a fifth (E)
// puts 1300 in and then folds on the turn. E's 1300 is a folded *partial*
// contribution: it never carves its own side pot, it is absorbed as dead
// money into the layers the live players contest. D is rigged with quad aces
// and wins every layer, so the whole 4950-chip pot (including E's dead money)
// is paid out and nothing is refunded to E.
func TestMultiWayAllInThreeDistinctAmountsWithFoldedPartialBuildsSidePots(t *testing.T) {
	A := &Player{ID: "A", Stack: 100, Ready: true}
	B := &Player{ID: "B", Stack: 250, Ready: true}
	C := &Player{ID: "C", Stack: 500, Ready: true}
	D := &Player{ID: "D", Stack: 3000, Ready: true}
	E := &Player{ID: "E", Stack: 3000, Ready: true}
	table := NewTable([]*Player{A, B, C, D, E}, 10, 20)
	// Button on C (index 2): SB=D, BB=E, first to act pre-flop = A.
	table.dealerSeat = 2
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Rig D's quad aces deterministically. 5 players => 10 hole cards
	// (indices 0-9); flop burns 10 and deals 11/12/13, turn burns 14 deals
	// 15, river burns 16 deals 17.
	A.HoleCards = [2]deck.Card{{Rank: deck.Five, Suit: deck.Clubs}, {Rank: deck.Six, Suit: deck.Clubs}}
	B.HoleCards = [2]deck.Card{{Rank: deck.Seven, Suit: deck.Diamonds}, {Rank: deck.Eight, Suit: deck.Diamonds}}
	C.HoleCards = [2]deck.Card{{Rank: deck.Nine, Suit: deck.Hearts}, {Rank: deck.Ten, Suit: deck.Hearts}}
	D.HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}
	E.HoleCards = [2]deck.Card{{Rank: deck.Two, Suit: deck.Hearts}, {Rank: deck.Two, Suit: deck.Diamonds}}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[12] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[13] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[15] = deck.Card{Rank: deck.Three, Suit: deck.Diamonds}
	table.shuffle.Cards[17] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	mustAct(t, table, "A", betting.ActionRaise, 100)  // all-in, level 1
	mustAct(t, table, "B", betting.ActionRaise, 250)  // all-in, level 2
	mustAct(t, table, "C", betting.ActionRaise, 500)  // all-in, level 3
	mustAct(t, table, "D", betting.ActionCall, 500)   //
	mustAct(t, table, "E", betting.ActionCall, 500)   //
	if table.Stage() != Flop {
		t.Fatalf("expected Flop once D and E matched C's all-in, got %v", table.Stage())
	}
	mustAct(t, table, "D", betting.ActionBet, 800) // D builds past every all-in
	mustAct(t, table, "E", betting.ActionCall, 800)
	if table.Stage() != Turn {
		t.Fatalf("expected Turn, got %v", table.Stage())
	}
	mustAct(t, table, "D", betting.ActionBet, 1500)
	mustAct(t, table, "E", betting.ActionFold, 0) // E's 1300 is now folded dead money

	drainRunout(t, table)

	// Contributions: A 100, B 250, C 500, D 2800, E 1300 => 4950 total.
	// Layers: [0,100]=500, [100,250]=600, [250,500]=750, [500,2800]=3100
	// (the last one folds in E's 800 dead money above 500). Four contested
	// layers, no uncalled/refund layer.
	outcome := table.LastOutcomeForActor()
	if len(outcome.PotResults) != 4 {
		t.Fatalf("expected 4 contested side-pot layers, got %d: %+v", len(outcome.PotResults), outcome.PotResults)
	}
	for i, pr := range outcome.PotResults {
		if pr.Refund {
			t.Fatalf("layer %d must not be a refund layer — E's partial bet is dead money, not uncalled: %+v", i, pr)
		}
		if len(pr.Winners) != 1 || pr.Winners[0] != "D" {
			t.Fatalf("layer %d must be won by D's quad aces, got winners %v", i, pr.Winners)
		}
	}
	wantAmts := []int64{500, 600, 750, 3100}
	for i, pr := range outcome.PotResults {
		if pr.Amount != wantAmts[i] {
			t.Fatalf("layer %d amount = %d, want %d", i, pr.Amount, wantAmts[i])
		}
	}

	payouts := table.Payouts()
	if payouts["E"] != 0 {
		t.Fatalf("E folded — their 1300 is dead money, never refunded, got %d", payouts["E"])
	}
	if payouts["A"] != 0 || payouts["B"] != 0 || payouts["C"] != 0 {
		t.Fatalf("A/B/C all lost to D's quads, got %+v", payouts)
	}
	if payouts["D"] != 4950 {
		t.Fatalf("D must win every layer including E's dead money = 4950, got %d", payouts["D"])
	}
	var total int64
	for _, v := range payouts {
		total += v
	}
	if total != 4950 {
		t.Fatalf("chips must be conserved: total payout %d, want 4950", total)
	}
}

// (b) -----------------------------------------------------------------------

// TestRunItTwiceHalvesMainAndSidePotPerBoardOddChipToBoardOne pins a
// three-handed pre-flop all-in with a main pot AND a side pot, run it twice.
// Every layer's (post-rake) pot is halved between the two boards; the odd
// chip from halving an odd layer belongs to board one. B is rigged to win
// board one outright and C to win board two outright, so each of the four
// (2 layers x 2 boards) sub-pots is awarded independently and the four
// credits recombine to the full pot.
func TestRunItTwiceHalvesMainAndSidePotPerBoardOddChipToBoardOne(t *testing.T) {
	A := &Player{ID: "A", Stack: 201, Ready: true, RunItTwice: true}
	B := &Player{ID: "B", Stack: 603, Ready: true, RunItTwice: true}
	C := &Player{ID: "C", Stack: 603, Ready: true, RunItTwice: true}
	table := NewTable([]*Player{A, B, C}, 10, 20)
	table.ConfigureRunItTwice(true)
	table.dealerSeat = 0 // A button, B SB, C BB, first to act = A
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// All-in pre-flop => boardSplitAt = 0, the two boards are fully
	// independent. 3 players => 6 hole cards (0-5). Board one deals
	// 7/8/9 (flop), 11 (turn), 13 (river). Board two deals 15/16/17
	// (flop), 19 (turn), 21 (river).
	A.HoleCards = [2]deck.Card{{Rank: deck.Two, Suit: deck.Clubs}, {Rank: deck.Two, Suit: deck.Spades}}
	B.HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.Ace, Suit: deck.Diamonds}}
	C.HoleCards = [2]deck.Card{{Rank: deck.King, Suit: deck.Clubs}, {Rank: deck.King, Suit: deck.Spades}}
	// Board one: an ace on the flop => B has trip aces, unbeatable here.
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Ace, Suit: deck.Spades}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Nine, Suit: deck.Hearts}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Four, Suit: deck.Diamonds}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Seven, Suit: deck.Clubs}
	table.shuffle.Cards[13] = deck.Card{Rank: deck.Three, Suit: deck.Hearts}
	// Board two: two more kings => C has quad kings, unbeatable there.
	table.shuffle.Cards[15] = deck.Card{Rank: deck.King, Suit: deck.Diamonds}
	table.shuffle.Cards[16] = deck.Card{Rank: deck.King, Suit: deck.Hearts}
	table.shuffle.Cards[17] = deck.Card{Rank: deck.Five, Suit: deck.Clubs}
	table.shuffle.Cards[19] = deck.Card{Rank: deck.Six, Suit: deck.Spades}
	table.shuffle.Cards[21] = deck.Card{Rank: deck.Nine, Suit: deck.Diamonds}

	mustAct(t, table, "A", betting.ActionRaise, 201) // all-in
	mustAct(t, table, "B", betting.ActionRaise, 603) // all-in
	mustAct(t, table, "C", betting.ActionCall, 603)  // all-in

	drainRunout(t, table)

	outcome := table.LastOutcomeForActor()
	if len(outcome.BoardTwo) != 5 {
		t.Fatalf("expected a full second board, got %v", outcome.BoardTwo)
	}
	// Main pot: 201*3 = 603 (odd). Side pot: 402*2 = 804.
	// Main board one = 603/2 + 603%2 = 302; board two = 301.
	// Side board one = 402; board two = 402.
	if len(outcome.PotResults) != 4 {
		t.Fatalf("expected 2 layers x 2 runouts = 4 pot results, got %d: %+v", len(outcome.PotResults), outcome.PotResults)
	}
	var mainB1, mainB2, sideB1, sideB2 int64
	for _, pr := range outcome.PotResults {
		switch {
		case pr.Amount == 603 && pr.Runout == 1:
			mainB1 = pr.PayoutAmount
		case pr.Amount == 603 && pr.Runout == 2:
			mainB2 = pr.PayoutAmount
		case pr.Amount == 804 && pr.Runout == 1:
			sideB1 = pr.PayoutAmount
		case pr.Amount == 804 && pr.Runout == 2:
			sideB2 = pr.PayoutAmount
		default:
			t.Fatalf("unexpected pot result %+v", pr)
		}
	}
	if mainB1 != 302 || mainB2 != 301 {
		t.Fatalf("main pot must halve 302/301 with the odd chip on board one, got %d/%d", mainB1, mainB2)
	}
	if sideB1 != 402 || sideB2 != 402 {
		t.Fatalf("side pot must halve evenly 402/402, got %d/%d", sideB1, sideB2)
	}

	payouts := table.Payouts()
	if payouts["A"] != 0 {
		t.Fatalf("A was covered by both boards' losers, got %d", payouts["A"])
	}
	if payouts["B"] != 302+402 {
		t.Fatalf("B wins board one's main+side = 704, got %d", payouts["B"])
	}
	if payouts["C"] != 301+402 {
		t.Fatalf("C wins board two's main+side = 703, got %d", payouts["C"])
	}
	var total int64
	for _, v := range payouts {
		total += v
	}
	if total != 201+603+603 {
		t.Fatalf("chips must be conserved across both runouts: total %d, want 1407", total)
	}
}

// (c) -----------------------------------------------------------------------

// TestOddChipOnChoppedPotGoesToFirstSeatLeftOfButton runs a genuine
// three-handed showdown where B and C chop the pot (both play the same
// straight off the board) and A loses. The 45-chip pot does not divide
// evenly; the odd chip must go to the first live seat clockwise from the
// button — B here (button is A) — matching OVERVIEW.md's rule and
// oddChipWinner's traversal.
func TestOddChipOnChoppedPotGoesToFirstSeatLeftOfButton(t *testing.T) {
	A := &Player{ID: "A", Stack: 15, Ready: true}
	B := &Player{ID: "B", Stack: 15, Ready: true}
	C := &Player{ID: "C", Stack: 15, Ready: true}
	table := NewTable([]*Player{A, B, C}, 2, 4)
	table.dealerSeat = 0 // A button; B is the first seat left of the button
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Board makes a ten-high straight for anyone holding a Ten; B and C both
	// do, A does not. 3 players => 6 hole cards; flop 7/8/9, turn 11,
	// river 13.
	A.HoleCards = [2]deck.Card{{Rank: deck.Two, Suit: deck.Diamonds}, {Rank: deck.Three, Suit: deck.Diamonds}}
	B.HoleCards = [2]deck.Card{{Rank: deck.Ten, Suit: deck.Clubs}, {Rank: deck.Four, Suit: deck.Spades}}
	C.HoleCards = [2]deck.Card{{Rank: deck.Ten, Suit: deck.Diamonds}, {Rank: deck.Five, Suit: deck.Spades}}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Nine, Suit: deck.Spades}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Eight, Suit: deck.Diamonds}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Seven, Suit: deck.Clubs}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Six, Suit: deck.Hearts}
	table.shuffle.Cards[13] = deck.Card{Rank: deck.Queen, Suit: deck.Spades}

	mustAct(t, table, "A", betting.ActionRaise, 15) // all-in
	mustAct(t, table, "B", betting.ActionCall, 15)  // all-in
	mustAct(t, table, "C", betting.ActionCall, 15)  // all-in
	drainRunout(t, table)

	payouts := table.Payouts()
	if payouts["A"] != 0 {
		t.Fatalf("A cannot win — no Ten, no straight; got %d", payouts["A"])
	}
	if payouts["B"] != 23 || payouts["C"] != 22 {
		t.Fatalf("B and C chop 45: the odd chip goes to B (first seat left of the button), want B=23 C=22, got B=%d C=%d", payouts["B"], payouts["C"])
	}
	outcome := table.LastOutcomeForActor()
	if len(outcome.PotResults) != 1 {
		t.Fatalf("expected a single chopped layer, got %+v", outcome.PotResults)
	}
	pr := outcome.PotResults[0]
	if pr.Payouts["B"] != 23 || pr.Payouts["C"] != 22 {
		t.Fatalf("pot result must record the odd chip on B, got %+v", pr.Payouts)
	}
}

// (d) -----------------------------------------------------------------------

// TestWinnerlessLayerRollsToLiveWinnerNeverRefunded exercises runShowdown's
// "len(winners) == 0" defensive branch directly. Through the public API this
// is unreachable: sidepots.ComputeSidePots never puts a folded player in a
// contested layer's Eligible, and RemovePlayerForActor refuses to remove
// anyone still in t.handOrder for the whole hand. This test manufactures the
// state anyway by splicing the side pot's only two eligible players out of
// t.players (leaving them in t.handOrder) so their layer resolves with zero
// winners — and asserts the fix: those chips are never refunded to the
// (removed) contributors, they roll to the hand's live winner.
func TestWinnerlessLayerRollsToLiveWinnerNeverRefunded(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 700, State: Active, Contributed: 300}
	p2 := &Player{ID: "p2", Stack: 700, State: Active, Contributed: 300}
	p3 := &Player{ID: "p3", Stack: 0, State: AllIn, Contributed: 100}
	table := NewTable([]*Player{p1, p2, p3}, 10, 20)
	table.handOrder = []*Player{p1, p2, p3}
	table.dealerSeat = 0
	table.stage = River
	table.board = []deck.Card{
		{Rank: deck.King, Suit: deck.Clubs}, {Rank: deck.Queen, Suit: deck.Clubs},
		{Rank: deck.Jack, Suit: deck.Diamonds}, {Rank: deck.Three, Suit: deck.Spades},
		{Rank: deck.Nine, Suit: deck.Hearts},
	}
	// p3's pair of aces takes the main pot outright vs the others' air.
	p3.HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}
	p1.HoleCards = [2]deck.Card{{Rank: deck.Two, Suit: deck.Diamonds}, {Rank: deck.Four, Suit: deck.Diamonds}}
	p2.HoleCards = [2]deck.Card{{Rank: deck.Six, Suit: deck.Diamonds}, {Rank: deck.Eight, Suit: deck.Diamonds}}

	// Manufacture the otherwise-impossible state.
	table.players = []*Player{p3}

	table.runShowdown()

	payouts := table.Payouts()
	if payouts["p1"] != 0 || payouts["p2"] != 0 {
		t.Fatalf("removed contributors must never be refunded dead money, got %+v", payouts)
	}
	if payouts["p3"] != 700 {
		t.Fatalf("p3 must receive the main pot (300) plus the winnerless 400-chip side pot rolled to them = 700, got %+v", payouts)
	}
	if p3.Stack != 700 {
		t.Fatalf("p3's stack must be credited the full 700, got %d", p3.Stack)
	}
}

// helpers ------------------------------------------------------------------

func mustAct(t *testing.T, table *Table, id string, action betting.Action, amount int64) {
	t.Helper()
	if err := table.Act(id, action, amount); err != nil {
		t.Fatalf("%s %v %d: %v", id, action, amount, err)
	}
}

func drainRunout(t *testing.T, table *Table) {
	t.Helper()
	for i := 0; i < 12 && table.Stage() != Complete; i++ {
		table.AdvanceRunoutStreetForActor()
	}
	if table.Stage() != Complete {
		t.Fatalf("hand did not reach Complete, stuck at %v", table.Stage())
	}
}
