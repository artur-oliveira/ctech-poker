package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestViewForHidesOtherHoleCards(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	view := table.ViewFor("p1")
	var seatP1, seatP2 SeatView
	for _, s := range view.Seats {
		if s.PlayerID == "p1" {
			seatP1 = s
		}
		if s.PlayerID == "p2" {
			seatP2 = s
		}
	}
	if len(seatP1.HoleCards) != 2 {
		t.Fatalf("expected viewer to see their own 2 hole cards, got %d", len(seatP1.HoleCards))
	}
	if len(seatP2.HoleCards) != 0 {
		t.Fatalf("expected viewer NOT to see opponent hole cards, got %v", seatP2.HoleCards)
	}
}

func TestViewForHidesMidHandJoinerZeroValueCards(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}

	// p3 joins after the hand is already Complete — never dealt cards this
	// hand, so p3.HoleCards is still deck.Card{}'s zero value.
	p3 := &Player{ID: "p3", Stack: 1000}
	if err := table.AddMidHandJoiner(p3); err != nil {
		t.Fatalf("AddMidHandJoiner: %v", err)
	}

	view := table.ViewFor("p3")
	for _, s := range view.Seats {
		if s.PlayerID != "p3" {
			continue
		}
		if len(s.HoleCards) != 0 {
			t.Fatalf("mid-hand joiner never dealt cards this hand must not see hole_cards, got %v", s.HoleCards)
		}
	}

	// Other viewers must not see p3's phantom cards either (revealAll clause).
	view2 := table.ViewFor("p1")
	for _, s := range view2.Seats {
		if s.PlayerID != "p3" {
			continue
		}
		if len(s.HoleCards) != 0 {
			t.Fatalf("other viewers must not see mid-hand joiner's phantom cards, got %v", s.HoleCards)
		}
	}
}

// TestViewForHidesWinnerHoleCardsWhenHandEndsByFold reproduces the reported
// bug: a hand that ends because every other player folded (no genuine
// showdown) must not reveal the lone remaining player's hole cards to anyone
// but themselves. Only a hand that actually reaches Complete via a real
// showdown (2+ non-folded players comparing hands) may reveal.
func TestViewForHidesWinnerHoleCardsWhenHandEndsByFold(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	toAct := table.playerToActForTest()
	if err := table.Act(toAct, betting.ActionFold, 0); err != nil {
		t.Fatalf("%s folds: %v", toAct, err)
	}
	if table.Stage() != Complete {
		t.Fatalf("expected hand to reach Complete after fold-to-one, got %v", table.Stage())
	}

	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	view := table.ViewFor(toAct) // viewer is the player who folded, not the winner
	for _, s := range view.Seats {
		if s.PlayerID == winnerID && len(s.HoleCards) != 0 {
			t.Fatalf("winner-by-fold hole cards must stay hidden (no genuine showdown), got %v", s.HoleCards)
		}
	}
}

func TestViewForRevealsAllHandsAtShowdownForNonFolded(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	// Heads-up preflop: dealer(p1) posts SB and acts first. Call then check
	// through every street to reach Complete without any fold.
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	view := table.ViewFor("p1")
	for _, s := range view.Seats {
		if len(s.HoleCards) != 2 {
			t.Fatalf("expected every non-folded player's hand revealed at Complete, seat %s had %d cards", s.PlayerID, len(s.HoleCards))
		}
	}
}

func TestViewForIncludesHandCategoryWhenBoardIsComplete(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	view := table.ViewFor("p1")
	for _, s := range view.Seats {
		if s.HandCategory == "" {
			t.Fatalf("expected a hand_category for seat %s once the board is complete and cards are revealed", s.PlayerID)
		}
	}
}

func TestViewForFlagsWonWithoutShowdownForFoldToOne(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	toAct := table.playerToActForTest()
	if err := table.Act(toAct, betting.ActionFold, 0); err != nil {
		t.Fatalf("fold: %v", err)
	}
	view := table.ViewFor(toAct)
	if !view.WonWithoutShowdown {
		t.Fatal("expected won_without_showdown=true after a fold-to-one, so the client can offer a voluntary reveal button")
	}
}

