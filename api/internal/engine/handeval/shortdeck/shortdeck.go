// Package shortdeck evaluates 7-card hands under short-deck (6+ hold'em)
// rules: the deck drops ranks Two through Five (36 cards, deck.ShortDeck),
// which makes flushes harder to make than full houses — so short-deck
// ranks Flush above FullHouse, the one category-order swap versus standard
// hold'em — and the lowest straight is A-6-7-8-9 (2-3-4-5-6 and A-2-3-4-5
// can't exist without ranks Two-Five).
//
// This is intentionally the reference-style combinatorial evaluator
// (C(7,5)=21 sub-hands checked directly), the same shape as
// handeval/ref.Best7 — not a second perfect-hash table. Short-deck hands are
// a sandbox-only, lower-volume path (#296); handeval's packed perfect-hash
// tables exist because the standard evaluator runs on every showdown and
// every live-equity Monte Carlo sample across the whole fleet. Building and
// generating a second table (handeval/gen) for a second deck size is real
// work with no measured need behind it yet.
// ponytail: O(21) combinations per Best7 call, no lookup table — upgrade to
// a generated perfect-hash table (mirroring handeval/gen) if short-deck
// volume or equity-calculation use ever makes this evaluator's cost show up.
package shortdeck

import (
	"sort"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
)

// strengthOrder is short-deck hold'em's hand ranking, weakest to strongest.
// Only Flush and FullHouse swap places versus standard hold'em's order
// (handeval.Category's own iota order) — flush is harder to make than full
// house once ranks Two-Five are removed, so it must rank higher.
var strengthOrder = []handeval.Category{
	handeval.HighCard, handeval.Pair, handeval.TwoPair, handeval.ThreeOfAKind,
	handeval.Straight, handeval.FullHouse, handeval.Flush, handeval.FourOfAKind,
	handeval.StraightFlush, handeval.RoyalFlush,
}

var strengthOf = buildStrengthOf()
var categoryOf = buildCategoryOf()

func buildStrengthOf() map[handeval.Category]uint32 {
	m := make(map[handeval.Category]uint32, len(strengthOrder))
	for i, c := range strengthOrder {
		m[c] = uint32(i)
	}
	return m
}

func buildCategoryOf() map[uint32]handeval.Category {
	m := make(map[uint32]handeval.Category, len(strengthOrder))
	for i, c := range strengthOrder {
		m[uint32(i)] = c
	}
	return m
}

// makeScore encodes category strength (top bits) then up to 5 tiebreaker
// ranks (4 bits each, most significant first) — the same layout
// handeval/ref.makeScore uses, but keyed by short-deck strength rather than
// handeval.Category's raw enum value. A Score this package produces is
// comparable only against other Scores from this package: never against
// handeval.Best7's standard-deck scores, and never through
// handeval.Score.Category() (that method reads handeval's own packed table,
// which knows nothing about short-deck strength order — use this package's
// Category instead).
func makeScore(cat handeval.Category, tiebreaks ...deck.Rank) handeval.Score {
	s := handeval.Score(strengthOf[cat]) << 24
	shift := 20
	for _, r := range tiebreaks {
		s |= handeval.Score(r) << shift
		shift -= 4
	}
	return s
}

// Category recovers the display category (handeval.Category, e.g. Flush,
// FullHouse) from a Score this package produced via Best7.
func Category(s handeval.Score) handeval.Category {
	return categoryOf[uint32(s>>24)]
}

// Best7 returns the highest short-deck Score achievable from any 5 of the
// given 7 cards.
func Best7(cards [7]deck.Card) handeval.Score {
	var best handeval.Score
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

// nextCombination advances idx (a strictly increasing 5-subset of [0,n)) to
// the next combination in lexicographic order; returns false when exhausted.
// Identical to handeval/ref's helper of the same name (kept as its own copy
// rather than exported/shared — it's five lines and not worth a dependency
// between the two evaluators).
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

func evaluate5(hand [5]deck.Card) handeval.Score {
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
		return makeScore(handeval.RoyalFlush)
	}
	if isFlush && isStraight {
		return makeScore(handeval.StraightFlush, straightHigh)
	}

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
		return makeScore(handeval.FourOfAKind, groups[0].rank, groups[1].rank)
	case groups[0].count == 3 && groups[1].count == 2:
		return makeScore(handeval.FullHouse, groups[0].rank, groups[1].rank)
	case isFlush:
		return makeScore(handeval.Flush, ranks[0], ranks[1], ranks[2], ranks[3], ranks[4])
	case isStraight:
		return makeScore(handeval.Straight, straightHigh)
	case groups[0].count == 3:
		return makeScore(handeval.ThreeOfAKind, groups[0].rank, groups[1].rank, groups[2].rank)
	case groups[0].count == 2 && groups[1].count == 2:
		return makeScore(handeval.TwoPair, groups[0].rank, groups[1].rank, groups[2].rank)
	case groups[0].count == 2:
		return makeScore(handeval.Pair, groups[0].rank, groups[1].rank, groups[2].rank, groups[3].rank)
	default:
		return makeScore(handeval.HighCard, ranks[0], ranks[1], ranks[2], ranks[3], ranks[4])
	}
}

// straightHighCard is handeval/ref's straightHighCard plus short-deck's own
// low straight, A-6-7-8-9 — 2-3-4-5-6 and A-2-3-4-5 can't exist without
// ranks Two-Five, so A-6-7-8-9 takes over as the wheel-equivalent low
// straight, scored as a Nine-high straight (the next straight up, 6-7-8-9-10,
// is a legitimate Ten-high straight above it).
func straightHighCard(descRanks []deck.Rank) (deck.Rank, bool) {
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
	// A-6-7-8-9 sorted descending is [14,9,8,7,6].
	if descRanks[0] == deck.Ace && descRanks[1] == deck.Nine && descRanks[2] == deck.Eight &&
		descRanks[3] == deck.Seven && descRanks[4] == deck.Six {
		return deck.Nine, true
	}
	return 0, false
}
