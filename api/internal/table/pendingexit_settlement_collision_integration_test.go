//go:build integration

// Regression coverage for the "can't leave a table I've already left once"
// wedge: BuildSystemSettlementIntent keyed its create-only poker_pending_cashouts
// row as roomID#playerID#system_leave#reason — constant across every seating.
// The row is co-committed atomically with the seat removal, so the SECOND
// exit_requested at the same table hit the leftover (already-resolved) row's
// attribute_not_exists condition, failed the whole transaction, and left the
// player stuck with pending_exit=true forever (until an idle sweep with a
// different reason happened to catch them). The fix threads a fresh per-removal
// nonce from Actor.newSettlementNonce through to the key.
package table

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestRequestExitStillRemovesWhenAPriorSystemLeaveRowExists(t *testing.T) {
	db := testClient(t)
	env := "pendingexit_collision_test"
	mustCreateTestTables(t, db, env)
	createTestTable(t, db, env+"_poker_pending_cashouts", true)
	store := tablestore.NewStore(db, env)
	pending := reconcile.NewPendingStore(db, env)
	ctx := context.Background()

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20) // stays WaitingForPlayers — no hand, so p1 is never dealt in
	if err := store.SeedTable(ctx, tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Simulate an earlier seating at this same table that p1 already left via
	// request_exit: the recovery row is present and long since resolved.
	legacyKey := tableID + "#p1#system_leave#exit_requested"
	if err := pending.Record(ctx, reconcile.PendingCashout{
		ID: legacyKey, PlayerID: "p1", Amount: 500, CurrencyMode: "sandbox",
		TableRef: tableID, IdempotencyKey: legacyKey,
	}); err != nil {
		t.Fatalf("seed prior pending row: %v", err)
	}
	if err := pending.MarkResolved(ctx, legacyKey); err != nil {
		t.Fatalf("resolve prior pending row: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	defer stopActor(t, a, cancel)

	var gotNonce string
	a.SetSystemSettlementIntentForActor(func(_ context.Context, playerID, reason, nonce string, stack int64, _ string) (types.TransactWriteItem, error) {
		gotNonce = nonce
		key := tableID + "#" + playerID + "#system_leave#" + reason
		if nonce != "" {
			key += "#" + nonce
		}
		return pending.BuildRecordTx(reconcile.PendingCashout{
			ID: key, PlayerID: playerID, Amount: stack, CurrencyMode: "sandbox",
			TableRef: tableID, IdempotencyKey: key,
		})
	})

	if err := a.Dispatch(RequestExitCmd{PlayerID: "p1", ActionID: "exit-1", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("RequestExitCmd: %v", err)
	}

	if gotNonce == "" {
		t.Fatal("actor did not pass a per-removal settlement nonce")
	}

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	for _, p := range stored.State.Players {
		if p.ID == "p1" {
			t.Fatalf("p1 still seated after request_exit — the co-committed settlement row collided with the stale one: %+v", p)
		}
	}
	if len(stored.State.Players) != 1 || stored.State.Players[0].ID != "p2" {
		t.Fatalf("expected only p2 to remain, got %+v", stored.State.Players)
	}

	// The obligation for THIS removal was recorded under its own nonce'd key,
	// separate from the stale one.
	row, err := pending.Get(ctx, tableID+"#p1#system_leave#exit_requested#"+gotNonce)
	if err != nil {
		t.Fatalf("get new pending row: %v", err)
	}
	if row == nil {
		t.Fatal("expected a fresh nonce-keyed pending-cashout row for this removal")
	}
}
