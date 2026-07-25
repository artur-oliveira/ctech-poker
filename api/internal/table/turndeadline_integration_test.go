//go:build integration

package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// TestReconnectingActorResumesPersistedDeadlineInsteadOfAFreshWindow guards
// the bug behind the "auto_fold_unix resets on F5" report: a client
// reconnecting mid-turn (which may land on a brand-new Actor instance --
// ARCHITECTURE.md §2 lets any node serve any table, with no proxying to
// whichever instance originally armed the timer) must see the SAME action
// deadline it had before reconnecting, not a fresh full turnTimeout window
// granted just because the new instance's own bookkeeping started at zero
// values. StoredTable.TurnDeadlineUnixMs (committed atomically with the
// state that made a player current) is what makes that possible.
func TestReconnectingActorResumesPersistedDeadlineInsteadOfAFreshWindow(t *testing.T) {
	db := testClient(t)
	env := "turndeadline_test"
	mustCreateTestTables(t, db, env)
	store := tablestore.NewStore(db, env)
	tableID := uniqueTableID(t)

	origNow := timeNowFunc
	fakeNow := time.Now()
	timeNowFunc = func() time.Time { return fakeNow }
	t.Cleanup(func() { timeNowFunc = origNow })

	ctx0 := context.Background()
	if err := store.SeedTable(ctx0, tableID, hand.NewTable(nil, 10, 20).ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// a is the instance that originally seats both players -- joining p2
	// auto-starts the hand (tryStartHand runs inside applyJoinAndCommit,
	// before that same call's a.commit persists TurnDeadlineUnixMs).
	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)

	joinPlayer(t, a, "p1", 1000, 9)
	joinPlayer(t, a, "p2", 1000, 9)

	before := loadState(t, store, tableID)
	if before.TurnDeadlineUnixMs == 0 {
		t.Fatalf("expected a turn deadline to be persisted once the hand auto-started, got 0")
	}

	// Time passes -- well under DefaultTurnTimeout -- before the client
	// reconnects and lands on a brand-new instance that never armed this
	// turn's timer itself.
	fakeNow = fakeNow.Add(12 * time.Second)

	b := New(tableID, store, false, func(string, hand.Snapshot) {})
	bctx, bcancel := context.WithCancel(context.Background())
	t.Cleanup(bcancel)
	go b.Run(bctx)

	reply := make(chan error, 1)
	if err := b.Dispatch(ReconnectCmd{PlayerID: "p1", Reply: reply}); err != nil {
		t.Fatalf("reconnect on fresh instance: %v", err)
	}

	if got := b.turnDeadline.UnixMilli(); got != before.TurnDeadlineUnixMs {
		t.Fatalf("fresh instance must resume the persisted deadline %d, got %d (a fresh window would be %d)",
			before.TurnDeadlineUnixMs, got, fakeNow.Add(DefaultTurnTimeout).UnixMilli())
	}
}
