package equity

import (
	"fmt"
	"math"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
)

func equityTestSpot() ([2]deck.Card, []deck.Card) {
	return [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.King, Suit: deck.Clubs}},
		[]deck.Card{{Rank: deck.Two, Suit: deck.Hearts}, {Rank: deck.Seven, Suit: deck.Clubs}, {Rank: deck.Jack, Suit: deck.Diamonds}, {Rank: deck.Queen, Suit: deck.Spades}, {Rank: deck.Three, Suit: deck.Hearts}}
}

func isolateCache(t *testing.T) {
	t.Helper()
	previous := globalEquityCache
	globalEquityCache = newLRUCache(globalEquityCacheMaxBytes)
	t.Cleanup(func() { globalEquityCache = previous })
}

func TestInvalidCardsCannotHitCachedValidSpot(t *testing.T) {
	isolateCache(t)
	hole, board := equityTestSpot()
	if _, err := Estimate(hole, board, nil, 8, 200); err != nil {
		t.Fatal(err)
	}
	badHole := hole
	// (K, suit 4) and (A, clubs) have the same compact ID.
	badHole[0] = deck.Card{Rank: deck.King, Suit: deck.Suit(4)}
	badBoard := append([]deck.Card(nil), board...)
	badBoard[0] = deck.Card{Rank: deck.Rank(1), Suit: deck.Suit(6)}
	for _, tc := range []struct {
		hole  [2]deck.Card
		board []deck.Card
	}{{badHole, board}, {hole, badBoard}} {
		if _, stats, err := EstimateWithStats(tc.hole, tc.board, nil, 8, 200); err == nil || stats.CacheHit {
			t.Fatalf("invalid card accepted: stats=%+v err=%v", stats, err)
		}
	}
}

func TestEstimateRejectsImpossibleInputs(t *testing.T) {
	hole, board := equityTestSpot()
	for _, tc := range []struct {
		name        string
		board, dead []deck.Card
		opponents   int
	}{
		{"duplicate board", append(append([]deck.Card(nil), board[:3]...), board[0]), nil, 1},
		{"duplicate dead", board, []deck.Card{hole[0]}, 1},
		{"too many opponents", board, nil, 23},
		{"overflow opponents", nil, nil, int(^uint(0) >> 1)},
		{"too many board cards", append(append([]deck.Card(nil), board...), hole[0]), nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Estimate(hole, tc.board, tc.dead, tc.opponents, 200); err == nil {
				t.Fatal("accepted impossible input")
			}
		})
	}
}

// sampledEstimateForTest bypasses precomputed values and the result cache.
func sampledEstimateForTest(hole [2]deck.Card, board, dead []deck.Card, opponents, iterations int) (float64, error) {
	seen, err := knownCards(hole, board, dead)
	if err != nil {
		return 0, err
	}
	var pool []uint8
	for id := uint8(0); id < 52; id++ {
		if seen&(uint64(1)<<id) == 0 {
			pool = append(pool, id)
		}
	}
	return estimateUncached(hole, board, pool, opponents, iterations, seedFor(hole, board, dead, opponents, iterations)), nil
}

func TestEquivalentCardOrdersGiveSameEstimate(t *testing.T) {
	isolateCache(t)
	hole, board := equityTestSpot()
	dead := []deck.Card{{Rank: deck.Four, Suit: deck.Clubs}, {Rank: deck.Five, Suit: deck.Diamonds}}
	first, err := Estimate(hole, board[:3], dead, 2, 200)
	if err != nil {
		t.Fatal(err)
	}
	hole[0], hole[1] = hole[1], hole[0]
	board[0], board[2] = board[2], board[0]
	dead[0], dead[1] = dead[1], dead[0]
	second, err := Estimate(hole, board[:3], dead, 2, 200)
	if err != nil || first != second {
		t.Fatalf("reordered spot changed: %v vs %v, err=%v", first, second, err)
	}
}

