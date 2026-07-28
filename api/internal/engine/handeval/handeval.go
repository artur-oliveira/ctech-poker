// Package handeval ranks the best 5-card hand out of 7 (OVERVIEW.md § 3.4).
// Score is a single comparable integer: higher Score always wins; equal Score
// is a genuine tie (split pot).
//
// Evaluation is table-driven (HenryRLee's perfect-hash scheme). A 7-card hand
// is either a flush — decided by the 13-bit rank mask of the suit holding 5+
// cards — or it is not, in which case only the rank multiset matters and a
// minimal perfect hash (handeval/hashq) turns it into one array index. Either
// way Best7 is a handful of adds and one lookup, with no allocation and no
// enumeration of the C(7,5) sub-hands.
//
// The tables are generated offline from the reference evaluator
// (handeval/ref) and embedded, so startup cost is a single ~120 KB decode.
package handeval

//go:generate go run ./gen -o tables.bin

import (
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/hashq"
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
// It returns 0 for malformed input (a rank or suit outside the deck, or the
// same rank appearing five or more times, which only a duplicate card can
// cause). A showdown seating real money must not panic on a card that some
// upstream bug left zeroed, and 0 loses every comparison, so a malformed hand
// simply cannot win a pot.
func Best7(cards [7]deck.Card) Score {
	var quinary [hashq.Cards]uint8
	var suitMask [4]uint16
	var suitCount [4]uint8

	for _, c := range cards {
		if c.Rank < deck.Two || c.Rank > deck.Ace || c.Suit > deck.Spades {
			return 0
		}
		r := c.Rank - deck.Two
		quinary[r]++
		if quinary[r] > 4 {
			return 0
		}
		suitMask[c.Suit] |= 1 << r
		suitCount[c.Suit]++
	}

	// At most one suit can hold 5 of 7 cards, and a 5-card flush always beats
	// every non-flush hand those same 7 cards can make (the two off-suit
	// cards can never complete quads or a full house), so this is a decision,
	// not a candidate.
	for s, count := range suitCount {
		if count >= 5 {
			return Score(flushTable[suitMask[s]])
		}
	}
	return Score(noFlushTable[hashq.Hash(&quinary)])
}

// BoardState holds pre-computed quinary and suit information for up to 5 board cards.
type BoardState struct {
	quinary   [hashq.Cards]uint8
	suitMask  [4]uint16
	suitCount [4]uint8
	flushSuit int8 // -1 if no suit on board has >= 3 cards; 0..3 if a suit has >= 3 cards
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
	r := id >> 2
	s := id & 3
	b.quinary[r]++
	b.suitMask[s] |= 1 << r
	b.suitCount[s]++
}

// Finalize calculates the flushSuit flag after all 5 board cards are set.
func (b *BoardState) Finalize() {
	b.flushSuit = -1
	for s := range 4 {
		if b.suitCount[s] >= 3 {
			b.flushSuit = int8(s)
			break
		}
	}
}

// Eval2 evaluates 2 hole cards combined with this BoardState.
func (b *BoardState) Eval2(c1, c2 deck.Card) Score {
	return b.Eval2IDs(CardID(c1), CardID(c2))
}

// Eval2IDs evaluates 2 compact card IDs combined with this BoardState.
func (b *BoardState) Eval2IDs(c1ID, c2ID uint8) Score {
	q := b.quinary

	r1, s1 := c1ID>>2, c1ID&3
	q[r1]++

	r2, s2 := c2ID>>2, c2ID&3
	q[r2]++

	if b.flushSuit >= 0 {
		fs := uint8(b.flushSuit)
		sc := b.suitCount[fs]
		if s1 == fs {
			sc++
		}
		if s2 == fs {
			sc++
		}
		if sc >= 5 {
			sm := b.suitMask[fs]
			if s1 == fs {
				sm |= 1 << r1
			}
			if s2 == fs {
				sm |= 1 << r2
			}
			return Score(flushTable[sm])
		}
	}

	return Score(noFlushTable[hashq.Hash(&q)])
}
