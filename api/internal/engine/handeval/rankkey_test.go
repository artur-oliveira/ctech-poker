package handeval

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/handeval/hashq"
)

func TestAdditiveRankIndexMatchesEveryQuinary(t *testing.T) {
	var q [hashq.Cards]uint8
	count := 0
	var visit func(int, int, uint64)
	visit = func(rank, remaining int, key uint64) {
		if rank == hashq.Cards {
			if remaining == 0 {
				count++
				if got, want := rankIndex(key), hashq.Hash(&q); got != want {
					t.Fatalf("quinary %v: got %d, want %d", q, got, want)
				}
			}
			return
		}
		for v := 0; v < rankRadix && v <= remaining; v++ {
			q[rank] = uint8(v)
			visit(rank+1, remaining-v, key+uint64(v)*rankWeights[rank])
		}
	}
	visit(0, hashq.Hand, 0)
	if count != hashq.Size {
		t.Fatalf("checked %d vectors, want %d", count, hashq.Size)
	}
}

func TestBoardStateMatchesAcrossRandomHandsAndSplits(t *testing.T) {
	for _, cards := range benchHands() {
		// Rotate which five cards constitute the board: especially important for
		// flushes where the board's three-, four-, and five-suited paths differ.
		for start := range cards {
			var board BoardState
			for i := 0; i < 5; i++ {
				board.AddCard(cards[(start+i)%7])
			}
			board.Finalize()
			got := board.Eval2(cards[(start+5)%7], cards[(start+6)%7])
			if want := Best7(cards); got != want {
				t.Fatalf("hand %v split %d: got %d want %d", cards, start, got, want)
			}
		}
	}
}

var benchmarkScore Score

func BenchmarkBoardStateEval2(b *testing.B) {
	hands := benchHands()
	boards := make([]BoardState, len(hands))
	holes := make([][2]uint8, len(hands))
	for i, cards := range hands {
		for _, c := range cards[:5] {
			boards[i].AddCard(c)
		}
		boards[i].Finalize()
		holes[i] = [2]uint8{CardID(cards[5]), CardID(cards[6])}
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sink Score
	for i := 0; i < b.N; i++ {
		j := i & (len(hands) - 1)
		sink ^= boards[j].Eval2IDs(holes[j][0], holes[j][1])
	}
	benchmarkScore = sink
}