func TestHeadsUpRiverMatchesReferenceEnumeration(t *testing.T) {
	isolateCache(t)
	hole, board := equityTestSpot()
	// Include suited boards, paired boards and dead cards; compare against the
	// independent five-card-combination reference evaluator, including split pots.
	royal := []deck.Card{{Rank: deck.Ten, Suit: deck.Spades}, {Rank: deck.Jack, Suit: deck.Spades}, {Rank: deck.Queen, Suit: deck.Spades}, {Rank: deck.King, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Spades}}
	paired := []deck.Card{{Rank: deck.Two, Suit: deck.Hearts}, {Rank: deck.Two, Suit: deck.Clubs}, {Rank: deck.Jack, Suit: deck.Diamonds}, {Rank: deck.Jack, Suit: deck.Spades}, {Rank: deck.Three, Suit: deck.Hearts}}
	for _, tc := range []struct {
		name        string
		board, dead []deck.Card
	}{{"high card", board, nil}, {"paired", paired, nil}, {"board royal", royal, nil}, {"dead cards", board, []deck.Card{{Rank: deck.Four, Suit: deck.Clubs}, {Rank: deck.Five, Suit: deck.Diamonds}}}} {
		t.Run(tc.name, func(t *testing.T) {
			known := map[deck.Card]bool{hole[0]: true, hole[1]: true}
			for _, c := range tc.board {
				known[c] = true
			}
			for _, c := range tc.dead {
				known[c] = true
			}
			var pool []deck.Card
			for r := deck.Two; r <= deck.Ace; r++ {
				for s := deck.Clubs; s <= deck.Spades; s++ {
					c := deck.Card{Rank: r, Suit: s}
					if !known[c] {
						pool = append(pool, c)
					}
				}
			}
			var h [7]deck.Card
			copy(h[:], tc.board)
			h[5], h[6] = hole[0], hole[1]
			hero := ref.Best7(h)
			var shares float64
			count := 0
			for i := range pool {
				for j := i + 1; j < len(pool); j++ {
					h[5], h[6] = pool[i], pool[j]
					other := ref.Best7(h)
					count++
					if hero > other {
						shares++
					} else if hero == other {
						shares += .5
					}
				}
			}
			want := shares / float64(count)
			for _, iterations := range []int{1, 200, 20000} {
				got, err := Estimate(hole, tc.board, tc.dead, 1, iterations)
				if err != nil || math.Abs(got-want) > 1e-15 {
					t.Fatalf("iterations %d: got %v want %v err=%v", iterations, got, want, err)
				}
			}
		})
	}
}

func BenchmarkEstimateUncached(b *testing.B) {
	previous := globalEquityCache
	globalEquityCache = newLRUCache(0)
	b.Cleanup(func() { globalEquityCache = previous })
	hole, board := equityTestSpot()
	for _, n := range []int{0, 3, 4, 5} {
		for _, opp := range []int{1, 8} {
			b.Run(fmt.Sprintf("board%d/opp%d", n, opp), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := Estimate(hole, board[:n], nil, opp, 200); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func TestMultiwayRiverSharesBoardRoyal(t *testing.T) {
	hole, _ := equityTestSpot()
	board := []deck.Card{{Rank: deck.Ten, Suit: deck.Spades}, {Rank: deck.Jack, Suit: deck.Spades}, {Rank: deck.Queen, Suit: deck.Spades}, {Rank: deck.King, Suit: deck.Spades}, {Rank: deck.Ace, Suit: deck.Spades}}
	for _, opponents := range []int{2, 8} {
		got, err := Estimate(hole, board, nil, opponents, 200)
		want := 1 / float64(opponents+1)
		if err != nil || math.Abs(got-want) > 1e-14 {
			t.Fatalf("opponents %d: got %v want %v err=%v", opponents, got, want, err)
		}
	}
}

func TestHeadsUpRiverWithOnlyOneLegalOpponentHand(t *testing.T) {
	hole, board := equityTestSpot()
	known := map[deck.Card]bool{hole[0]: true, hole[1]: true}
	for _, c := range board {
		known[c] = true
	}
	// Leave just the other two aces: they beat hero's ace-high on this board.
	opponents := [2]deck.Card{{Rank: deck.Ace, Suit: deck.Hearts}, {Rank: deck.Ace, Suit: deck.Diamonds}}
	var dead []deck.Card
	for r := deck.Two; r <= deck.Ace; r++ {
		for s := deck.Clubs; s <= deck.Spades; s++ {
			c := deck.Card{Rank: r, Suit: s}
			if !known[c] && c != opponents[0] && c != opponents[1] {
				dead = append(dead, c)
			}
		}
	}
	got, err := Estimate(hole, board, dead, 1, 1)
	if err != nil || got != 0 {
		t.Fatalf("got %v err=%v, want certain loss", got, err)
	}
	if _, err := Estimate(hole, board, append(dead, opponents[0]), 1, 1); err == nil {
		t.Fatal("accepted fewer than two remaining cards")
	}
}
