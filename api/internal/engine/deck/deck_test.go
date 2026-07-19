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

func TestVerifySucceedsForGenuineReveal(t *testing.T) {
	result, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}
	if !Verify(result.ServerSeed, result.Cards, result.CommitHash) {
		t.Fatal("Verify should succeed for a genuine seed/deck/hash triple")
	}
}

func TestVerifyFailsIfDeckWasTamperedWith(t *testing.T) {
	result, err := NewShuffle()
	if err != nil {
		t.Fatalf("NewShuffle: %v", err)
	}
	tampered := result.Cards
	tampered[0], tampered[1] = tampered[1], tampered[0]
	if Verify(result.ServerSeed, tampered, result.CommitHash) {
		t.Fatal("Verify should fail when the revealed deck doesn't match the committed hash")
	}
}

func TestTwoShufflesProduceDifferentSeeds(t *testing.T) {
	a, _ := NewShuffle()
	b, _ := NewShuffle()
	if a.ServerSeed == b.ServerSeed {
		t.Fatal("two independent shuffles produced the same seed — CSPRNG not being used correctly")
	}
}
