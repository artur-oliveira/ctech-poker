//go:build integration

package sessionlog

import (
	"context"
	"testing"
)

func TestFindOpenSessionReturnsTheMostRecentUnclosedSessionForTable(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_ = store.RecordSession(ctx, SessionItem{PK: "p1", TableID: "t1", JoinedAt: 1})
	_ = store.RecordSession(ctx, SessionItem{PK: "p1", TableID: "t2", JoinedAt: 2})

	open, err := store.FindOpenSession(ctx, "p1", "t2")
	if err != nil {
		t.Fatalf("FindOpenSession: %v", err)
	}
	if open == nil || open.TableID != "t2" {
		t.Fatalf("expected the open session for t2, got %+v", open)
	}
}

// TestFindOpenSessionSurvivesFiftyPlusNewerSessionsElsewhere pins down the
// multi-tabling bug: FindOpenSession used to cap its query at the 50 most
// recent items across the player's ENTIRE history (all tables), so an old
// table's still-open session got pushed out of that window by 50+ newer
// sessions opened/closed at other tables in the meantime — leaving ended_at
// stuck at 0 forever once the player actually left the old table. It must
// now page through the whole partition (filtered server-side to tableID)
// instead of giving up after the first page.
func TestFindOpenSessionSurvivesFiftyPlusNewerSessionsElsewhere(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_ = store.RecordSession(ctx, SessionItem{PK: "p3", TableID: "old-table", JoinedAt: 1})

	for i := 0; i < 60; i++ {
		item := SessionItem{PK: "p3", TableID: "other-table", JoinedAt: 2, EndedAt: 3}
		if err := store.RecordSession(ctx, item); err != nil {
			t.Fatalf("seed session %d: %v", i, err)
		}
	}

	open, err := store.FindOpenSession(ctx, "p3", "old-table")
	if err != nil {
		t.Fatalf("FindOpenSession: %v", err)
	}
	if open == nil || open.TableID != "old-table" {
		t.Fatalf("expected to still find old-table's open session behind 60 newer closed sessions, got %+v", open)
	}
}

func TestCloseSessionOverwritesTheSameItem(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_ = store.RecordSession(ctx, SessionItem{PK: "p2", SK: "fixed", TableID: "t1", JoinedAt: 1, BuyinAmount: 500})

	open, _ := store.FindOpenSession(ctx, "p2", "t1")
	open.EndedAt = 99
	open.CashoutAmount = 700
	open.NetPnL = 200
	if err := store.CloseSession(ctx, *open); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	sessions, _ := store.ListSessions(ctx, "p2", 10)
	if len(sessions) != 1 {
		t.Fatalf("expected the close to overwrite, not append — got %d items", len(sessions))
	}
	if sessions[0].EndedAt != 99 {
		t.Fatal("expected the overwritten item to carry EndedAt")
	}
}
