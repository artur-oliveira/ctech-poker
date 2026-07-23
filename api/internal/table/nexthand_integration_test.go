//go:build integration

package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestHandCompleteAutoStartsNextHandAfterDelay(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a := newTestActor(t, store)
	a.nextHandDelay = 20 * time.Millisecond

	ctx := context.Background()
	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	stored, _ := store.LoadTable(ctx, "table-1")
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	reply3 := make(chan error, 1)
	_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply3})

	stored, _ = store.LoadTable(ctx, "table-1")
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected hand to reach Complete after fold-to-one, got %v", stored.State.Stage)
	}
	handIDAfterFold := stored.HandID

	time.Sleep(50 * time.Millisecond)

	stored, _ = store.LoadTable(ctx, "table-1")
	if stored.HandID == handIDAfterFold {
		t.Fatal("expected a new hand to have started automatically after the 5s (here 20ms) delay")
	}
}
