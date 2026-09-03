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

// TestAddBuyinAccumulatesAcrossRebuys pins down the responsible-gaming
// correctness fix (#70): a session's buyin_amount must be the SUM of every
// rebuy, not just the last write, and the atomic ADD must not clobber the
// item's other fields (table_id, joined_at).
func TestAddBuyinAccumulatesAcrossRebuys(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)

	if err := store.RecordSession(ctx, SessionItem{PK: playerID, SK: "fixed", TableID: "t1", JoinedAt: 1, BuyinAmount: 400}); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}

	if err := store.AddBuyin(ctx, playerID, "fixed", 200, "rebuy-1"); err != nil {
		t.Fatalf("AddBuyin rebuy 1: %v", err)
	}
	if err := store.AddBuyin(ctx, playerID, "fixed", 300, "rebuy-2"); err != nil {
		t.Fatalf("AddBuyin rebuy 2: %v", err)
	}

	open, err := store.FindOpenSession(ctx, playerID, "t1")
	if err != nil || open == nil {
		t.Fatalf("expected an open session, got %+v err=%v", open, err)
	}
	if open.BuyinAmount != 900 {
		t.Fatalf("expected cumulative buyin 400+200+300=900, got %d", open.BuyinAmount)
	}
	if open.TableID != "t1" || open.JoinedAt != 1 {
		t.Fatalf("expected the ADD to leave other fields untouched, got %+v", open)
	}
}

// TestAddBuyinIsIdempotentPerKey pins down that retrying the exact same
// buy-in (same idemKey — a client resubmit, or the auto-rebuy sweep
// double-firing for one hand) can never double-count the rebuy.
func TestAddBuyinIsIdempotentPerKey(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)

	if err := store.RecordSession(ctx, SessionItem{PK: playerID, SK: "fixed", TableID: "t1", JoinedAt: 1, BuyinAmount: 100}); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := store.AddBuyin(ctx, playerID, "fixed", 500, "dup-key"); err != nil {
			t.Fatalf("AddBuyin attempt %d: %v", i, err)
		}
	}

	open, err := store.FindOpenSession(ctx, playerID, "t1")
	if err != nil || open == nil {
		t.Fatalf("expected an open session, got %+v err=%v", open, err)
	}
	if open.BuyinAmount != 600 {
		t.Fatalf("expected the duplicate calls to add 500 exactly once (100+500=600), got %d", open.BuyinAmount)
	}
}

// TestAddBuyinRejectsAlreadyClosedSession pins down that a rebuy losing a
// race with a concurrent cash-out never reopens (or silently lands inside)
// an already-closed session's total.
func TestAddBuyinRejectsAlreadyClosedSession(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)

	if err := store.RecordSession(ctx, SessionItem{PK: playerID, SK: "fixed", TableID: "t1", JoinedAt: 1, BuyinAmount: 400, EndedAt: 99, CashoutAmount: 0, NetPnL: -400}); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}

	if err := store.AddBuyin(ctx, playerID, "fixed", 200, "late-rebuy"); err != nil {
		t.Fatalf("AddBuyin: %v", err)
	}

	sessions, _, err := store.ListSessions(ctx, playerID, 10, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].BuyinAmount != 400 || sessions[0].EndedAt != 99 {
		t.Fatalf("expected the closed session to be left untouched, got %+v", sessions)
	}
}

// TestAddBuyinGuardRowNeverPollutesPlayerQueries pins down that the
// idempotency guard AddBuyin writes lives in a namespaced partition
// (buyinGuardPK), not the player's own — so it can never surface as a bogus
// session in ListSessions, FindOpenSession, or FindLatestOpenSession.
func TestAddBuyinGuardRowNeverPollutesPlayerQueries(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)

	if err := store.RecordSession(ctx, SessionItem{PK: playerID, SK: "fixed", TableID: "t1", JoinedAt: 1, BuyinAmount: 100}); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}
	if err := store.AddBuyin(ctx, playerID, "fixed", 200, "guard-key"); err != nil {
		t.Fatalf("AddBuyin: %v", err)
	}

	sessions, _, err := store.ListSessions(ctx, playerID, 10, nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected exactly the one real session, got %d: %+v", len(sessions), sessions)
	}

	latest, err := store.FindLatestOpenSession(ctx, playerID)
	if err != nil || latest != "t1" {
		t.Fatalf("expected FindLatestOpenSession to still report t1, got %q err=%v", latest, err)
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
