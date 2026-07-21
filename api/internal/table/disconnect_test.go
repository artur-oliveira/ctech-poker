//go:build integration

package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestDisconnectAutoFoldsAtActionDeadline(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a := newTestActor(t, store)
	a.actionDeadline = 20 * time.Millisecond

	ctx := context.Background()
	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	stored, _ := store.LoadTable(ctx, "table-1")
	var toAct string
	for _, s := range stored.State.Players {
		if hand.NewTableFromState(stored.State).CurrentPlayerCanActForActor(s.ID) {
			toAct = s.ID
			break
		}
	}
	if toAct == "" {
		t.Fatal("no player found with action to act")
	}
	reply3 := make(chan error, 1)
	_ = a.Dispatch(DisconnectCmd{PlayerID: toAct, Reply: reply3})

	time.Sleep(50 * time.Millisecond)

	stored, _ = store.LoadTable(ctx, "table-1")
	for _, s := range stored.State.Players {
		if s.ID == toAct && s.State != hand.Folded {
			t.Fatalf("expected %s to be auto-folded after missing its action deadline, got state %v", toAct, s.State)
		}
	}
}
