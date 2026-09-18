//go:build exhaustive

package handeval

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/hashq"
)

// Compare packed evaluation, including prepared boards, with the original
// quinary/suit-array algorithm on every valid hand. Unlike the slower ref
// ordering test, this requires exact score equality for each individual hand.
func TestPackedMatchesQuinaryExhaustively(t *testing.T) {
	full := fullDeck()
	work := make(chan int, 46)
	for a := 0; a < 46; a++ {
		work <- a
	}
	close(work)
	var wg sync.WaitGroup
	var checked atomic.Uint64
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var count uint64
			for a := range work {
				for b := a + 1; b < 52; b++ {
					for c := b + 1; c < 52; c++ {
						for d := c + 1; d < 52; d++ {
							for e := d + 1; e < 52; e++ {
								var board BoardState
								for _, card := range []deck.Card{full[a], full[b], full[c], full[d], full[e]} {
									board.AddCard(card)
								}
								board.Finalize()
								for f := e + 1; f < 52; f++ {
									for g := f + 1; g < 52; g++ {
										cards := [7]deck.Card{full[a], full[b], full[c], full[d], full[e], full[f], full[g]}
										var ranks [hashq.Cards]uint8
										var counts [4]uint8
										var masks [4]uint16
										for _, card := range cards {
											rank := card.Rank - deck.Two
											ranks[rank]++
											counts[card.Suit]++
											masks[card.Suit] |= 1 << rank
										}
										var want Score
										for suit, n := range counts {
											if n >= 5 {
												want = Score(flushTable[masks[suit]])
												break
											}
										}
										if want == 0 {
											want = Score(noFlushTable[hashq.Hash(&ranks)])
										}
										if got := Best7(cards); got != want {
											t.Errorf("hand %v: got %d want %d", cards, got, want)
											return
										}
										if got := board.Eval2(full[f], full[g]); got != want {
											t.Errorf("board hand %v: got %d want %d", cards, got, want)
											return
										}
										count++
									}
								}
							}
						}
					}
				}
			}
			checked.Add(count)
		}()
	}
	wg.Wait()
	const allSevenCardHands = 133_784_560
	if checked.Load() != allSevenCardHands {
		t.Fatalf("checked %d hands, want %d", checked.Load(), allSevenCardHands)
	}
}
