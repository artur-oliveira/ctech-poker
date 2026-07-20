// Package deck implements a CSPRNG-shuffled 52-card deck with commit-reveal
// fairness (OVERVIEW.md § 3.5): the server commits to a hash of the shuffle
// before dealing, then reveals the seed after the hand so anyone can verify
// no card order was altered mid-hand.
package deck

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
)

type Suit uint8

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

// Rank uses the card's face value directly (2-10, Jack=11, Queen=12, King=13,
// Ace=14) so comparisons and the hand evaluator's tiebreak encoding (Task 6)
// never need a translation table.
type Rank uint8

const (
	Two   Rank = 2
	Three Rank = 3
	Four  Rank = 4
	Five  Rank = 5
	Six   Rank = 6
	Seven Rank = 7
	Eight Rank = 8
	Nine  Rank = 9
	Ten   Rank = 10
	Jack  Rank = 11
	Queen Rank = 12
	King  Rank = 13
	Ace   Rank = 14
)

type Card struct {
	Rank Rank
	Suit Suit
}

// ShuffleResult holds a freshly shuffled deck plus its fairness proof. Cards
// and ServerSeed must be kept secret by the caller until HAND_COMPLETE;
// CommitHash is safe to publish immediately (ARCHITECTURE.md § 3.5).
type ShuffleResult struct {
	Cards      [52]Card
	ServerSeed [32]byte
	CommitHash [32]byte
}

// NewShuffle draws a fresh CSPRNG seed and produces a shuffled deck plus its
// publishable commit hash.
func NewShuffle() (*ShuffleResult, error) {
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
	}
	cards := shuffleWithSeed(seed)
	return &ShuffleResult{
		Cards:      cards,
		ServerSeed: seed,
		CommitHash: commitHash(seed, cards),
	}, nil
}

func orderedDeck() [52]Card {
	var d [52]Card
	i := 0
	for _, s := range []Suit{Clubs, Diamonds, Hearts, Spades} {
		for r := Two; r <= Ace; r++ {
			d[i] = Card{Rank: r, Suit: s}
			i++
		}
	}
	return d
}

// shuffleWithSeed runs Fisher-Yates driven by a deterministic HMAC-SHA256
// byte stream keyed on seed, so the same seed always reproduces the same
// permutation (required so Verify can recompute it), while the seed itself
// only ever comes from crypto/rand (unpredictable to anyone without it).
func shuffleWithSeed(seed [32]byte) [52]Card {
	d := orderedDeck()
	var counter uint32
	nextIndex := func(max uint32) uint32 {
		for {
			var ctrBytes [4]byte
			binary.BigEndian.PutUint32(ctrBytes[:], counter)
			counter++
			mac := hmac.New(sha256.New, seed[:])
			mac.Write(ctrBytes[:])
			sum := mac.Sum(nil)
			v := binary.BigEndian.Uint32(sum[:4])
			// Rejection sampling to avoid modulo bias.
			// Compute limit = 2^32 - (2^32 % max)
			m := uint32(^uint32(0))
			rem := (m%max + 1) % max
			limit := m - rem + 1
			if rem == 0 || v < limit {
				return v % max
			}
		}
	}
	for i := len(d) - 1; i > 0; i-- {
		j := nextIndex(uint32(i + 1))
		d[i], d[j] = d[j], d[i]
	}
	return d
}

func commitHash(seed [32]byte, cards [52]Card) [32]byte {
	h := sha256.New()
	h.Write(seed[:])
	for _, c := range cards {
		h.Write([]byte{byte(c.Rank), byte(c.Suit)})
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
