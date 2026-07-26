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

func TestLegalRaisePresetsAreAuthoritativeAndBounded(t *testing.T) {
	table := NewTable([]*Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	current := table.currentPlayerToAct()
	legal := table.ViewFor(current).LegalActions
	if legal == nil || legal.MaxRaiseTo == 0 {
		t.Fatal("current player must receive server-authored raise limits")
	}
	values := []int64{
		legal.OneThirdPotRaiseTo, legal.HalfPotRaiseTo,
		legal.TwoThirdsPotRaiseTo, legal.PotRaiseTo,
	}
	for i, value := range values {
		if value < legal.MinRaiseTo || value > legal.MaxRaiseTo {
			t.Fatalf("preset %d=%d outside [%d,%d]", i, value, legal.MinRaiseTo, legal.MaxRaiseTo)
		}
		if i > 0 && value < values[i-1] {
			t.Fatalf("presets must be monotonic, got %v", values)
		}
	}
	if legal.Step != 20 {
		t.Fatalf("raise step should use the 20-chip big blind, got %d", legal.Step)
	}
}

func TestOddChipStartsLeftOfDealer(t *testing.T) {
	players := []*Player{{ID: "dealer"}, {ID: "left"}, {ID: "right"}}
	table := NewTable(players, 10, 20)
	table.handOrder = players
	table.dealerSeat = 0

	if got := table.oddChipWinner([]string{"dealer", "left"}); got != "left" {
		t.Fatalf("odd chip winner = %s, want player left of dealer", got)
	}
	table.dealerSeat = 1
	if got := table.oddChipWinner([]string{"dealer", "right"}); got != "right" {
		t.Fatalf("odd chip winner after button move = %s, want right", got)
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

func TestViewForMarksActiveMidHandRebuyAsNotDealtIn(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	p3 := &Player{ID: "p3", Stack: 1000, Ready: true}
	p4 := &Player{ID: "p4", Stack: 0, Ready: true, State: SittingOut}
	table := NewTable([]*Player{p1, p2, p3, p4}, 10, 20)
	table.dealerSeat = 0
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	if err := table.AddMidHandJoiner(&Player{ID: p4.ID, Stack: 500}); err != nil {
		t.Fatalf("mid-hand rebuy: %v", err)
	}
	if p4.State != Active {
		t.Fatalf("expected p4 active for the next deal, got %v", p4.State)
	}

	for table.Stage() != Complete {
		current := table.CurrentPlayerIDForActor()
		if current == "" {
			t.Fatalf("hand stopped without a current player at stage %v", table.Stage())
		}
		if err := table.Act(current, betting.ActionCall, 0); err != nil {
			if checkErr := table.Act(current, betting.ActionCheck, 0); checkErr != nil {
				t.Fatalf("%s could neither call (%v) nor check (%v)", current, err, checkErr)
			}
		}
	}

	view := table.ViewFor(p4.ID)
	if len(view.Payouts) == 0 {
		t.Fatal("expected completed-hand snapshot to contain payouts")
	}
	for _, seat := range view.Seats {
		switch seat.PlayerID {
		case p1.ID, p2.ID, p3.ID:
			if !seat.DealtIn {
				t.Fatalf("dealt player %s reported dealt_in=false", seat.PlayerID)
			}
		case p4.ID:
			if seat.State != "active" {
				t.Fatalf("expected rebuy seat active for the next deal, got %q", seat.State)
			}
			if seat.DealtIn {
				t.Fatal("active mid-hand rebuy reported dealt_in=true")
			}
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
	table.shuffle.Cards[5] = deck.Card{Rank: deck.King, Suit: deck.Diamonds}
	table.shuffle.Cards[6] = deck.Card{Rank: deck.Queen, Suit: deck.Hearts}
	table.shuffle.Cards[7] = deck.Card{Rank: deck.Nine, Suit: deck.Spades}
	table.shuffle.Cards[9] = deck.Card{Rank: deck.Two, Suit: deck.Clubs}
	table.shuffle.Cards[11] = deck.Card{Rank: deck.Seven, Suit: deck.Diamonds}

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

// TestViewForExposesBlindSeatsHeadsUp mirrors TestHeadsUpDealerPostsSmallBlind's
// dealer-posts-small-blind rule, but asserts on the wire snapshot instead of
// Contributed, since that's what the client actually has to work with.
func TestViewForExposesBlindSeatsHeadsUp(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	table.dealerSeat = 0 // p1 is dealer
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	view := table.ViewFor("p1")
	if view.SmallBlindPlayerID != "p1" {
		t.Fatalf("heads-up: dealer (p1) must be small blind, got %q", view.SmallBlindPlayerID)
	}
	if view.BigBlindPlayerID != "p2" {
		t.Fatalf("heads-up: non-dealer (p2) must be big blind, got %q", view.BigBlindPlayerID)
	}
	if view.DealerPlayerID != "p1" {
		t.Fatalf("heads-up: dealer must be p1, got %q", view.DealerPlayerID)
	}
}

func TestViewForExposesBlindSeatsThreeHanded(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	p3 := &Player{ID: "p3", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2, p3}, 10, 20)
	table.dealerSeat = 0 // p1 is dealer
	table.dealerDrawn = true
	if err := table.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	view := table.ViewFor("p1")
	if view.SmallBlindPlayerID != "p2" {
		t.Fatalf("3-handed: seat left of dealer (p2) must be small blind, got %q", view.SmallBlindPlayerID)
	}
	if view.BigBlindPlayerID != "p3" {
		t.Fatalf("3-handed: seat two left of dealer (p3) must be big blind, got %q", view.BigBlindPlayerID)
	}
	if view.DealerPlayerID != "p1" {
		t.Fatalf("3-handed: dealer must be p1, got %q", view.DealerPlayerID)
	}
}

func TestViewForOmitsBlindSeatsBeforeFirstHand(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1}, 10, 20)
	view := table.ViewFor("p1")
	if view.SmallBlindPlayerID != "" || view.BigBlindPlayerID != "" {
		t.Fatalf("no hand has started, blind seats must be empty, got sb=%q bb=%q", view.SmallBlindPlayerID, view.BigBlindPlayerID)
	}
	if view.DealerPlayerID != "" {
		t.Fatalf("no hand has started, dealer must be empty, got %q", view.DealerPlayerID)
	}
}
