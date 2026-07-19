// Package handeval ranks the best 5-card hand out of 7 (OVERVIEW.md § 3.4).
// Score packs category + tiebreaker ranks into a single comparable integer:
// higher Score always wins; equal Score is a genuine tie (split pot).
package handeval

import (
	"sort"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

type Category uint8

const (
	HighCard Category = iota
	Pair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
	RoyalFlush
)

// Score encodes Category in the top 4 bits, then up to 5 tiebreaker ranks
// (4 bits each, most significant first) below it — ranks fit in 4 bits since
// the highest rank value is Ace=14 (0b1110).
type Score uint32

func makeScore(cat Category, tiebreaks ...deck.Rank) Score {
	s := Score(cat) << 24
	shift := 20
	for _, r := range tiebreaks {
		s |= Score(r) << shift
		shift -= 4
	}
	return s
}

// Best7 returns the highest Score achievable from any 5 of the given 7 cards.
func Best7(cards [7]deck.Card) Score {
	var best Score
	// All C(7,5) = 21 combinations.
	idx := [5]int{0, 1, 2, 3, 4}
	for {
		var hand [5]deck.Card
		for i, ix := range idx {
			hand[i] = cards[ix]
		}
		if s := evaluate5(hand); s > best {
			best = s
		}
		if !nextCombination(&idx, 7) {
			break
		}
	}
	return best
}

// nextCombination advances idx (a strictly increasing k-subset of [0,n)) to
// the next combination in lexicographic order; returns false when exhausted.
func nextCombination(idx *[5]int, n int) bool {
	k := len(idx)
	i := k - 1
	for i >= 0 && idx[i] == n-k+i {
		i--
	}
	if i < 0 {
		return false
	}
	idx[i]++
	for j := i + 1; j < k; j++ {
		idx[j] = idx[j-1] + 1
	}
	return true
}

func evaluate5(hand [5]deck.Card) Score {
	ranks := make([]deck.Rank, 5)
	suitCount := map[deck.Suit]int{}
	rankCount := map[deck.Rank]int{}
	for i, c := range hand {
		ranks[i] = c.Rank
		suitCount[c.Suit]++
		rankCount[c.Rank]++
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i] > ranks[j] })

	isFlush := len(suitCount) == 1
	straightHigh, isStraight := straightHighCard(ranks)

	if isFlush && isStraight && straightHigh == deck.Ace {
		return makeScore(RoyalFlush)
	}
	if isFlush && isStraight {
		return makeScore(StraightFlush, straightHigh)
	}

	// Group ranks by count, descending count then descending rank.
	type group struct {
		rank  deck.Rank
		count int
	}
	groups := make([]group, 0, len(rankCount))
	for r, cnt := range rankCount {
		groups = append(groups, group{rank: r, count: cnt})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].rank > groups[j].rank
	})

	switch {
	case groups[0].count == 4:
		return makeScore(FourOfAKind, groups[0].rank, groups[1].rank)
	case groups[0].count == 3 && groups[1].count == 2:
		return makeScore(FullHouse, groups[0].rank, groups[1].rank)
	case isFlush:
		return makeScore(Flush, ranks[0], ranks[1], ranks[2], ranks[3], ranks[4])
	case isStraight:
		return makeScore(Straight, straightHigh)
	case groups[0].count == 3:
		return makeScore(ThreeOfAKind, groups[0].rank, groups[1].rank, groups[2].rank)
	case groups[0].count == 2 && groups[1].count == 2:
		return makeScore(TwoPair, groups[0].rank, groups[1].rank, groups[2].rank)
	case groups[0].count == 2:
		return makeScore(Pair, groups[0].rank, groups[1].rank, groups[2].rank, groups[3].rank)
	default:
		return makeScore(HighCard, ranks[0], ranks[1], ranks[2], ranks[3], ranks[4])
	}
}

// straightHighCard returns the high card of a straight among 5 descending,
// deduplicated-by-caller ranks, handling the wheel (A-2-3-4-5, where Ace
// counts low and the straight's "high card" for scoring is Five).
func straightHighCard(descRanks []deck.Rank) (deck.Rank, bool) {
	// A 5-card hand from evaluate5 always has exactly 5 ranks (possibly with
	// duplicates when not a straight candidate) — straights only apply when
	// all 5 are distinct.
	seen := map[deck.Rank]bool{}
	for _, r := range descRanks {
		if seen[r] {
			return 0, false
		}
		seen[r] = true
	}
	if descRanks[0]-descRanks[4] == 4 {
		return descRanks[0], true
	}
	// Wheel: A,5,4,3,2 sorted descending is [14,5,4,3,2].
	if descRanks[0] == deck.Ace && descRanks[1] == deck.Five && descRanks[2] == deck.Four &&
		descRanks[3] == deck.Three && descRanks[4] == deck.Two {
		return deck.Five, true
	}
	return 0, false
}
