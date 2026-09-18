package shortdeck

import (
	"math/rand"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
)

func card(rank deck.Rank, suit deck.Suit) deck.Card { return deck.Card{Rank: rank, Suit: suit} }

// #296 acceptance criterion: flush beats full house in short-deck, the
// inverse of standard hold'em.
func TestFlushBeatsFullHouseInShortDeck(t *testing.T) {
	flush := [7]deck.Card{
		card(deck.Six, deck.Clubs), card(deck.Eight, deck.Clubs), card(deck.Ten, deck.Clubs),
		card(deck.Queen, deck.Clubs), card(deck.Ace, deck.Clubs),
		card(deck.Six, deck.Diamonds), card(deck.Six, deck.Hearts),
	}
	fullHouse := [7]deck.Card{
		card(deck.King, deck.Clubs), card(deck.King, deck.Diamonds), card(deck.King, deck.Hearts),
		card(deck.Queen, deck.Clubs), card(deck.Queen, deck.Diamonds),
		card(deck.Seven, deck.Hearts), card(deck.Nine, deck.Spades),
	}
	flushScore := Best7(flush)
	fullHouseScore := Best7(fullHouse)
	if Category(flushScore) != handeval.Flush {
		t.Fatalf("flush hand categorized as %v", Category(flushScore))
	}
	if Category(fullHouseScore) != handeval.FullHouse {
		t.Fatalf("full house hand categorized as %v", Category(fullHouseScore))
	}
	if flushScore <= fullHouseScore {
		t.Fatalf("flush (%d) must outrank full house (%d) in short-deck", flushScore, fullHouseScore)
	}
}

// Sanity check on the standard evaluator: same two hands, full house wins.
func TestFullHouseBeatsFlushInStandardDeck(t *testing.T) {
	flush := [7]deck.Card{
		card(deck.Six, deck.Clubs), card(deck.Eight, deck.Clubs), card(deck.Ten, deck.Clubs),
		card(deck.Queen, deck.Clubs), card(deck.Ace, deck.Clubs),
		card(deck.Six, deck.Diamonds), card(deck.Six, deck.Hearts),
	}
	fullHouse := [7]deck.Card{
		card(deck.King, deck.Clubs), card(deck.King, deck.Diamonds), card(deck.King, deck.Hearts),
		card(deck.Queen, deck.Clubs), card(deck.Queen, deck.Diamonds),
		card(deck.Seven, deck.Hearts), card(deck.Nine, deck.Spades),
	}
	if handeval.Best7(fullHouse) <= handeval.Best7(flush) {
		t.Fatal("standard hold'em must still rank full house above flush")
	}
}

func TestLowStraightIsAceSixSevenEightNine(t *testing.T) {
	low := [7]deck.Card{
		card(deck.Ace, deck.Clubs), card(deck.Six, deck.Diamonds), card(deck.Seven, deck.Hearts),
		card(deck.Eight, deck.Spades), card(deck.Nine, deck.Clubs),
		card(deck.King, deck.Hearts), card(deck.Jack, deck.Diamonds),
	}
	score := Best7(low)
	if Category(score) != handeval.Straight {
		t.Fatalf("A-6-7-8-9 must be a straight, got %v", Category(score))
	}
	// A higher straight (6-7-8-9-10) must outrank it.
	sixToTen := [7]deck.Card{
		card(deck.Six, deck.Clubs), card(deck.Seven, deck.Diamonds), card(deck.Eight, deck.Hearts),
		card(deck.Nine, deck.Spades), card(deck.Ten, deck.Clubs),
		card(deck.King, deck.Hearts), card(deck.Jack, deck.Diamonds),
	}
	if Best7(sixToTen) <= score {
		t.Fatal("6-7-8-9-10 must outrank the A-6-7-8-9 low straight")
	}
}

// Differential test (mirrors handeval's differential_test.go pattern):
// every random 7-card hand drawn strictly from short-deck ranks (Six..Ace)
// must be internally consistent between the two entry points this package
// exposes — Best7 (7-choose-5 direct) and a brute-force re-check that
// enumerates every 5-card sub-hand through evaluate5 independently, so a
// bug in Best7's own combination loop can't hide behind evaluate5 being
// right.
func TestBest7DifferentialAgainstDirectEnumeration(t *testing.T) {
	ranks := []deck.Rank{deck.Six, deck.Seven, deck.Eight, deck.Nine, deck.Ten,
		deck.Jack, deck.Queen, deck.King, deck.Ace}
	suits := []deck.Suit{deck.Clubs, deck.Diamonds, deck.Hearts, deck.Spades}
	var full []deck.Card
	for _, r := range ranks {
		for _, s := range suits {
			full = append(full, card(r, s))
		}
	}
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 2000; trial++ {
		rng.Shuffle(len(full), func(i, j int) { full[i], full[j] = full[j], full[i] })
		var hand [7]deck.Card
		copy(hand[:], full[:7])

		got := Best7(hand)

		var want handeval.Score
		idx := [5]int{0, 1, 2, 3, 4}
		for {
			var five [5]deck.Card
			for i, ix := range idx {
				five[i] = hand[ix]
			}
			if s := evaluate5(five); s > want {
				want = s
			}
			if !nextCombination(&idx, 7) {
				break
			}
		}
		if got != want {
			t.Fatalf("trial %d: Best7=%d, direct enumeration=%d, hand=%v", trial, got, want, hand)
		}
	}
}

func TestCategoryRoundTripsThroughStrengthOrder(t *testing.T) {
	for _, cat := range strengthOrder {
		s := makeScore(cat)
		if got := Category(s); got != cat {
			t.Fatalf("category=%v round-tripped as %v", cat, got)
		}
	}
}
