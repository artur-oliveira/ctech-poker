package preflop

import (
	"math"
	"math/rand/v2"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
)

// Independently draw complete deals and score them with the brute-force
// reference evaluator, rather than using the generator's early exits or keys.
func TestLookupAgainstReferenceSimulation(t *testing.T) {
	for _, hole := range [][2]deck.Card{
		{{Rank: deck.Seven, Suit: deck.Clubs}, {Rank: deck.Two, Suit: deck.Diamonds}},
		{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.Ace, Suit: deck.Diamonds}},
	} {
		var pool []deck.Card
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			for suit := deck.Clubs; suit <= deck.Spades; suit++ {
				c := deck.Card{Rank: rank, Suit: suit}
				if c != hole[0] && c != hole[1] {
					pool = append(pool, c)
				}
			}
		}
		rng := rand.New(rand.NewPCG(91, 37))
		const samples = 30_000
		var shares float64
		for range samples {
			for i := 0; i < 7; i++ {
				j := i + rng.IntN(len(pool)-i)
				pool[i], pool[j] = pool[j], pool[i]
			}
			var h [7]deck.Card
			copy(h[:5], pool[:5])
			h[5], h[6] = hole[0], hole[1]
			hero := ref.Best7(h)
			h[5], h[6] = pool[5], pool[6]
			opponent := ref.Best7(h)
			if hero > opponent {
				shares++
			} else if hero == opponent {
				shares += .5
			}
		}
		want := shares / samples
		got := Lookup(hole, 1)
		if math.Abs(got-want) > .01 {
			t.Fatalf("hole=%v table=%v reference=%v", hole, got, want)
		}
		t.Logf("hole=%v table=%v reference=%v", hole, got, want)
	}
}
