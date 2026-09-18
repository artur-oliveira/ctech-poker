package equity

import (
	"math"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/equity/preflop"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
)

func TestPreflopLookupSelectionAndValidation(t *testing.T) {
	isolateCache(t)
	hole, _ := equityTestSpot()
	for _, iterations := range []int{1, 200, preflop.GeneratedSamples} {
		got, stats, err := EstimateWithStats(hole, nil, nil, 8, iterations)
		if err != nil || !stats.Precomputed || stats.CacheHit || got != preflop.Lookup(hole, 8) {
			t.Fatalf("lookup: value=%v stats=%+v err=%v", got, stats, err)
		}
	}
	if len(globalEquityCache.items) != 0 {
		t.Fatal("immutable preflop values populated LRU")
	}
	renamed := [2]deck.Card{{Rank: deck.King, Suit: deck.Hearts}, {Rank: deck.Ace, Suit: deck.Hearts}}
	got, _, err := EstimateWithStats(renamed, nil, nil, 8, 200)
	if err != nil || got != preflop.Lookup(hole, 8) {
		t.Fatal("suit-equivalent hands differ")
	}
	for _, bad := range [][2]deck.Card{{hole[0], hole[0]}, {{Rank: deck.King, Suit: deck.Suit(4)}, hole[1]}} {
		if _, _, err := EstimateWithStats(bad, nil, nil, 1, 200); err == nil {
			t.Fatal("lookup accepted malformed cards")
		}
	}
	for _, tc := range []struct {
		board, dead           []deck.Card
		opponents, iterations int
	}{
		{nil, []deck.Card{{Rank: deck.Two, Suit: deck.Clubs}}, 1, 200},
		{[]deck.Card{{Rank: deck.Two, Suit: deck.Clubs}}, nil, 1, 200},
		{nil, nil, 9, 200},
		{nil, nil, 1, preflop.GeneratedSamples + 1},
	} {
		got, stats, err := EstimateWithStats(hole, tc.board, tc.dead, tc.opponents, tc.iterations)
		if err != nil || stats.Precomputed || got < 0 || got > 1 {
			t.Fatalf("fallback: value=%v stats=%+v err=%v", got, stats, err)
		}
	}
}

// Enumerate a small remaining deck with the independent reference evaluator.
// This checks lazy early exits and split pots without relying on the sampler's
// shuffle or on precomputed preflop results.
func TestLazySamplingAgainstExhaustiveSmallDeck(t *testing.T) {
	hole, fullBoard := equityTestSpot()
	available := []deck.Card{{Rank: deck.Ace, Suit: deck.Diamonds}, {Rank: deck.Ace, Suit: deck.Hearts}, {Rank: deck.Two, Suit: deck.Clubs}, {Rank: deck.Four, Suit: deck.Diamonds}, {Rank: deck.King, Suit: deck.Diamonds}, {Rank: deck.Five, Suit: deck.Spades}}
	for _, boardLen := range []int{3, 4, 5} {
		board := fullBoard[:boardLen]
		keep := map[deck.Card]bool{hole[0]: true, hole[1]: true}
		for _, c := range board {
			keep[c] = true
		}
		for _, c := range available {
			keep[c] = true
		}
		var dead []deck.Card
		for id := uint8(0); id < 52; id++ {
			c := handeval.CardFromID(id)
			if !keep[c] {
				dead = append(dead, c)
			}
		}
		need := 5 - boardLen + 4
		var draws [6]deck.Card
		count := 0
		var shares float64
		wins, ties, losses := 0, 0, 0
		var enumerate func(int, uint8)
		enumerate = func(pos int, used uint8) {
			if pos == need {
				var cards [7]deck.Card
				copy(cards[:], board)
				copy(cards[boardLen:5], draws[:5-boardLen])
				cards[5], cards[6] = hole[0], hole[1]
				hero := ref.Best7(cards)
				tied := 1
				lost := false
				for opp := 0; opp < 2; opp++ {
					offset := 5 - boardLen + 2*opp
					cards[5], cards[6] = draws[offset], draws[offset+1]
					score := ref.Best7(cards)
					if score > hero {
						lost = true
					}
					if score == hero {
						tied++
					}
				}
				count++
				if lost {
					losses++
				} else {
					shares += 1 / float64(tied)
					if tied > 1 {
						ties++
					} else {
						wins++
					}
				}
				return
			}
			for i, c := range available {
				if used&(1<<i) == 0 {
					draws[pos] = c
					enumerate(pos+1, used|1<<i)
				}
			}
		}
		enumerate(0, 0)
		want := shares / float64(count)
		got, err := Estimate(hole, board, dead, 2, 200_000)
		if err != nil || math.Abs(got-want) > .006 {
			t.Fatalf("board %d: got %v want %v err=%v (wins=%d ties=%d losses=%d)", boardLen, got, want, err, wins, ties, losses)
		}
		if boardLen == 5 && (wins == 0 || ties == 0 || losses == 0) {
			t.Fatal("river fixture must exercise wins, losses, and ties")
		}
	}
}

func TestSampledPreflopAgreesWithIndependentOfflineTable(t *testing.T) {
	for _, class := range []int{0, 12, 12 * 13, 83, preflop.Classes - 1} {
		hole := preflop.Hole(class)
		for _, opponents := range []int{1, 2, 8} {
			got, err := sampledEstimateForTest(hole, nil, nil, opponents, 100_000)
			want := preflop.Lookup(hole, opponents)
			if err != nil || math.Abs(got-want) > .008 {
				t.Fatalf("class %d opponents %d got=%v want~%v err=%v", class, opponents, got, want, err)
			}
		}
	}
}

func BenchmarkEstimateGlobalCacheHit(b *testing.B) {
	hole, board := equityTestSpot()
	Estimate(hole, board[:3], nil, 8, 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Estimate(hole, board[:3], nil, 8, 200); err != nil {
			b.Fatal(err)
		}
	}
}
