package handeval

import "gopkg.aoctech.app/poker/api/internal/engine/handeval/hashq"

// Split the quinary vector into six low ranks and seven high ranks. Each
// half is a base-5 integer, packed into a separate field of an additive key.
// No field can carry into the next on a valid hand. Two partial-rank lookups
// replace Hash's thirteen-rank scan, preserving its exact lexicographic index.
const (
	rankRadix      = 5
	lowRankCount   = 6
	highRankCount  = hashq.Cards - lowRankCount
	lowRankStates  = 15625 // 5^6
	highRankStates = 78125 // 5^7
	rankKeyShift   = 16    // enough bits for every low-rank state
	rankKeyMask    = (1 << rankKeyShift) - 1
)

// These tables occupy 187,500 bytes and depend only on the combinatorics of
// ranks, not poker hand strength. The existing generated tables.bin remains
// the source of scores. rankkey_test checks every valid key against hashq.
var (
	rankWeights   [hashq.Cards]uint64
	lowRankIndex  [lowRankStates]uint16
	highRankIndex [highRankStates]uint16
)

func init() {
	// ways[n][k] counts n-rank vectors with k cards, at most four per rank.
	var ways [hashq.Cards + 1][hashq.Hand + 1]int
	ways[0][0] = 1
	for n := 1; n <= hashq.Cards; n++ {
		for k := 0; k <= hashq.Hand; k++ {
			for v := 0; v < rankRadix && v <= k; v++ {
				ways[n][k] += ways[n-1][k-v]
			}
		}
	}
	weight := uint64(1)
	for r := range rankWeights {
		if r == lowRankCount {
			weight = 1 << rankKeyShift
		}
		rankWeights[r] = weight
		weight *= rankRadix
	}
	initPackedCards()
	// Count lexicographically earlier vectors by replacing each digit with
	// each smaller value and counting the possible remaining suffixes.
	partialIndex := func(code, start, length, remaining int) uint16 {
		index := 0
		for r := start; r < start+length; r++ {
			digit := code % rankRadix
			code /= rankRadix
			if digit > remaining {
				// Unreachable on a valid seven-card hand.
				return 0
			}
			for v := range digit {
				index += ways[hashq.Cards-1-r][remaining-v]
			}
			remaining -= digit
		}
		return uint16(index)
	}
	for code := range lowRankIndex {
		lowRankIndex[code] = partialIndex(code, 0, lowRankCount, hashq.Hand)
	}
	for code := range highRankIndex {
		remaining := 0
		for digits := code; digits > 0; digits /= rankRadix {
			remaining += digits % rankRadix
		}
		if remaining <= hashq.Hand {
			highRankIndex[code] = partialIndex(code, lowRankCount, highRankCount, remaining)
		}
	}
}

// rankIndex requires exactly seven distinct valid cards in the additive key.
func rankIndex(key uint64) int {
	return int(lowRankIndex[key&rankKeyMask]) + int(highRankIndex[key>>rankKeyShift])
}
