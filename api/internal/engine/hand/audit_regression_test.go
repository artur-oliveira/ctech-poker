package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestSittingOutDealerAdvancesClockwiseWithinEligiblePlayers(t *testing.T) {
	a := &Player{ID: "a"}
	b := &Player{ID: "b", State: SittingOut}
	c := &Player{ID: "c"}
	d := &Player{ID: "d"}
	table := NewTable([]*Player{a, b, c, d}, 10, 20)
	table.dealerSeat = 1
	table.dealerDrawn = true

	active := []*Player{a, c, d}
	if got := active[table.dealerIndexWithin(active)].ID; got != "c" {
		t.Fatalf("button must advance clockwise from sitting-out b to c, got %s", got)
	}
	sb, bb := table.blindSeats(active)
	if active[sb].ID != "d" || active[bb].ID != "a" {
		t.Fatalf("expected c button, d small blind, a big blind; got sb=%s bb=%s", active[sb].ID, active[bb].ID)
	}
}

func TestPartialRevealUpdatesDurableHandOutcomeMask(t *testing.T) {
	p1 := &Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &Player{ID: "p2", Stack: 1000, Ready: true}
	table := NewTable([]*Player{p1, p2}, 10, 20)
	table.dealerSeat, table.dealerDrawn = 0, true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if err := table.Act("p1", betting.ActionFold, 0); err != nil {
		t.Fatal(err)
	}
	index := int32(0)
	if changed, err := table.RevealHoleCard("p1", &index); err != nil || !changed {
		t.Fatalf("partial reveal: changed=%v err=%v", changed, err)
	}
	info := table.LastOutcomeForActor().PlayerHands["p1"]
	if info.Revealed || info.RevealedCards != ([2]bool{true, false}) {
		t.Fatalf("expected only card zero persisted as public, got %+v", info)
	}
}

func TestBoardStreetsConsumeBurnCards(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1"}, {ID: "p2"}}, 10, 20)
	table.shuffle = &deck.ShuffleResult{Cards: [52]deck.Card{}}
	table.stage = PreFlop
	table.nextCard = 4

	table.dealFlop()
	if table.nextCard != 8 || len(table.board) != 3 {
		t.Fatalf("flop must consume one burn plus three board cards: next=%d board=%d", table.nextCard, len(table.board))
	}
	table.dealBoardCard()
	table.dealBoardCard()
	if table.nextCard != 12 || len(table.board) != 5 {
		t.Fatalf("turn+river must each consume a burn and board card: next=%d board=%d", table.nextCard, len(table.board))
	}
}

func TestHoleCardsAreDealtClockwiseInTwoPasses(t *testing.T) {
	a := &Player{ID: "a", Stack: 1000, Ready: true}
	b := &Player{ID: "b", Stack: 1000, Ready: true}
	c := &Player{ID: "c", Stack: 1000, Ready: true}
	table := NewTable([]*Player{a, b, c}, 10, 20)
	table.dealerSeat, table.dealerDrawn = 0, true
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if b.HoleCards[0] != table.shuffle.Cards[0] ||
		c.HoleCards[0] != table.shuffle.Cards[1] ||
		a.HoleCards[0] != table.shuffle.Cards[2] ||
		b.HoleCards[1] != table.shuffle.Cards[3] ||
		c.HoleCards[1] != table.shuffle.Cards[4] ||
		a.HoleCards[1] != table.shuffle.Cards[5] {
		t.Fatalf("unexpected deal order: a=%v b=%v c=%v", a.HoleCards, b.HoleCards, c.HoleCards)
	}
}
