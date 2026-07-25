package achievements

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

func (m *memStore) ListAchievements(_ context.Context, playerID string, _ int, _ map[string]types.AttributeValue) ([]PlayerAchievementProgress, map[string]types.AttributeValue, error) {
	out := make([]PlayerAchievementProgress, 0, len(m.progress[playerID]))
	for key, count := range m.progress[playerID] {
		out = append(out, PlayerAchievementProgress{Key: key, Count: count})
	}
	return out, nil, nil
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

func TestRecordHandTracksShowdownLossesAndGiantSlayer(t *testing.T) {
	store := &memStore{progress: map[string]map[string]int{}}
	service := NewServiceWithStore(store)
	outcome := hand.HandOutcome{
		Winners:         []string{"p1", "p2"},
		WinningCategory: "flush",
		ComebackWinners: []string{"p1"},
		Participants:    []string{"p1", "p2", "p3", "p4"},
		Contributions:   map[string]int64{"p1": 100, "p2": 500, "p3": 300, "p4": 300},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p3": {HoleCards: [2]string{"Ah", "Ac"}},
			"p4": {HoleCards: [2]string{"Kh", "Kc"}},
		},
		ShowdownResults: map[string]hand.ShowdownResult{
			"p1": {Category: "flush", Won: true, Tied: true},
			"p2": {Category: "flush", Won: true, Tied: true},
			"p3": {Category: "flush", Won: false},      // almost_winner: same category, lost
			"p4": {Category: "full_house", Won: false}, // outranked flush despite full_house: bad_beat + cooler
		},
	}
	if _, err := service.RecordHand(context.Background(), "table-1", outcome); err != nil {
		t.Fatal(err)
	}
	if store.progress["p1"][KeyTied] != 1 || store.progress["p2"][KeyTied] != 1 {
		t.Fatalf("tied winners not counted: p1=%+v p2=%+v", store.progress["p1"], store.progress["p2"])
	}
	if store.progress["p1"][KeyGiantSlayer] != 1 {
		t.Fatalf("p1 beat bigger stacks (p2/p3/p4) while all-in, want giant_slayer: %+v", store.progress["p1"])
	}
	if store.progress["p3"][KeyLooser] != 1 || store.progress["p3"][KeyAlmostWinner] != 1 || store.progress["p3"][KeyCrackedAces] != 1 {
		t.Fatalf("p3 (lost flush vs flush with pocket aces) progress: %+v", store.progress["p3"])
	}
	if store.progress["p3"][KeyBadBeat] != 1 {
		t.Fatalf("p3 lost with a flush (>= three_of_a_kind), want bad_beat=1: %+v", store.progress["p3"])
	}
	if store.progress["p3"][KeyCooler] != 0 {
		t.Fatalf("p3's flush is below full_house's floor, must not count as cooler: %+v", store.progress["p3"])
	}
	if store.progress["p4"][KeyBadBeat] != 1 || store.progress["p4"][KeyCooler] != 1 || store.progress["p4"][KeyAlmostWinner] != 0 {
		t.Fatalf("p4 (lost with full_house, different category) progress: %+v", store.progress["p4"])
	}
	for _, id := range []string{"p1", "p2", "p3", "p4"} {
		if store.progress[id][KeyShowdownWarrior] != 1 {
			t.Fatalf("%s reached showdown, want showdown_warrior=1, got %d", id, store.progress[id][KeyShowdownWarrior])
		}
	}
}

func TestTierCrossedReturnsHighestTierAcrossLargeIncrement(t *testing.T) {
	stars, ok := TierCrossed(KeyWins, 0, 100)
	if !ok || stars != 3 {
		t.Fatalf("got (%d,%v), want (3,true)", stars, ok)
	}
}
