// Package handeval ranks the best 5-card hand out of 7 (OVERVIEW.md § 3.4).
// Score is a single comparable integer: higher Score always wins; equal Score
// is a genuine tie (split pot).
//
// Evaluation is table-driven (HenryRLee's perfect-hash scheme). A 7-card hand
// is either a flush — decided by the 13-bit rank mask of the suit holding 5+
// cards — or it is not, in which case only the rank multiset matters and a
// minimal perfect hash turns it into one array index. Additive card keys
// and two partial-rank tables compute the same index as handeval/hashq. Either
// way Best7 uses adds and small array lookups, with no allocation and no
// enumeration of the C(7,5) sub-hands.
//
// Score tables are generated offline from the reference evaluator
// (handeval/ref) and embedded. Startup decodes ~120 KB of scores and builds
// ~183 KiB of partial-rank indices using rank combinatorics.
package handeval

//go:generate go run ./gen -o tables.bin

import "gopkg.aoctech.app/poker/api/internal/engine/deck"

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

// Score is a dense hand strength in [1, 7462] — 1 is the worst possible hand
// (7-5-4-3-2 offsuit), 7462 a royal flush. The zero value means "no hand" and
// loses to everything, which is what showdown code relies on when it seeds a
// running best with a bare `var best Score`.
type Score uint32

// Category reports which of the ten hand categories the Score falls in.
func (s Score) Category() Category {
	if int(s) >= len(categoryTable) {
		return HighCard
	}
	return Category(categoryTable[s])
}

// Best7 returns the highest Score achievable from any 5 of the given 7 cards.
//
// It returns 0 for malformed input (a rank or suit outside the deck, or a
// duplicate card). A showdown must not panic on a card that an upstream bug
// left zeroed. Zero loses to every valid score.
func Best7(cards [7]deck.Card) Score {
	var key, mask uint64
	for _, c := range cards {
		if c.Rank < deck.Two || c.Rank > deck.Ace || c.Suit > deck.Spades {
			return 0
		}
		card := packedCards[CardID(c)]
		if mask&card.mask != 0 {
			return 0
		}
		key += card.key
		mask |= card.mask
	}
	return packedScore(key, mask)
}

// BoardState holds an additive card key and suited rank masks for the board.
// AddCard/AddCardID and Eval2/Eval2IDs are trusted fast paths: the board must
// contain exactly five valid distinct cards, and hole cards must also be valid
// and distinct from each other and the board.
type BoardState struct {
	key  uint64
	mask uint64
}

// CardID returns a compact uint8 identifier in [0, 51] for a deck.Card.
func CardID(c deck.Card) uint8 {
	return uint8(c.Rank-deck.Two)*4 + uint8(c.Suit)
}

// CardFromID reconstructs a deck.Card from a compact CardID in [0, 51].
func CardFromID(id uint8) deck.Card {
	return deck.Card{
		Rank: deck.Rank(id>>2) + deck.Two,
		Suit: deck.Suit(id & 3),
	}
}

// AddCard adds a single deck.Card to the BoardState.
func (b *BoardState) AddCard(c deck.Card) {
	b.AddCardID(CardID(c))
}

// AddCardID adds a compact card ID (0..51) to the BoardState.
func (b *BoardState) AddCardID(id uint8) {
	card := packedCards[id]
	b.key += card.key
	b.mask |= card.mask
}

// Finalize is retained for callers of the original BoardState API. Packed
// evaluation derives flush information directly, so no finalization is needed.
func (b *BoardState) Finalize() {}

// Eval2 evaluates 2 hole cards combined with this BoardState.
func (b *BoardState) Eval2(c1, c2 deck.Card) Score {
	return b.Eval2IDs(CardID(c1), CardID(c2))
}

// Eval2IDs evaluates 2 compact card IDs combined with this BoardState.
func (b *BoardState) Eval2IDs(c1ID, c2ID uint8) Score {
	c1, c2 := packedCards[c1ID], packedCards[c2ID]
	return packedScore(b.key+c1.key+c2.key, b.mask|c1.mask|c2.mask)
}
