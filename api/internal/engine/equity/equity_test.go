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

// TestEstimateKnownEquities is the guard on the sampler itself. A biased or
// stale-state shuffle still produces plausible-looking numbers, so the only
// real check is hitting known preflop equities within Monte Carlo error — at
// 200k iterations the standard error is about 0.1%, so a 1.5-point tolerance
// catches bias while staying stable in CI.
//
// The heads-up figures are the widely published ones. The nine-way figure is
// a regression pin measured here and cross-checked against an independent
// implementation (crypto/rand sampling driving handeval/ref), not a quoted
// number — published nine-way tables usually report win rate, which excludes
// the split-pot shares Estimate counts.
func TestEstimateKnownEquities(t *testing.T) {
	const iterations = 200_000
	card := func(r deck.Rank, s deck.Suit) deck.Card { return deck.Card{Rank: r, Suit: s} }

	cases := []struct {
		name      string
		hole      [2]deck.Card
		opponents int
		want      float64
	}{
		{"AA vs one random hand", [2]deck.Card{card(deck.Ace, deck.Clubs), card(deck.Ace, deck.Diamonds)}, 1, .852},
		{"72 offsuit vs one random hand", [2]deck.Card{card(deck.Seven, deck.Clubs), card(deck.Two, deck.Diamonds)}, 1, .354},
		{"AA vs eight random hands", [2]deck.Card{card(deck.Ace, deck.Clubs), card(deck.Ace, deck.Diamonds)}, 8, .345},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Estimate(tc.hole, nil, nil, tc.opponents, iterations)
			if err != nil {
				t.Fatal(err)
			}
			if diff := got - tc.want; diff > .015 || diff < -.015 {
				t.Fatalf("equity %.4f, want %.3f within 0.015 — sampler looks biased", got, tc.want)
			}
		})
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

// BenchmarkEstimateProduction exercises a cached full-table estimate. The
// actor requests 200 iterations and attaches the result to each snapshot.
func BenchmarkEstimateProduction(b *testing.B) {
	hole := [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.King, Suit: deck.Clubs}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Estimate(hole, nil, nil, 8, 200); err != nil {
			b.Fatal(err)
		}
	}
}
