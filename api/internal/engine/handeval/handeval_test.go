package handeval

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func c(rank deck.Rank, suit deck.Suit) deck.Card { return deck.Card{Rank: rank, Suit: suit} }

func TestRoyalFlushBeatsStraightFlush(t *testing.T) {
	royal := [7]deck.Card{
		c(deck.Ten, deck.Spades), c(deck.Jack, deck.Spades), c(deck.Queen, deck.Spades),
		c(deck.King, deck.Spades), c(deck.Ace, deck.Spades), c(deck.Two, deck.Clubs), c(deck.Three, deck.Diamonds),
	}
	straightFlush := [7]deck.Card{
		c(deck.Five, deck.Hearts), c(deck.Six, deck.Hearts), c(deck.Seven, deck.Hearts),
		c(deck.Eight, deck.Hearts), c(deck.Nine, deck.Hearts), c(deck.Two, deck.Clubs), c(deck.Three, deck.Diamonds),
	}
	if Best7(royal) <= Best7(straightFlush) {
		t.Fatal("royal flush must beat a lower straight flush")
	}
}

func TestFourOfAKindBeatsFullHouse(t *testing.T) {
	quads := [7]deck.Card{
		c(deck.Nine, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Nine, deck.Hearts),
		c(deck.Nine, deck.Spades), c(deck.Two, deck.Clubs), c(deck.Three, deck.Diamonds), c(deck.Four, deck.Hearts),
	}
	fullHouse := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.King, deck.Diamonds), c(deck.King, deck.Hearts),
		c(deck.Queen, deck.Spades), c(deck.Queen, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Four, deck.Hearts),
	}
	if Best7(quads) <= Best7(fullHouse) {
		t.Fatal("four of a kind must beat a full house")
	}
}

func TestFlushBeatsStraight(t *testing.T) {
	flush := [7]deck.Card{
		c(deck.Two, deck.Spades), c(deck.Five, deck.Spades), c(deck.Nine, deck.Spades),
		c(deck.Jack, deck.Spades), c(deck.King, deck.Spades), c(deck.Two, deck.Clubs), c(deck.Three, deck.Diamonds),
	}
	straight := [7]deck.Card{
		c(deck.Four, deck.Clubs), c(deck.Five, deck.Diamonds), c(deck.Six, deck.Hearts),
		c(deck.Seven, deck.Spades), c(deck.Eight, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Nine, deck.Hearts),
	}
	if Best7(flush) <= Best7(straight) {
		t.Fatal("flush must beat a straight")
	}
}

func TestWheelStraightAceCountsLow(t *testing.T) {
	wheel := [7]deck.Card{
		c(deck.Ace, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
		c(deck.Four, deck.Spades), c(deck.Five, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Jack, deck.Hearts),
	}
	highCardOnly := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.Jack, deck.Diamonds), c(deck.Nine, deck.Hearts),
		c(deck.Seven, deck.Spades), c(deck.Four, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
	}
	if Best7(wheel) <= Best7(highCardOnly) {
		t.Fatal("A-2-3-4-5 must be recognized as a straight (the wheel), beating a no-pair hand")
	}
}

func TestKickerBreaksTieBetweenEqualPairs(t *testing.T) {
	pairWithAceKicker := [7]deck.Card{
		c(deck.Nine, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Ace, deck.Hearts),
		c(deck.Four, deck.Spades), c(deck.Six, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
	}
	pairWithKingKicker := [7]deck.Card{
		c(deck.Nine, deck.Spades), c(deck.Nine, deck.Hearts), c(deck.King, deck.Diamonds),
		c(deck.Four, deck.Clubs), c(deck.Six, deck.Diamonds), c(deck.Two, deck.Clubs), c(deck.Three, deck.Spades),
	}
	if Best7(pairWithAceKicker) <= Best7(pairWithKingKicker) {
		t.Fatal("same pair rank must be broken by the higher kicker")
	}
}

func TestIdenticalHandsScoreEqualForSplitPot(t *testing.T) {
	handA := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.King, deck.Diamonds), c(deck.Queen, deck.Hearts),
		c(deck.Jack, deck.Spades), c(deck.Nine, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
	}
	handB := [7]deck.Card{
		c(deck.King, deck.Hearts), c(deck.King, deck.Spades), c(deck.Queen, deck.Diamonds),
		c(deck.Jack, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Four, deck.Clubs), c(deck.Five, deck.Spades),
	}
	if Best7(handA) != Best7(handB) {
		t.Fatal("identical pair+kickers across different suits must score equal (split pot)")
	}
}

