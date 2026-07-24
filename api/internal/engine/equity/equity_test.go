package equity

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestPocketAcesHeadsUpPreflopIsStrongFavorite(t *testing.T) {
	hole := [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.Ace, Suit: deck.Diamonds}}
	eq, err := Estimate(hole, nil, nil, 1, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if eq < .75 || eq > 1 {
		t.Fatalf("pocket aces equity out of expected range: %f", eq)
	}
}

func TestEstimateRejectsInvalidInputs(t *testing.T) {
	ace := deck.Card{Rank: deck.Ace, Suit: deck.Clubs}
	if _, err := Estimate([2]deck.Card{ace, ace}, nil, nil, 1, 10); err == nil {
		t.Fatal("expected duplicate-card error")
	}
	if _, err := Estimate([2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.King, Suit: deck.Clubs}}, nil, nil, 1, 0); err == nil {
		t.Fatal("expected invalid-iterations error")
	}
}