func TestViewForOmitsWonWithoutShowdownForGenuineShowdown(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	view := table.ViewFor("p1")
	if view.WonWithoutShowdown {
		t.Fatal("expected won_without_showdown=false after a genuine showdown")
	}
}

// TestViewForOmitsUncalledExcessRecipientFromWinners is the wire-level half
// of TestUncalledAllInExcessIsNotCountedAsAWin (hand_test.go): the engine
// already keeps HandOutcome.Winners correct, but Snapshot — what the client
// actually receives — didn't expose it at all, leaving the frontend to infer
// "win" from payout>0, which is also true for a shover's refunded excess.
func TestViewForOmitsUncalledExcessRecipientFromWinners(t *testing.T) {
	players := []*Player{
		{ID: "Shover", Stack: 1000, Ready: true},
		{ID: "Caller", Stack: 100, Ready: true},
	}
	table := NewTable(players, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	players[0].HoleCards = [2]deck.Card{{Rank: deck.Five, Suit: deck.Clubs}, {Rank: deck.Six, Suit: deck.Clubs}}
	players[1].HoleCards = [2]deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Hearts}}
	table.shuffle.Cards[4] = deck.Card{Rank: deck.King, Suit: deck.Diamonds}
	table.shuffle.Cards[5] = deck.Card{Rank: deck.Queen, Suit: deck.Hearts}
	table.shuffle.Cards[6] = deck.Card{Rank: deck.Nine, Suit: deck.Spades}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Two, Suit: deck.Clubs}
	table.shuffle.Cards[8] = deck.Card{Rank: deck.Seven, Suit: deck.Diamonds}

	if err := table.Act("Shover", betting.ActionRaise, 1000); err != nil {
		t.Fatalf("Shover shoves all-in for 1000: %v", err)
	}
	if err := table.Act("Caller", betting.ActionCall, 0); err != nil {
		t.Fatalf("Caller calls all-in for their remaining 90: %v", err)
	}
	table.AdvanceRunoutStreetForActor()
	table.AdvanceRunoutStreetForActor()
	if table.Stage() != Complete {
		t.Fatalf("expected Complete, got %v", table.Stage())
	}

	view := table.ViewFor("Shover")
	if view.Payouts["Shover"] <= 0 {
		t.Fatal("expected Shover's uncalled excess to still show up in Payouts")
	}
	for _, id := range view.Winners {
		if id == "Shover" {
			t.Fatal("Shover lost the hand — must not appear in the wire Snapshot's Winners just because their uncalled excess was refunded")
		}
	}
	found := false
	for _, id := range view.Winners {
		if id == "Caller" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected Caller, who actually won the contested pot, in the wire Snapshot's Winners")
	}
}

func TestViewForPublishesCommitHashAssoonAsHandStarts(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	view := table.ViewFor("p1")
	if view.ShuffleCommitHash == "" {
		t.Fatal("expected the shuffle commit hash to be published as soon as the hand starts")
	}
	if view.ShuffleServerSeedHex != "" {
		t.Fatal("must not reveal the server seed before the hand is complete")
	}
}

func TestViewForRevealsServerSeedOnlyOnceComplete(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	_ = table.StartHand()
	for table.Stage() != Complete {
		toAct := table.playerToActForTest()
		if err := table.Act(toAct, betting.ActionCall, 0); err != nil {
			_ = table.Act(toAct, betting.ActionCheck, 0)
		}
	}
	view := table.ViewFor("p1")
	if view.ShuffleServerSeedHex == "" {
		t.Fatal("expected the server seed revealed once the hand is Complete")
	}
}

func TestViewForOmitsHandCategoryWhenCardsAreHidden(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	view := table.ViewFor("p1")
	for _, s := range view.Seats {
		if s.PlayerID == "p2" && s.HandCategory != "" {
			t.Fatal("must not leak an opponent's hand category before their cards are visible")
		}
	}
}
