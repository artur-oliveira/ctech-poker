//go:build integration

package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// ReconnectCmd is dispatched on EVERY inbound WS frame (tablews.go's read
// loop, ahead of every message including a bare ping), so it only broadcasts
// when clearing a genuine disconnect mark this actor instance itself
// recorded (handleReconnect's doc comment) — otherwise every idle ping from
// every seat would flood the whole table with a snapshot for no state
// change. It is NOT how a freshly connected client gets its first snapshot:
// that is tablews.go's own explicit SnapshotCmd dispatch, sent directly to
// that one connection right after the socket opens (tablews.go, "Push the
// current table state to this connection immediately"), independent of
// Reconnect/broadcastAll entirely.
func TestReconnectBroadcastsOnlyAfterAGenuineDisconnect(t *testing.T) {
	db := testClient(t)
	mustCreateTestTables(t, db, "table_test")
	store := tablestore.NewStore(db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	if err := store.SeedTable(context.Background(), tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got := make(chan hand.Snapshot, 8)
	a := New(tableID, store, true, func(_ string, snap hand.Snapshot) { got <- snap })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)

	// A bare reconnect on a fresh actor -- this player was never marked
	// disconnected here -- must NOT broadcast (the ping-flood guard).
	reply := make(chan error, 1)
	if err := a.Dispatch(ReconnectCmd{PlayerID: "p1", Reply: reply}); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	select {
	case snap := <-got:
		t.Fatalf("expected no broadcast for a reconnect with no prior disconnect mark, got %+v", snap)
	case <-time.After(50 * time.Millisecond):
	}

	// Now genuinely disconnect p1 (handleDisconnect marks disconnectedSince
	// and itself broadcasts once), then reconnect -- THIS reconnect must
	// broadcast, since clearDisconnectMark actually cleared something.
	discReply := make(chan error, 1)
	if err := a.Dispatch(DisconnectCmd{PlayerID: "p1", Reply: discReply}); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("expected handleDisconnect itself to broadcast")
	}

	reconnReply := make(chan error, 1)
	if err := a.Dispatch(ReconnectCmd{PlayerID: "p1", Reply: reconnReply}); err != nil {
		t.Fatalf("reconnect after disconnect: %v", err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the reconnect that actually clears a disconnect mark to broadcast")
	}
}
