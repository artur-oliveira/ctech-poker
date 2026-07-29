package achievements

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
)

func TestReferenceBestNAgreesWithRuntimeBest7Category(t *testing.T) {
	cards := [7]deck.Card{
		{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.King, Suit: deck.Spades},
		{Rank: deck.Queen, Suit: deck.Spades}, {Rank: deck.Jack, Suit: deck.Spades},
		{Rank: deck.Ten, Suit: deck.Spades}, {Rank: deck.Two, Suit: deck.Clubs},
		{Rank: deck.Three, Suit: deck.Diamonds},
	}
	if int(ref.BestN(cards[:]).Category()) != int(handeval.Best7(cards).Category()) {
		t.Fatal("reference and runtime evaluators disagree")
	}
}

func TestCardCombinatorics(t *testing.T) {
	t.Run("four to royal", func(t *testing.T) {
		outcome := hand.HandOutcome{
			Board:       []string{"Qs", "Js", "2d", "7c", "3h"},
			PlayerHands: map[string]hand.PlayerHandInfo{"p": {HoleCards: [2]string{"As", "Ks"}}},
		}
		cards, ok := playerCards(outcome, "p")
		if !ok || !fourToRoyalMissed(cards) || !fourToStraightFlushMissed(cards) {
			t.Fatal("expected both royal and straight-flush near misses")
		}
	})

	t.Run("river misses flush draw", func(t *testing.T) {
		outcome := hand.HandOutcome{
			Board:       []string{"2s", "7s", "Kd", "Qc", "3h"},
			PlayerHands: map[string]hand.PlayerHandInfo{"p": {HoleCards: [2]string{"As", "Js"}}},
		}
		cards, _ := playerCards(outcome, "p")
		if !riverDrawMissed(cards[:6], cards[6]) {
			t.Fatal("expected missed four-flush")
		}
	})

	t.Run("known nuts", func(t *testing.T) {
		outcome := hand.HandOutcome{
			Board: []string{"As", "Ah", "Ad", "Kc", "2d"},
			PlayerHands: map[string]hand.PlayerHandInfo{
				"winner": {HoleCards: [2]string{"Ac", "Ks"}},
				"loser":  {HoleCards: [2]string{"Kh", "Qh"}},
			},
		}
		if !isNuts(outcome, "winner") {
			t.Fatal("nut enumeration did not identify quad aces with king kicker")
		}
	})
}
