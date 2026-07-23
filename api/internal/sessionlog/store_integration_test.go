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
