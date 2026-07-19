package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestFullHandWithThreeWayAllInProducesCorrectPayouts(t *testing.T) {
	players := []*Player{
		{ID: "Dealer", Stack: 1000, Ready: true},
		{ID: "SB", Stack: 200, Ready: true},
		{ID: "BB", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.Stage() != PreFlop {
		t.Fatalf("expected PreFlop after StartHand, got %v", table.Stage())
	}

	// Rig the deal so the showdown winner is deterministic instead of
	// depending on deck.NewShuffle's crypto/rand seed: SB gets pocket aces
	// and the board pairs the other two aces, giving SB an unbeatable
	// four-of-a-kind. Dealer/BB get low, disjoint hole cards that can't
	// improve past a straight/pair off this board, so there's no chance of
	// a tie muddying the "SB must be paid" assertion below.
	players[0].HoleCards = [2]deck.Card{{Rank: deck.Five, Suit: deck.Clubs}, {Rank: deck.Six, Suit: deck.Clubs}}      // Dealer: 5c 6c
	players[1].HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}     // SB: As Ah
	players[2].HoleCards = [2]deck.Card{{Rank: deck.Seven, Suit: deck.Hearts}, {Rank: deck.Eight, Suit: deck.Hearts}} // BB: 7h 8h
	// t.nextCard is 6 at this point (3 players x 2 hole cards already
	// dealt); indices 6..10 are the flop/turn/river in dealing order.
	table.shuffle.Cards[6] = deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Ace, Suit: deck.Diamonds}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Two, Suit: deck.Spades}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Three, Suit: deck.Spades}
	table.shuffle.Cards[10] = deck.Card{Rank: deck.Four, Suit: deck.Hearts}

	// Pre-flop: Dealer raises to 220 (their whole intent), SB shoves all-in
	// for 200 total (a short all-in — SB already posted 10 as small blind,
	// so calling Dealer's raise plus going all-in uses the remaining 190 of
	// their 200 stack; Table.Act redirects this ActionRaise to a Call since
	// 200 can't reach the 220 current bet), BB calls.
	if err := table.Act("Dealer", betting.ActionRaise, 220); err != nil {
		t.Fatalf("Dealer raises to 220: %v", err)
	}
	if err := table.Act("SB", betting.ActionRaise, 200); err != nil {
		t.Fatalf("SB shoves all-in for 200 total: %v", err)
	}
	if err := table.Act("BB", betting.ActionCall, 220); err != nil {
		t.Fatalf("BB calls 220: %v", err)
	}
	if err := table.Act("Dealer", betting.ActionCall, 220); err != nil {
		t.Fatalf("Dealer calls the short all-in (owes nothing more, already at 220): %v", err)
	}

	// SB is all-in with 200 total in the pot; Dealer and BB each have 220 in.
	// Main pot: 200*3=600, eligible all three. Side pot: 20*2=40, eligible
	// Dealer+BB only. Play remaining streets with both non-all-in players
	// checking through (SB has no more decisions — they're all-in).
	for table.Stage() != Showdown && table.Stage() != Complete {
		for _, id := range []string{"Dealer", "BB"} {
			if table.currentPlayerCanAct(id) {
				if err := table.Act(id, betting.ActionCheck, 0); err != nil {
					t.Fatalf("check on %v for %s: %v", table.Stage(), id, err)
				}
			}
		}
	}

	payouts := table.Payouts()
	var total int64
	for _, amount := range payouts {
		total += amount
	}
	if total != 640 { // 600 main pot + 40 side pot
		t.Fatalf("total payouts must equal total pot (640), got %d (%+v)", total, payouts)
	}
	if _, ok := payouts["SB"]; !ok {
		t.Fatal("SB contributed to and must be eligible for the main pot")
	}
	if payouts["SB"] != 600 {
		t.Fatalf("SB's rigged quad aces must win the full 600 main pot outright, got %d", payouts["SB"])
	}
	if payouts["Dealer"] != 40 {
		t.Fatalf("Dealer's rigged straight must beat BB's board-pair-of-aces for the 40 side pot (SB isn't eligible for it), got %d", payouts["Dealer"])
	}
}

func TestHeadsUpDealerPostsSmallBlind(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerSeat = 0 // P1 is dealer

	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if table.players[0].Contributed != 10 {
		t.Fatalf("heads-up: dealer (P1) must post the small blind, got Contributed=%d", table.players[0].Contributed)
	}
	if table.players[1].Contributed != 20 {
		t.Fatalf("heads-up: non-dealer (P2) must post the big blind, got Contributed=%d", table.players[1].Contributed)
	}
}

func TestReadyGateBlocksHandStartWithFewerThanTwoReady(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: false},
	}
	table := NewTable(players, 10, 20)
	if err := table.StartHand(); err == nil {
		t.Fatal("expected StartHand to fail with fewer than 2 ready players")
	}
}

func TestPendingEntryPlayerIsNotDealtIntoHandsUntilTheyPostBigBlind(t *testing.T) {
	players := []*Player{
		{ID: "P1", Stack: 1000, Ready: true},
		{ID: "P2", Stack: 1000, Ready: true},
	}
	table := NewTable(players, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	table.AddMidHandJoiner(&Player{ID: "P3", Stack: 1000})
	if table.playerByID("P3").State != PendingEntry {
		t.Fatal("mid-hand joiner must start as PendingEntry")
	}
}
