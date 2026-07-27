// Package equity estimates poker equity against random opponent ranges.
package equity

import (
	"fmt"
	"math/rand/v2"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
)

func Estimate(hole [2]deck.Card, board, deadCards []deck.Card, numOpponents, iterations int) (float64, error) {
	if numOpponents < 1 || iterations < 1 {
		return 0, fmt.Errorf("equity: opponents and iterations must be positive")
	}
	if len(board) > 5 {
		return 0, fmt.Errorf("equity: board has %d cards, maximum is 5", len(board))
	}
	pool, err := remainingDeck(hole, board, deadCards)
	if err != nil {
		return 0, err
	}
	boardNeeded := 5 - len(board)
	need := boardNeeded + numOpponents*2
	if need > len(pool) {
		return 0, fmt.Errorf("equity: not enough cards to sample %d opponents", numOpponents)
	}

	// pool is freshly built for this call, so the sampler can permute it in
	// place. It only ever touches the first `need` slots, which leaves a
	// validly shuffled deck behind for the next iteration — so the whole loop
	// allocates nothing.
	var fullBoard [5]deck.Card
	copy(fullBoard[:], board)

	var shares float64
	for range iterations {
		draw := sample(pool, need)
		copy(fullBoard[len(board):], draw[:boardNeeded])
		myScore := best7(hole, fullBoard)
		bestScore := myScore
		tiedWinners := 1
		for opponent := range numOpponents {
			offset := boardNeeded + opponent*2
			score := best7([2]deck.Card{draw[offset], draw[offset+1]}, fullBoard)
			switch {
			case score > bestScore:
				bestScore, tiedWinners = score, 0
			case score == bestScore && score == myScore:
				tiedWinners++
			}
		}
		if bestScore == myScore {
			shares += 1 / float64(tiedWinners)
		}
	}
	return shares / float64(iterations), nil
}

func best7(hole [2]deck.Card, board [5]deck.Card) handeval.Score {
	var cards [7]deck.Card
	cards[0], cards[1] = hole[0], hole[1]
	copy(cards[2:], board[:])
	return handeval.Best7(cards)
}

func remainingDeck(hole [2]deck.Card, board, dead []deck.Card) ([]deck.Card, error) {
	excluded := make(map[deck.Card]bool, 2+len(board)+len(dead))
	known := append([]deck.Card{hole[0], hole[1]}, board...)
	known = append(known, dead...)
	for _, card := range known {
		if card.Rank < deck.Two || card.Rank > deck.Ace || card.Suit < deck.Clubs || card.Suit > deck.Spades {
			return nil, fmt.Errorf("equity: invalid card %+v", card)
		}
		if excluded[card] {
			return nil, fmt.Errorf("equity: duplicate known card %+v", card)
		}
		excluded[card] = true
	}
	pool := make([]deck.Card, 0, 52-len(excluded))
	for suit := deck.Clubs; suit <= deck.Spades; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			card := deck.Card{Rank: rank, Suit: suit}
			if !excluded[card] {
				pool = append(pool, card)
			}
		}
	}
	return pool, nil
}

// sample partially Fisher-Yates shuffles cards in place and returns its first
// n as the drawn subset.
//
// It draws from math/rand/v2 rather than crypto/rand. This is a Monte Carlo
// estimate shown to a player as a UI hint — it never picks a card that is
// dealt, never touches the shuffle commit-reveal, and never moves a chip, so
// unpredictability buys nothing here while a CSPRNG read per card dominated
// the whole estimate's cost. Anything that actually deals cards still goes
// through deck.NewShuffle.
func sample(cards []deck.Card, n int) []deck.Card {
	for i := 0; i < n; i++ {
		j := i + rand.IntN(len(cards)-i)
		cards[i], cards[j] = cards[j], cards[i]
	}
	return cards[:n]
}
