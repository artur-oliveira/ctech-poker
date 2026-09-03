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

var standardDeck = orderedDeck()

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
	d := standardDeck
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

// commitHash produces the legacy global commit hash: SHA256(seed || every
// card). Superseded by RootCommitHash, which commits to each card
// individually so one hand's mucked cards stay hidden while the rest are
// verifiable — but NOT dead. It is still live wire and storage:
// ShuffleResult.CommitHash flows into hand.Snapshot.ShuffleCommitHash (proto
// field 12, every table snapshot), hand.HandOutcome.CommitHash, and
// sessionlog's persisted commit_hash column, so every hand ever recorded is
// verifiable only against this exact construction. Compat-only: publish it,
// never build anything new on it, and never change the byte layout below —
// TestLegacyCommitHashLayoutIsPinned fails if you do.
func commitHash(seed [32]byte, cards [52]Card) [32]byte {
	var buf [32 + 52*2]byte
	copy(buf[:32], seed[:])
	for i, c := range cards {
		buf[32+i*2] = byte(c.Rank)
		buf[32+i*2+1] = byte(c.Suit)
	}
	return sha256.Sum256(buf[:])
}

// CardSalt derives a position-specific salt for card index i using HMAC-SHA256(seed, i).
// This allows revealing individual card salts at hand end without exposing unrevealed mucked cards.
func CardSalt(seed [32]byte, index int) [32]byte {
	mac := hmac.New(sha256.New, seed[:])
	var idxBytes [4]byte
	binary.BigEndian.PutUint32(idxBytes[:], uint32(index))
	mac.Write(idxBytes[:])
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// CardHash computes the cryptographic hash commitment for card at index i: SHA256(CardSalt(seed, i) || Rank || Suit).
func CardHash(seed [32]byte, index int, c Card) [32]byte {
	salt := CardSalt(seed, index)
	var buf [32 + 2]byte
	copy(buf[:32], salt[:])
	buf[32] = byte(c.Rank)
	buf[33] = byte(c.Suit)
	return sha256.Sum256(buf[:])
}

// RootCommitHash computes the root commitment hash over all 52 individual card hashes: SHA256(CardHash_0 || ... || CardHash_51).
func RootCommitHash(seed [32]byte, cards [52]Card) [32]byte {
	var buf [52 * 32]byte
	for i, c := range cards {
		h := CardHash(seed, i, c)
		copy(buf[i*32:(i+1)*32], h[:])
	}
	return sha256.Sum256(buf[:])
}

// VerifyPartial verifies that a set of revealed cards and unrevealed card commitments match rootCommit.
func VerifyPartial(rootCommit [32]byte, revealed map[int]struct {
	Card Card
	Salt [32]byte
}, unrevealedHashes map[int][32]byte) bool {
	var buf [52 * 32]byte
	for i := range 52 {
		if rev, ok := revealed[i]; ok {
			var cardBuf [32 + 2]byte
			copy(cardBuf[:32], rev.Salt[:])
			cardBuf[32] = byte(rev.Card.Rank)
			cardBuf[33] = byte(rev.Card.Suit)
			h := sha256.Sum256(cardBuf[:])
			copy(buf[i*32:(i+1)*32], h[:])
		} else if unrevHash, ok := unrevealedHashes[i]; ok {
			copy(buf[i*32:(i+1)*32], unrevHash[:])
		} else {
			return false // missing commitment for position i
		}
	}
	return sha256.Sum256(buf[:]) == rootCommit
}
