//go:build integration

// Regression coverage for the same "mutate a.cached then only reload on
// ErrVersionConflict" gap fixed 2026-08-03 in handleTurnTimeout's disconnect
// branch, handleNextHand, and handleRunoutStep (docs/plans/2026-08-03-leave-settlement-atomicity.md).
// handleNextHand calls StartHand() — dealing cards, posting blinds, moving
// chips into the pot — directly into a.cached BEFORE the commit; any commit
// error that isn't ErrVersionConflict left that fabricated hand trusted in
// memory, for the next unrelated successful commit to persist as if it had
// really happened.
package table

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// mustCreateTestTablesNoLog is mustCreateTestTables minus poker_action_log —
// every real CommitAction call always includes a log-table write, so a store
// pointed at an env missing that table fails every commit with a real AWS
// error (table not found), never with ErrVersionConflict. That is the
// cheapest way to force the exact non-version-conflict commit failure the
// production incident hit, without needing an extra settlement item.
func mustCreateTestTablesNoLog(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	createTestTable(t, db, env+"_poker_table_state", false)
	createTestTable(t, db, env+"_poker_action_guards", false)
	createTestTable(t, db, env+"_poker_table_state_history", true)
}

func TestNextHandDiscardsFabricatedHandOnNonConflictCommitFailure(t *testing.T) {
	db := testClient(t)
	env := "nexthand_atomicity_test_broken"
	mustCreateTestTablesNoLog(t, db, env)
	store := tablestore.NewStore(db, env)
	ctx := context.Background()

	tableID := uniqueTableID(t)
	table := completedTable(t) // heads-up, p1 folded, Stage == Complete
	if err := store.SeedTable(ctx, tableID, table.ExportState()); err != nil {
		t.Fatalf("seed table: %v", err)
	}
	before, err := store.LoadTable(ctx, tableID)
	if err != nil || before == nil {
		t.Fatalf("load seeded table: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	if err := a.ensureLoaded(ctx, true); err != nil {
		t.Fatalf("load actor: %v", err)
	}

	err = a.handleNextHand(ctx, nextHandCmd{Reply: make(chan error, 1)})
	if err == nil {
		t.Fatal("expected handleNextHand to fail against a store with no action-log table")
	}
	if errors.Is(err, tablestore.ErrVersionConflict) {
		t.Fatalf("test setup bug: got ErrVersionConflict, not the intended commit failure: %v", err)
	}

	if a.cached.Stage() != hand.Complete {
		t.Fatalf("actor cache still holds a fabricated next hand after a failed commit: stage=%v (want Complete, reloaded from the seed)", a.cached.Stage())
	}

	after, err := store.LoadTable(ctx, tableID)
	if err != nil || after == nil {
		t.Fatalf("load table after failed next-hand: %v", err)
	}
	if after.Version != before.Version || after.State.Stage != hand.Complete {
		t.Fatalf("persisted state changed despite the commit having failed: before=%+v after=%+v", before, after)
	}
}
