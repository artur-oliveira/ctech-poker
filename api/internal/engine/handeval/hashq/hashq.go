// Package hashq is the minimal perfect hash over 7-card rank multisets.
//
// A 7-card hand that is not a flush is fully determined by how many cards it
// holds of each rank — a "quinary" vector q[13] with each entry in 0..4 and
// Sum(q) == 7. There are exactly Size == 49205 such vectors, and Hash maps
// them bijectively onto [0, Size) in lexicographic order, so the evaluator can
// answer with one array index instead of enumerating C(7,5) sub-hands.
//
// It lives in its own package because both the runtime evaluator (handeval)
// and the offline table generator (handeval/gen) must agree on the mapping
// exactly; a drifted copy would silently mis-rank hands.
package hashq

// Cards is the number of cards hashed (a Texas Hold'em showdown hand).
const Cards = 13

// Hand is the number of cards in the multiset being hashed.
const Hand = 7

// Size is the number of distinct rank multisets of Hand cards drawn from
// Cards ranks with at most 4 of any one rank — i.e. the exact length of the
// non-flush lookup table.
const Size = 49205

// dp[v][j][k] is the number of quinary vectors of length j summing to k that
// start with a value strictly less than v. Summing it position by position
// yields the lexicographic index of a vector. It is 520 small integers, built
// once at init in microseconds — no generated data needed.
var dp [5][Cards][Hand + 1]int32

func init() {
	// n[j][k]: ways to fill j positions, each 0..4, summing to exactly k.
	var n [Cards + 1][Hand + 1]int32
	n[0][0] = 1
	for j := 1; j <= Cards; j++ {
		for k := 0; k <= Hand; k++ {
			for u := 0; u <= 4 && u <= k; u++ {
				n[j][k] += n[j-1][k-u]
			}
		}
	}
	for v := 1; v <= 4; v++ {
		for j := 0; j < Cards; j++ {
			for k := 0; k <= Hand; k++ {
				var sum int32
				for u := 0; u < v && u <= k; u++ {
					sum += n[j][k-u]
				}
				dp[v][j][k] = sum
			}
		}
	}
}

// Hash returns the lexicographic index in [0, Size) of the quinary vector q.
//
// The caller must guarantee each q[i] <= 4 and Sum(q) == Hand; violating that
// yields a meaningless index (or panics), so callers validate their cards
// first rather than paying for the check here.
func Hash(q *[Cards]uint8) int {
	sum, remaining := int32(0), Hand
	for i := 0; i < Cards; i++ {
		v := q[i]
		if v == 0 {
			continue
		}
		sum += dp[v][Cards-1-i][remaining]
		remaining -= int(v)
		if remaining == 0 {
			break
		}
	}
	return int(sum)
}
