package achievements

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type memStore struct{ progress map[string]map[string]int }

func (m *memStore) Increment(_ context.Context, playerID, key string, by int) (int, int, error) {
	if m.progress[playerID] == nil {
		m.progress[playerID] = map[string]int{}
	}
	previous := m.progress[playerID][key]
	m.progress[playerID][key] += by
	return previous, m.progress[playerID][key], nil
}

func TestRecordHandUpdatesProgressAndUnlocks(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{Winners: []string{"p1"}, WinningCategory: "flush", ComebackWinners: []string{"p1"}, Participants: []string{"p1", "p2"}}
	unlocks, err := service.RecordHand(context.Background(), "table-1", outcome)
	if err != nil {
		t.Fatal(err)
	}
	if store.progress["p1"][KeyWins] != 1 || store.progress["p1"][KeyWinByCategory("flush")] != 1 || store.progress["p1"][KeyComeback] != 1 {
		t.Fatalf("winner progress: %+v", store.progress["p1"])
	}
	if store.progress["p1"][KeyHandsPlayed] != 1 || store.progress["p2"][KeyHandsPlayed] != 1 {
		t.Fatal("participants not counted")
	}
	if len(unlocks) != 3 {
		t.Fatalf("got %d first-tier unlocks, want 3", len(unlocks))
	}
}

func TestTierCrossedReturnsHighestTierAcrossLargeIncrement(t *testing.T) {
	stars, ok := TierCrossed(KeyWins, 0, 100)
	if !ok || stars != 3 {
		t.Fatalf("got (%d,%v), want (3,true)", stars, ok)
	}
}
