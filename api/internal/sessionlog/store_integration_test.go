//go:build integration

package sessionlog

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func uniqueSessionPlayerID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
}

func TestFindOpenSessionReturnsTheMostRecentUnclosedSessionForTable(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)
	_ = store.RecordSession(ctx, SessionItem{PK: playerID, TableID: "t1", JoinedAt: 1})
	_ = store.RecordSession(ctx, SessionItem{PK: playerID, TableID: "t2", JoinedAt: 2})

	open, err := store.FindOpenSession(ctx, playerID, "t2")
	if err != nil {
		t.Fatalf("FindOpenSession: %v", err)
	}
	if open == nil || open.TableID != "t2" {
		t.Fatalf("expected the open session for t2, got %+v", open)
	}
}

func TestFindLatestOpenSessionReconcilesAcrossTables(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)
	if err := store.RecordSession(ctx, SessionItem{PK: playerID, SK: "1", TableID: "closed", JoinedAt: 1, EndedAt: 2}); err != nil {
		t.Fatal(err)
	}
	open, err := store.FindLatestOpenSession(ctx, playerID)
	if err != nil || open != "" {
		t.Fatalf("open=%v err=%v", open, err)
	}
	if err := store.RecordSession(ctx, SessionItem{PK: playerID, SK: "2", TableID: "open", JoinedAt: 3}); err != nil {
		t.Fatal(err)
	}
	open, err = store.FindLatestOpenSession(ctx, playerID)
	if err != nil || open == "" {
		t.Fatalf("open=%v err=%v", open, err)
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
	playerID := uniqueSessionPlayerID(t)

	_ = store.RecordSession(ctx, SessionItem{PK: playerID, TableID: "old-table", JoinedAt: 1})

	for i := 0; i < 60; i++ {
		item := SessionItem{PK: playerID, TableID: "other-table", JoinedAt: 2, EndedAt: 3}
		if err := store.RecordSession(ctx, item); err != nil {
			t.Fatalf("seed session %d: %v", i, err)
		}
	}

	open, err := store.FindOpenSession(ctx, playerID, "old-table")
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
	playerID := uniqueSessionPlayerID(t)
	_ = store.RecordSession(ctx, SessionItem{PK: playerID, SK: "fixed", TableID: "t1", JoinedAt: 1, BuyinAmount: 500})

	open, _ := store.FindOpenSession(ctx, playerID, "t1")
	open.EndedAt = 99
	open.CashoutAmount = 700
	open.NetPnL = 200
	if err := store.CloseSession(ctx, *open); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	sessions, _, _ := store.ListSessions(ctx, playerID, 10, nil)
	if len(sessions) != 1 {
		t.Fatalf("expected the close to overwrite, not append — got %d items", len(sessions))
	}
	if sessions[0].EndedAt != 99 {
		t.Fatal("expected the overwritten item to carry EndedAt")
	}
}

// TestCloseSessionRefreshesTTL pins down that closing a long-open session
// resets its TTL from close time, not join time — CloseSession forwards the
// item's original (open-time) TTL to RecordSession, whose "default only if
// zero" guard would otherwise leave it unchanged, letting a session that sat
// open for most of sessionTTLDays expire right after (or even before) close.
func TestCloseSessionRefreshesTTL(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)
	staleTTL := time.Now().Add(time.Hour).Unix()
	_ = store.RecordSession(ctx, SessionItem{PK: playerID, SK: "fixed", TableID: "t1", JoinedAt: 1, TTL: staleTTL})

	open, _ := store.FindOpenSession(ctx, playerID, "t1")
	open.EndedAt = 99
	if err := store.CloseSession(ctx, *open); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	sessions, _, _ := store.ListSessions(ctx, playerID, 10, nil)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].TTL <= staleTTL {
		t.Fatalf("expected TTL refreshed past the stale open-time value %d, got %d", staleTTL, sessions[0].TTL)
	}
}
