// Package preflop provides offline Monte Carlo equity estimates for all
// suit-equivalent Hold'em starting hands against uniform random opponents.
package preflop

//go:generate go run ./gen -o table_generated.go

import "gopkg.aoctech.app/poker/api/internal/engine/deck"

const (
	Ranks                 = 13
	Classes               = Ranks * Ranks
	MaxOpponents          = 8
	DefaultSamples        = 4_000_000
	GeneratorSeed  uint64 = 0x706f6b65722d6571
)

// Class maps validated cards to a 13x13 matrix: pairs on the diagonal,
// suited hands above it, and offsuit hands below it. Reordering the cards
// or consistently renaming suits cannot change the result.
func Class(hole [2]deck.Card) int {
	low, high := int(hole[0].Rank-deck.Two), int(hole[1].Rank-deck.Two)
	if low > high {
		low, high = high, low
	}
	if hole[0].Suit == hole[1].Suit {
		return low*Ranks + high
	}
	return high*Ranks + low
}

// Hole returns a representative of a class in [0, Classes), for generation
// and validation. Distinct suits are used for pairs and offsuit hands.
func Hole(class int) [2]deck.Card {
	row, col := class/Ranks, class%Ranks
	suit := deck.Diamonds
	if row < col {
		suit = deck.Clubs
	}
	return [2]deck.Card{{Rank: deck.Rank(row) + deck.Two, Suit: deck.Clubs}, {Rank: deck.Rank(col) + deck.Two, Suit: suit}}
}

// Lookup requires valid distinct hole cards and a supported opponent count.
// Values are approximate expected pot shares, not exact probabilities.
func Lookup(hole [2]deck.Card, opponents int) float64 {
	return float64(equities[Class(hole)][opponents-1])
}
