// Package sidepots implements OVERVIEW.md § 3.3's side-pot algorithm as a
// pure, independently-tested function — this is the #1 place real-money
// poker engines have historically had payout bugs.
package sidepots

import "sort"

// Contribution is the total amount one player put into the pot this hand
// (win-or-lose — folded players' chips still count, they're simply never in
// any layer's Eligible list because the caller filters folded players out
// before running the showdown evaluation against each layer).
type Contribution struct {
	PlayerID string
	Amount   int64
}

// PotLayer is one slice of the pot: an Amount, and the set of player IDs who
// contributed enough to be eligible to win it. The caller (Task 9's hand
// lifecycle) is responsible for further excluding folded players from
// Eligible before running the showdown — this function only knows about
// chip amounts, not fold state.
type PotLayer struct {
	Amount   int64
	Eligible []string
}

// ComputeSidePots sorts distinct contribution levels ascending; each layer
// between two consecutive levels is (levelDelta * numContributorsAtOrAboveLevel),
// and a player is eligible for a layer only if their own contribution reaches
// that layer's upper bound.
func ComputeSidePots(contributions []Contribution) []PotLayer {
	levels := make([]int64, 0, len(contributions))
	seen := map[int64]bool{}
	for _, c := range contributions {
		if c.Amount > 0 && !seen[c.Amount] {
			seen[c.Amount] = true
			levels = append(levels, c.Amount)
		}
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })

	layers := make([]PotLayer, 0, len(levels))
	var prev int64
	for _, level := range levels {
		delta := level - prev
		var eligible []string
		for _, c := range contributions {
			if c.Amount >= level {
				eligible = append(eligible, c.PlayerID)
			}
		}
		layers = append(layers, PotLayer{
			Amount:   delta * int64(len(eligible)),
			Eligible: eligible,
		})
		prev = level
	}
	return layers
}
