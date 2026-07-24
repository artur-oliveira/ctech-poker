// Package equity estimates poker equity against random opponent ranges.
package equity

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"

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

	var shares float64
	for range iterations {
		draw, err := shuffleSubset(pool, need)
		if err != nil {
			return 0, fmt.Errorf("equity: sample: %w", err)
		}
		fullBoard := append(append([]deck.Card(nil), board...), draw[:boardNeeded]...)
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

func best7(hole [2]deck.Card, board []deck.Card) handeval.Score {
	var cards [7]deck.Card
	cards[0], cards[1] = hole[0], hole[1]
	copy(cards[2:], board)
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

func shuffleSubset(pool []deck.Card, n int) ([]deck.Card, error) {
	cards := append([]deck.Card(nil), pool...)
	for i := 0; i < n; i++ {
		offset, err := randIntn(len(cards) - i)
		if err != nil {
			return nil, err
		}
		j := i + offset
		cards[i], cards[j] = cards[j], cards[i]
	}
	return cards[:n], nil
}

func randIntn(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("invalid upper bound %d", n)
	}
	limit := ^uint64(0) - (^uint64(0) % uint64(n))
	for {
		var bytes [8]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return 0, err
		}
		value := binary.BigEndian.Uint64(bytes[:])
		if value < limit {
			return int(value % uint64(n)), nil
		}
	}
}
