package preflop

import (
	"math"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func TestAllStartingHandsMapToTheirSuitClass(t *testing.T) {
	var counts [Classes]int
	for a := 0; a < 52; a++ {
		for b := a + 1; b < 52; b++ {
			hole := [2]deck.Card{{Rank: deck.Rank(a/4) + deck.Two, Suit: deck.Suit(a % 4)}, {Rank: deck.Rank(b/4) + deck.Two, Suit: deck.Suit(b % 4)}}
			class := Class(hole)
			counts[class]++
			if Class([2]deck.Card{hole[1], hole[0]}) != class {
				t.Fatal("card order changed class")
			}
		}
	}
	for class, n := range counts {
		want := 12
		if class/Ranks == class%Ranks {
			want = 6
		} else if class/Ranks < class%Ranks {
			want = 4
		}
		if n != want {
			t.Fatalf("class %d has %d hands, want %d", class, n, want)
		}
		if got := Class(Hole(class)); got != class {
			t.Fatalf("class %d representative maps to %d", class, got)
		}
	}
}

func TestGeneratedTableQuality(t *testing.T) {
	if GeneratedSamples != DefaultSamples {
		t.Fatalf("table has %d samples/class, want %d", GeneratedSamples, DefaultSamples)
	}
	for class, row := range equities {
		previous := float32(1)
		for opponents, v := range row {
			if math.IsNaN(float64(v)) || v <= 0 || v >= 1 || v > previous {
				t.Fatalf("class %d opponents %d: invalid/nonmonotone equity %v", class, opponents+1, v)
			}
			previous = v
		}
	}
	aa := [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.Ace, Suit: deck.Diamonds}}
	sevenTwo := [2]deck.Card{{Rank: deck.Seven, Suit: deck.Clubs}, {Rank: deck.Two, Suit: deck.Diamonds}}
	for _, tc := range []struct {
		hole      [2]deck.Card
		opponents int
		want      float64
	}{{aa, 1, .852}, {aa, 8, .345}, {sevenTwo, 1, .346}} {
		if got := Lookup(tc.hole, tc.opponents); math.Abs(got-tc.want) > .003 {
			t.Fatalf("hole=%v opponents=%d got=%v want~%v", tc.hole, tc.opponents, got, tc.want)
		}
	}
}
