package recentplayers

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type memoryRecentStore struct {
	players  map[string]Player
	recorded []HandCompletion
}

func (s *memoryRecentStore) RecordHand(_ context.Context, hand HandCompletion) error {
	s.recorded = append(s.recorded, hand)
	for _, viewer := range hand.Players {
		for _, opponent := range hand.Players {
			if viewer != opponent {
				s.players[viewer+"#"+opponent] = Player{ViewerPlayerID: viewer, OpponentPlayerID: opponent, LastPlayedAt: hand.PlayedAt.UnixMilli(), HandsTogether: 1}
			}
		}
	}
	return nil
}

func (s *memoryRecentStore) List(_ context.Context, viewer string, _ map[string]types.AttributeValue, _ int) (Page, error) {
	page := Page{Players: []Player{}}
	for _, item := range s.players {
		if item.ViewerPlayerID == viewer {
			page.Players = append(page.Players, item)
		}
	}
	return page, nil
}

type fakeHistory []sessionlog.HandItem

func (h fakeHistory) ListRecentHandsAcrossModes(context.Context, string, int) ([]sessionlog.HandItem, error) {
	return h, nil
}

type fakeBlocks map[string]bool

func (b fakeBlocks) BlockedInEitherDirection(_ context.Context, _ string, ids []string) (map[string]bool, error) {
	result := make(map[string]bool)
	for _, id := range ids {
		result[id] = b[id]
	}
	return result, nil
}

func TestListBootstrapsPlayerScopedHistoryAndFiltersEitherDirectionBlocks(t *testing.T) {
	store := &memoryRecentStore{players: make(map[string]Player)}
	history := fakeHistory{{TableID: "table-1", HandID: "hand-1", EndedAt: time.Now().UnixMilli(), Opponents: []sessionlog.OpponentSummary{{PlayerID: "allowed"}, {PlayerID: "blocked"}}}}
	svc := NewService(store, history, fakeBlocks{"blocked": true})

	page, err := svc.List(context.Background(), "viewer", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 1 || len(store.recorded[0].Players) != 3 {
		t.Fatalf("bootstrap=%+v", store.recorded)
	}
	if len(page.Players) != 1 || page.Players[0].OpponentPlayerID != "allowed" {
		t.Fatalf("page=%+v", page.Players)
	}
}

func TestRecordHandForNinePlayersCoversEveryDirectedPair(t *testing.T) {
	store := &memoryRecentStore{players: make(map[string]Player)}
	players := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}
	if err := NewService(store, nil, nil).RecordHand(context.Background(), "table", "hand", players, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Every viewer still sees all 8 opponents (9x8 relations); what changed in
	// #199 is that they are derived from 9 rows at read time instead of being
	// written as 72 items. The write budget itself is pinned by
	// TestRecordHandWriteBudget.
	if got := len(store.players); got != 72 {
		t.Fatalf("directed pairs=%d want=72", got)
	}
}
