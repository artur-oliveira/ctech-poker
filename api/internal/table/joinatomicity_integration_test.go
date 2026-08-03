//go:build integration

// Regression coverage for the same bug class fixed 2026-08-03 in
// tablestore.CommitAction (docs/plans/2026-08-03-leave-settlement-atomicity.md):
// a handler mutates a.cached in place before attempting the DynamoDB commit,
// so a commit failure that ISN'T a version conflict (retryOnConflict/
// ensureLoaded already reload-and-retry on that one) must roll the in-memory
// mutation back, or the next unrelated successful commit silently persists a
// half-applied change with no matching poker_action_log entry. Leave already
// had this guard (applyLeaveAndCommit); Join did not.
package table

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// TestJoinRollsBackCacheOnNonConflictCommitFailure forces applyJoinAndCommit's
// final a.commit(...) call to fail for a reason that is NOT a version
// conflict — the extra settlement TransactWriteItem targets a DynamoDB table
// that was never created, so the whole TransactWriteItems call comes back
// with a real AWS error (ResourceNotFound), exactly the shape of the
// production incident (an extra item failing due to a permissions gap on its
// own table). Asserts the phantom joiner never lands in the persisted state
// and does not leak into the actor's in-memory cache either.
func TestJoinRollsBackCacheOnNonConflictCommitFailure(t *testing.T) {
	db := testClient(t)
	env := "join_atomicity_test"
	mustCreateTestTables(t, db, env)
	store := tablestore.NewStore(db, env)
	ctx := context.Background()

	tableID := uniqueTableID(t)
	seed := hand.NewTable(nil, 10, 20)
	if err := store.SeedTable(ctx, tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed table: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	if err := a.ensureLoaded(ctx, true); err != nil {
		t.Fatalf("load actor: %v", err)
	}

	// Points at a table that was never created in DynamoDB Local, so the
	// transaction this item rides in always fails — not with ErrVersionConflict.
	missing := dynamo.NewBase(db, env, "table_intentionally_missing")
	failingItem, err := dynamo.Encode(struct {
		PK string `dynamodbav:"pk"`
	}{PK: "settlement-1"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	failingTx := missing.BuildPutTxItem(failingItem)

	err = a.handleJoin(ctx, JoinCmd{
		PlayerID: "ghost", Stack: 5000,
		SettlementIntent: func() (types.TransactWriteItem, error) { return failingTx, nil },
	})
	if err == nil {
		t.Fatal("expected the join commit to fail against a missing settlement table")
	}
	if errors.Is(err, tablestore.ErrVersionConflict) {
		t.Fatalf("test setup bug: got ErrVersionConflict, not the intended commit failure: %v", err)
	}

	for _, p := range a.cached.PlayersForActor() {
		if p.ID == "ghost" {
			t.Fatal("phantom joiner still present in actor cache after a failed, non-version-conflict commit — rollback missing")
		}
	}

	loaded, err := store.LoadTable(ctx, tableID)
	if err != nil || loaded == nil {
		t.Fatalf("load table: %v", err)
	}
	for _, p := range loaded.State.Players {
		if p.ID == "ghost" {
			t.Fatal("phantom joiner persisted to DynamoDB despite the commit having failed")
		}
	}

	// A subsequent, unrelated successful join must not resurrect the ghost —
	// this is the exact way the bug would have manifested: the next
	// successful commit silently persisting the earlier failed mutation.
	if err := a.handleJoin(ctx, JoinCmd{PlayerID: "real-player", Stack: 1000}); err != nil {
		t.Fatalf("second (legitimate) join: %v", err)
	}
	loaded, err = store.LoadTable(ctx, tableID)
	if err != nil || loaded == nil {
		t.Fatalf("load table after second join: %v", err)
	}
	if len(loaded.State.Players) != 1 || loaded.State.Players[0].ID != "real-player" {
		t.Fatalf("expected only real-player seated after recovery, got %+v", loaded.State.Players)
	}
}