// Self-added: two-pair vs two-pair distinguished by top pair rank, not by the
// second pair or kicker. Not in the brief's six; covers a category boundary
// (TwoPair-vs-TwoPair comparison) the brief's tests never exercise.
func TestTwoPairBrokenByTopPairRank(t *testing.T) {
	acesAndTwos := [7]deck.Card{
		c(deck.Ace, deck.Clubs), c(deck.Ace, deck.Diamonds), c(deck.Two, deck.Hearts),
		c(deck.Two, deck.Spades), c(deck.Three, deck.Clubs), c(deck.Five, deck.Diamonds), c(deck.Seven, deck.Hearts),
	}
	kingsAndQueens := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.King, deck.Diamonds), c(deck.Queen, deck.Hearts),
		c(deck.Queen, deck.Spades), c(deck.Ace, deck.Hearts), c(deck.Five, deck.Clubs), c(deck.Seven, deck.Diamonds),
	}
	if Best7(acesAndTwos) <= Best7(kingsAndQueens) {
		t.Fatal("higher top pair must win even though the second pair and kicker are lower")
	}
}

// Self-added: three-of-a-kind vs three-of-a-kind distinguished purely by trip
// rank. Not in the brief's six; covers a category boundary the brief's tests
// never exercise (ThreeOfAKind-vs-ThreeOfAKind).
func TestThreeOfAKindBrokenByTripRank(t *testing.T) {
	tripSevens := [7]deck.Card{
		c(deck.Seven, deck.Clubs), c(deck.Seven, deck.Diamonds), c(deck.Seven, deck.Hearts),
		c(deck.Two, deck.Spades), c(deck.Four, deck.Clubs), c(deck.Six, deck.Diamonds), c(deck.Nine, deck.Hearts),
	}
	tripSixes := [7]deck.Card{
		c(deck.Six, deck.Clubs), c(deck.Six, deck.Diamonds), c(deck.Six, deck.Hearts),
		c(deck.Ace, deck.Spades), c(deck.King, deck.Clubs), c(deck.Queen, deck.Diamonds), c(deck.Two, deck.Hearts),
	}
	if Best7(tripSevens) <= Best7(tripSixes) {
		t.Fatal("higher trip rank must win even though the losing hand has much higher kickers")
	}
}

// Regression: full house vs full house with the same trip rank, distinguished
// only by the pair rank. Added from task review — this exact boundary wasn't
// covered by any existing test and is exactly the kind of case a future
// refactor could silently regress.
func TestFullHouseBrokenByPairRankWhenTripRankTies(t *testing.T) {
	kingsFullOfQueens := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.King, deck.Diamonds), c(deck.King, deck.Hearts),
		c(deck.Queen, deck.Spades), c(deck.Queen, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
	}
	kingsFullOfJacks := [7]deck.Card{
		c(deck.King, deck.Clubs), c(deck.King, deck.Diamonds), c(deck.King, deck.Hearts),
		c(deck.Jack, deck.Spades), c(deck.Jack, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
	}
	if Best7(kingsFullOfQueens) <= Best7(kingsFullOfJacks) {
		t.Fatal("same trip rank must be broken by the higher pair rank")
	}
}

// Regression: A-2-3-4-6 contains an Ace and four low cards but is NOT the
// wheel (A-2-3-4-5) — must not be misdetected as a straight. Added from task
// review — the wheel special case is the most common wrong-way-round bug in
// amateur straight detection.
func TestAceLowNonWheelIsNotMisdetectedAsStraight(t *testing.T) {
	notAWheel := [7]deck.Card{
		c(deck.Ace, deck.Clubs), c(deck.Two, deck.Diamonds), c(deck.Three, deck.Hearts),
		c(deck.Four, deck.Spades), c(deck.Six, deck.Clubs), c(deck.Nine, deck.Diamonds), c(deck.Jack, deck.Hearts),
	}
	if Best7(notAWheel) >= makeScore(Straight, deck.Five) {
		t.Fatal("A-2-3-4-6 must not be scored as a straight — it is not the wheel (A-2-3-4-5)")
	}
}
