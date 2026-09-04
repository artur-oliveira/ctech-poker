//go:build integration

package sessionlog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/dynamo"
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

// TestFindOpenSessionCostsOneQueryBehindALargeHistory pins down #224: the
// lookup used to page the player's ENTIRE session partition with a non-key
// FilterExpression, so its cost grew with 30 days of history and an old
// table's still-open session could be pushed past the window entirely,
// leaving ended_at stuck at 0 forever. It now reads the sparse open-session
// index: exactly one Query, whatever the history behind it.
func TestFindOpenSessionCostsOneQueryBehindALargeHistory(t *testing.T) {
	store, queries := newCountingTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)

	_ = store.RecordSession(ctx, SessionItem{PK: playerID, TableID: "old-table", JoinedAt: 1})

	for i := 0; i < 200; i++ {
		item := SessionItem{PK: playerID, TableID: "other-table", JoinedAt: 2, EndedAt: 3}
		if err := store.RecordSession(ctx, item); err != nil {
			t.Fatalf("seed session %d: %v", i, err)
		}
	}

	queries.Store(0)
	open, err := store.FindOpenSession(ctx, playerID, "old-table")
	if err != nil {
		t.Fatalf("FindOpenSession: %v", err)
	}
	if open == nil || open.TableID != "old-table" {
		t.Fatalf("expected to still find old-table's open session behind 200 closed sessions, got %+v", open)
	}
	if n := queries.Load(); n != 1 {
		t.Fatalf("expected exactly 1 Query, got %d", n)
	}

	queries.Store(0)
	latest, err := store.FindLatestOpenSession(ctx, playerID)
	if err != nil || latest != "old-table" {
		t.Fatalf("FindLatestOpenSession = %q err=%v", latest, err)
	}
	if n := queries.Load(); n != 1 {
		t.Fatalf("expected exactly 1 Query for FindLatestOpenSession, got %d", n)
	}

	queries.Store(0)
	seen, err := store.HasSessionAtTable(ctx, playerID, "other-table")
	if err != nil || !seen {
		t.Fatalf("HasSessionAtTable = %v err=%v", seen, err)
	}
	if n := queries.Load(); n != 1 {
		t.Fatalf("expected exactly 1 Query for HasSessionAtTable, got %d", n)
	}
}

// TestCloseSessionEvictsFromTheOpenIndex pins down the other half of the
// sparse index's contract: closing a session must drop open_table_id, so the
// row leaves gsi_open_table and neither finder reports the player as still
// seated. Without that, an open-session lookup would answer from a stale
// index entry forever.
func TestCloseSessionEvictsFromTheOpenIndex(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)

	if err := store.RecordSession(ctx, SessionItem{PK: playerID, SK: "fixed", TableID: "t1", JoinedAt: 1}); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}
	open, err := store.FindOpenSession(ctx, playerID, "t1")
	if err != nil || open == nil {
		t.Fatalf("expected the open session, got %+v err=%v", open, err)
	}

	open.EndedAt = 99
	if err := store.CloseSession(ctx, *open); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	if closed, err := store.FindOpenSession(ctx, playerID, "t1"); err != nil || closed != nil {
		t.Fatalf("expected no open session after close, got %+v err=%v", closed, err)
	}
	if latest, err := store.FindLatestOpenSession(ctx, playerID); err != nil || latest != "" {
		t.Fatalf("expected no latest open session after close, got %q err=%v", latest, err)
	}
	// The session itself is still on record — only the open-state index entry
	// went away, so table-scoped authorization keeps working after cash-out.
	if seen, err := store.HasSessionAtTable(ctx, playerID, "t1"); err != nil || !seen {
		t.Fatalf("expected HasSessionAtTable to still see the closed session, got %v err=%v", seen, err)
	}
	if seen, err := store.HasSessionAtTable(ctx, playerID, "never-played"); err != nil || seen {
		t.Fatalf("expected no session at an unvisited table, got %v err=%v", seen, err)
	}
}

// TestBestPublicHandReadsOnlyPublicAttributes pins down #225: the anonymous
// showcase used to read up to 50 FULL HandItems — opponents, seeds, fairness
// maps — to build a six-field summary. It now issues one projected Query
// over the same 50-hand window, so the per-view payload/RCU ceiling is those
// six attributes and nothing private is ever transported.
func TestBestPublicHandReadsOnlyPublicAttributes(t *testing.T) {
	store, queries := newCountingTestStore(t)
	ctx := context.Background()
	playerID := uniqueSessionPlayerID(t)

	for i := 0; i < ShowcaseHandScan; i++ {
		if err := store.RecordHand(ctx, HandItem{
			PK: playerID, CurrencyMode: "sandbox", HandID: fmt.Sprintf("h-%03d", i),
			TableID: "t1", Outcome: "won", NetChange: int64(i), EndedAt: int64(1_700_000_000_000 + i),
			Board: []string{"As", "Kd", "7c"}, HoleCards: []string{"Ah", "Ac"},
			Opponents: []OpponentSummary{{PlayerID: "villain", Name: "Vilão", HoleCards: []string{"2c", "3d"}}},
			ServerSeed: "deadbeef", CommitHash: "cafebabe", RootCommitHash: "f00d",
			RevealedCardSalts:    map[string]RevealedSalt{"0": {Card: "As", SaltHex: "aa"}},
			UnrevealedCardHashes: map[string]string{"7": "bb"},
		}); err != nil {
			t.Fatalf("RecordHand %d: %v", i, err)
		}
	}

	queries.Store(0)
	best, err := store.BestPublicHand(ctx, playerID, "sandbox")
	if err != nil || best == nil {
		t.Fatalf("BestPublicHand: %+v err=%v", best, err)
	}
	if n := queries.Load(); n != 1 {
		t.Fatalf("expected exactly 1 Query for a showcase view, got %d", n)
	}
	if want := int64(ShowcaseHandScan - 1); best.NetChange != want {
		t.Fatalf("best net_change = %d, want %d", best.NetChange, want)
	}
	if best.HandID != fmt.Sprintf("h-%03d", ShowcaseHandScan-1) || best.TableID != "t1" ||
		best.EndedAt != int64(1_700_000_000_000+ShowcaseHandScan-1) ||
		len(best.Board) != 3 || len(best.HoleCards) != 2 {
		t.Fatalf("public fields did not survive the projection: %+v", best)
	}

	// The projection itself is what keeps private attributes out: read the
	// same rows back with it and confirm DynamoDB returns nothing else.
	res, err := store.hands.Query(ctx, dynamo.QueryOpts{
		PK: playerID, SKPrefix: "sandbox#", Limit: ShowcaseHandScan,
		ScanIndexForward: false, ProjectionExpression: publicHandProjection,
	})
	if err != nil {
		t.Fatalf("projected query: %v", err)
	}
	if len(res.Items) != ShowcaseHandScan {
		t.Fatalf("expected %d projected rows, got %d", ShowcaseHandScan, len(res.Items))
	}
	for _, raw := range res.Items {
		if len(raw) != 6 {
			t.Fatalf("expected 6 projected attributes, got %d: %v", len(raw), raw)
		}
		for _, private := range []string{"opponents", "server_seed", "commit_hash", "root_commit_hash", "revealed_card_salts", "unrevealed_card_hashes"} {
			if _, found := raw[private]; found {
				t.Fatalf("private attribute %q reached a showcase read", private)
			}
		}
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
