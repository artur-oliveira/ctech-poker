// Package sidepots implements OVERVIEW.md § 3.3's side-pot algorithm as a
// pure, independently-tested function — this is the #1 place real-money
// poker engines have historically had payout bugs.
package sidepots

import "slices"

// Contribution is the total amount one player put into the pot this hand.
// Folded reports whether that player has since folded: a folded player's
// chips still count toward the pot total, but — unlike a live all-in — a
// folded partial bet must never carve out its own side pot. Folded money is
// dead money that is absorbed into the layers live players contest (and, if
// it sits above every live contribution, rolls down into the main pot).
type Contribution struct {
	PlayerID string
	Amount   int64
	Folded   bool
}

// PotLayer is one slice of the pot: an Amount, and the set of player IDs who
// are still in the hand and contributed enough to be eligible to win it.
// Folded players never appear in Eligible.
type PotLayer struct {
	Amount   int64
	Eligible []string
	// Uncalled marks a layer only one player ever put chips into — an
	// uncalled bet to be returned to that player, never contested or raked.
	Uncalled bool
}

// ComputeSidePots splits the pot into layers.
//
// Layer boundaries are drawn only at the distinct contribution levels of
// players still in the hand — a live all-in is what creates a side pot. A
// folded player's partial contribution never adds a boundary: its chips are
// spread across whichever layers cover the bands it spans and become dead
// money for the live players contesting those layers. The single exception
// is folded money sitting entirely above every live contribution: that band
// is kept separate only so a genuinely uncalled bet (one contributor, marked
// Uncalled and refunded) can be told apart from dead money that two folded
// players matched against each other (rolled down into the main pot).
func ComputeSidePots(contributions []Contribution) []PotLayer {
	var topLive int64
	for _, c := range contributions {
		if !c.Folded && c.Amount > topLive {
			topLive = c.Amount
		}
	}

	levels := make([]int64, 0, len(contributions))
	for _, c := range contributions {
		if c.Amount <= 0 {
			continue
		}
		// A live level always splits the pot; a folded level only does when
		// it rises above every live contribution.
		if !c.Folded || c.Amount > topLive {
			levels = append(levels, c.Amount)
		}
	}
	if len(levels) == 0 {
		return nil
	}
	slices.Sort(levels)
	levels = slices.Compact(levels)

	layers := make([]PotLayer, 0, len(levels))
	lastContested := -1
	var prev, deadCarry int64
	for _, level := range levels {
		var amount int64
		contributors := 0
		var soleContributor string
		var eligible []string
		for i := range contributions {
			c := &contributions[i]
			lo := min(c.Amount, prev)
			hi := min(c.Amount, level)
			if hi > lo {
				amount += hi - lo
				contributors++
				soleContributor = c.PlayerID
			}
			if !c.Folded && c.Amount >= level {
				eligible = append(eligible, c.PlayerID)
			}
		}
		prev = level
		if amount == 0 {
			continue
		}
		switch {
		case contributors == 1:
			// Nobody ever matched these chips — return them to their owner.
			layers = append(layers, PotLayer{
				Amount:   amount,
				Eligible: []string{soleContributor},
				Uncalled: true,
			})
		case len(eligible) == 0:
			// Every player who funded this band has folded: dead money. Roll
			// it into the nearest contested layer below, or carry it up to
			// the next one if none exists yet.
			if lastContested >= 0 {
				layers[lastContested].Amount += amount
			} else {
				deadCarry += amount
			}
		default:
			layers = append(layers, PotLayer{Amount: amount + deadCarry, Eligible: eligible})
			deadCarry = 0
			lastContested = len(layers) - 1
		}
	}
	if deadCarry > 0 {
		if len(layers) > 0 {
			layers[len(layers)-1].Amount += deadCarry
		} else {
			// No live player contributed anything (only reachable transiently,
			// before the hand resolves): pool the folded money and refund it
			// evenly rather than dropping chips.
			ids := make([]string, 0, len(contributions))
			for _, c := range contributions {
				if c.Amount > 0 {
					ids = append(ids, c.PlayerID)
				}
			}
			layers = append(layers, PotLayer{Amount: deadCarry, Eligible: ids, Uncalled: true})
		}
	}
	return layers
}
