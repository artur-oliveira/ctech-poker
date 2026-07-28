// Package equity estimates poker equity against random opponent ranges.
package equity

import (
	"fmt"
	"math/rand/v2"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
)

// rng64 is a 64-bit XorShift64Star PRNG with 32-bit caching.
// It generates two 32-bit pseudo-random values per 64-bit step (~0.5ns per output).
type rng64 struct {
	state uint64
	cache uint32
	has   bool
}

func (r *rng64) next32() uint32 {
	if r.has {
		r.has = false
		return r.cache
	}
	x := r.state
	if x == 0 {
		x = 0x853c49e6748fea9b
	}
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	val := x * 0x2545F4914F6CDD1D
	r.cache = uint32(val >> 32)
	r.has = true
	return uint32(val)
}

func (r *rng64) intn(k uint32) uint32 {
	m := uint64(r.next32()) * uint64(k)
	return uint32(m >> 32)
}

func Estimate(hole [2]deck.Card, board, deadCards []deck.Card, numOpponents, iterations int) (float64, error) {
	if numOpponents < 1 || iterations < 1 {
		return 0, fmt.Errorf("equity: opponents and iterations must be positive")
	}
	if len(board) > 5 {
		return 0, fmt.Errorf("equity: board has %d cards, maximum is 5", len(board))
	}

	var pool [52]uint8
	poolLen, err := buildPool(hole, board, deadCards, &pool)
	if err != nil {
		return 0, err
	}

	boardNeeded := 5 - len(board)
	need := boardNeeded + numOpponents*2
	if need > poolLen {
		return 0, fmt.Errorf("equity: not enough cards to sample %d opponents", numOpponents)
	}

	hero1ID := handeval.CardID(hole[0])
	hero2ID := handeval.CardID(hole[1])

	var baseBoardState handeval.BoardState
	for _, c := range board {
		baseBoardState.AddCard(c)
	}

	rng := rng64{state: rand.Uint64()}

	var cards [52]uint8
	copy(cards[:poolLen], pool[:poolLen])

	var shares float64

	for range iterations {
		for i := range need {
			j := i + int(rng.intn(uint32(poolLen-i)))
			cards[i], cards[j] = cards[j], cards[i]
		}

		boardState := baseBoardState
		for i := range boardNeeded {
			boardState.AddCardID(cards[i])
		}
		boardState.Finalize()

		myScore := boardState.Eval2IDs(hero1ID, hero2ID)
		bestScore := myScore
		tiedWinners := 1

		for opponent := range numOpponents {
			offset := boardNeeded + opponent*2
			score := boardState.Eval2IDs(cards[offset], cards[offset+1])
			if score > myScore {
				bestScore = score
				break
			}
			if score == myScore {
				tiedWinners++
			}
		}

		if bestScore == myScore {
			if tiedWinners == 1 {
				shares += 1.0
			} else {
				shares += 1.0 / float64(tiedWinners)
			}
		}
	}

	return shares / float64(iterations), nil
}

func buildPool(hole [2]deck.Card, board, dead []deck.Card, pool *[52]uint8) (int, error) {
	var seen uint64
	checkAndAdd := func(c deck.Card) error {
		if c.Rank < deck.Two || c.Rank > deck.Ace || c.Suit < deck.Clubs || c.Suit > deck.Spades {
			return fmt.Errorf("equity: invalid card %+v", c)
		}
		id := handeval.CardID(c)
		mask := uint64(1) << id
		if (seen & mask) != 0 {
			return fmt.Errorf("equity: duplicate known card %+v", c)
		}
		seen |= mask
		return nil
	}

	if err := checkAndAdd(hole[0]); err != nil {
		return 0, err
	}
	if err := checkAndAdd(hole[1]); err != nil {
		return 0, err
	}
	for _, c := range board {
		if err := checkAndAdd(c); err != nil {
			return 0, err
		}
	}
	for _, c := range dead {
		if err := checkAndAdd(c); err != nil {
			return 0, err
		}
	}

	poolLen := 0
	for id := uint8(0); id < 52; id++ {
		if (seen & (uint64(1) << id)) == 0 {
			pool[poolLen] = id
			poolLen++
		}
	}
	return poolLen, nil
}
