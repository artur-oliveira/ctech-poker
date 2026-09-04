//go:build integration

package sessionlog

import (
	"context"
	"fmt"
	"testing"
)

// TestRecordHandsWritesEveryParticipantOnceAndIsReplaySafe covers #200's two
// storage guarantees for the batched hand-history write: a nine-handed hand
// lands all nine per-player rows in one BatchWriteItem, and replaying the
// same completed hand (internal/handhook fails open, so two instances can
// both run the post-hand pipeline) overwrites those same rows instead of
// appending duplicates — the SK is the deterministic mode#hand_id.
func TestRecordHandsWritesEveryParticipantOnceAndIsReplaySafe(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	handID := uniqueSessionPlayerID(t)

	items := make([]HandItem, 0, 9)
	players := make([]string, 0, 9)
	for i := range 9 {
		playerID := fmt.Sprintf("%s-p%d", handID, i)
		players = append(players, playerID)
		items = append(items, HandItem{
			PK: playerID, CurrencyMode: "sandbox", HandID: handID, TableID: "t1",
			Outcome: "lost", NetChange: int64(-i), EndedAt: 1_700_000_000_000,
			Board: []string{"As", "Kd", "7c", "2h", "9s"},
		})
	}

	if err := store.RecordHands(ctx, items); err != nil {
		t.Fatalf("RecordHands: %v", err)
	}
	// Replay the identical hand, as a duplicate pipeline run would.
	if err := store.RecordHands(ctx, items); err != nil {
		t.Fatalf("RecordHands replay: %v", err)
	}

	for i, playerID := range players {
		hands, _, err := store.ListHands(ctx, playerID, "sandbox", 10, nil)
		if err != nil {
			t.Fatalf("ListHands %s: %v", playerID, err)
		}
		if len(hands) != 1 {
			t.Fatalf("%s: expected exactly 1 history row after a replay, got %d", playerID, len(hands))
		}
		if hands[0].HandID != handID || hands[0].NetChange != int64(-i) || len(hands[0].Board) != 5 {
			t.Fatalf("%s: wrong row persisted: %+v", playerID, hands[0])
		}
	}
}
