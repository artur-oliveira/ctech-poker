//go:build exhaustive

// Run with: go test -tags exhaustive -timeout 60m ./internal/engine/handeval/
//
// This walks every one of the C(52,7) = 133,784,560 possible showdown hands
// and proves the table evaluator induces exactly the reference evaluator's
// total order. It is gated behind a build tag because the reference evaluator
// costs microseconds per hand, which puts the full sweep in the minutes — too
// slow for `go test ./... -race` on every change.
package handeval

import (
	"runtime"
	"sync"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
)

func TestMatchesReferenceOrderingExhaustively(t *testing.T) {
	full := fullDeck()

	// Each worker takes whole first-card slices and records only the distinct
	// (reference score, table score) pairs it sees — at most 7462 of them, so
	// the maps stay tiny no matter how many hands go through.
	type pairs map[ref.Score]Score

	work := make(chan int, 52)
	for a := 0; a < 46; a++ {
		work <- a
	}
	close(work)

	var (
		mu     sync.Mutex
		merged = pairs{}
		wg     sync.WaitGroup
	)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := pairs{}
			for a := range work {
				for b := a + 1; b < 52; b++ {
					for c := b + 1; c < 52; c++ {
						for d := c + 1; d < 52; d++ {
							for e := d + 1; e < 52; e++ {
								for f := e + 1; f < 52; f++ {
									for g := f + 1; g < 52; g++ {
										hand := [7]deck.Card{full[a], full[b], full[c], full[d], full[e], full[f], full[g]}
										local[ref.Best7(hand)] = Best7(hand)
									}
								}
							}
						}
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for k, v := range local {
				merged[k] = v
			}
		}()
	}
	wg.Wait()

	// A map keyed by reference score cannot show a reference score mapping to
	// two different table scores, so the remaining risk is the other
	// direction: two distinct reference scores collapsing onto one table
	// score. verify's strict monotonicity check rules that out.
	check := newOrderCheck()
	for r, s := range merged {
		if prev, ok := check.byNew[s]; ok {
			t.Fatalf("table score %d is shared by reference scores %#x and %#x — distinct hands became a false tie", s, prev, r)
		}
		check.byRef[r] = s
		check.byNew[s] = r
		check.cat[s] = r.Category()
	}
	if len(merged) != 4824 {
		t.Fatalf("saw %d distinct strengths across all C(52,7) hands, want 4824", len(merged))
	}
	check.verify(t)
}
