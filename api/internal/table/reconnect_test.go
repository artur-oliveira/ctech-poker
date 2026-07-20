//go:build integration

package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// A fresh actor has cached==nil until a command loads state. Reconnect (the
// first command a connecting client triggers via its opening ping) must load
// state and broadcast a snapshot — otherwise the client hangs on the loading
// screen forever (no snapshot ever arrives).
func TestReconnectBroadcastsSnapshotOnFreshActor(t *testing.T) {
	db := testClient(t)
	mustCreateTestTables(t, db, "table_test")
	store := tablestore.NewStore(db, "table_test")

	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	if err := store.SeedTable(context.Background(), "table-recon", seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got := make(chan hand.Snapshot, 4)
	a := New("table-recon", store, true, func(_ string, snap hand.Snapshot) { got <- snap })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)

	reply := make(chan error, 1)
	if err := a.Dispatch(ReconnectCmd{PlayerID: "p1", Reply: reply}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if err := <-reply; err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("no snapshot broadcast on reconnect — client would hang on loading screen")
	}
}
