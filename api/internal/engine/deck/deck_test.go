package deck

import "testing"

func TestNewShuffleProducesAPermutationOf52UniqueCards(t *testing.T) {
	result, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}
	seen := make(map[Card]bool, 52)
	for _, c := range result.Cards {
		if seen[c] {
			t.Fatalf("duplicate card in shuffled deck: %+v", c)
		}
		seen[c] = true
	}
	if len(seen) != 52 {
		t.Fatalf("expected 52 unique cards, got %d", len(seen))
	}
}

func TestSameSeedReproducesSameShuffle(t *testing.T) {
	result, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}
	reproduced := shuffleWithSeed(result.ServerSeed)
	if reproduced != result.Cards {
		t.Fatal("shuffleWithSeed(seed) did not reproduce the original shuffle")
	}
}

func TestTwoShufflesProduceDifferentSeeds(t *testing.T) {
	a, _ := NewShuffle()
	b, _ := NewShuffle()
	if a.ServerSeed == b.ServerSeed {
		t.Fatal("two independent shuffles produced the same seed — CSPRNG not being used correctly")
	}
}

func TestPartialRevealCommitmentVerification(t *testing.T) {
	shuf, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}

	rootCommit := RootCommitHash(shuf.ServerSeed, shuf.Cards)

	revealed := make(map[int]struct {
		Card Card
		Salt [32]byte
	})
	unrevealed := make(map[int][32]byte)

	// Reveal community cards (e.g., indices 0, 1, 2)
	for i := 0; i < 3; i++ {
		revealed[i] = struct {
			Card Card
			Salt [32]byte
		}{
			Card: shuf.Cards[i],
			Salt: CardSalt(shuf.ServerSeed, i),
		}
	}

	// Keep remaining 49 cards unrevealed (only hashes)
	for i := 3; i < 52; i++ {
		unrevealed[i] = CardHash(shuf.ServerSeed, i, shuf.Cards[i])
	}

	if !VerifyPartial(rootCommit, revealed, unrevealed) {
		t.Fatal("VerifyPartial failed for valid revealed cards and unrevealed commitments")
	}

	// Tamper with a revealed card (ensure rank is actually different)
	tampered := revealed[0]
	if tampered.Card.Rank == Ace {
		tampered.Card.Rank = Two
	} else {
		tampered.Card.Rank = Ace
	}
	revealed[0] = tampered

	if VerifyPartial(rootCommit, revealed, unrevealed) {
		t.Fatal("VerifyPartial passed for tampered card rank")
	}
}
