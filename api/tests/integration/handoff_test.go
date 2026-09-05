//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// TestRequestHandoffDoesNotDisruptAnAlreadyInFlightAction proves the ADR's
// one acceptance criterion that doesn't follow "for free" from an existing
// guarantee (docs/specs/2026-09-05-session-handoff-tableconn.md §5): a
// RequestHandoffCmd racing a genuine ActCmd from the same player's old
// connection must never lose or duplicate that action. The actor's single
// mailbox channel already serializes every command for one table into one
// arrival order, whichever it turns out to be — this test fires both
// concurrently (no artificial synchronization forcing one ahead of the
// other, which would only prove the easy ordering) and asserts the fold
// applied exactly once either way: chip conservation holds and the folded
// seat's state reflects one fold, not two, not zero.
func TestRequestHandoffDoesNotDisruptAnAlreadyInFlightAction(t *testing.T) {
	db := testDynamoClient(t)
	store := tablestore.NewStore(db, "flow_test")
	mustCreatePokerTables(t, db, "flow_test")
	tableID := uniqueTableID(t)
	leaseBackend := cache.NewMemoryBackend(16)

	mgr := tablemanager.NewManager(tablelease.NewService(leaseBackend), store, nil, nil)
	seed := func() *hand.Table {
		return hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	}
	actor, err := mgr.GetOrCreateActor(context.Background(), tableID, seed)
	if err != nil {
		t.Fatalf("acquire actor: %v", err)
	}

	if err := actor.Dispatch(table.ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("start hand: %v", err)
	}
	st, err := store.LoadTable(context.Background(), tableID)
	if err != nil || st.State.Stage != hand.PreFlop {
		t.Fatalf("expected the hand to start, got %+v err=%v", st, err)
	}
	before := hand.NewTableFromState(st.State)
	current := before.CurrentPlayerIDForActor()
	if current == "" {
		t.Fatal("expected a current player to act preflop")
	}

	actReply := make(chan error, 1)
	handoffReply := make(chan error, 1)
	actDone := make(chan error, 1)
	go func() {
		actDone <- actor.Dispatch(table.ActCmd{
			PlayerID: current, ActionID: "handoff-race-act", Action: betting.ActionFold, Reply: actReply,
		})
	}()
	if err := actor.Dispatch(table.RequestHandoffCmd{
		PlayerID: current, NewConnID: "new-conn", Reply: handoffReply,
	}); err != nil {
		t.Fatalf("RequestHandoffCmd: %v", err)
	}

	select {
	case err := <-actDone:
		if err != nil {
			t.Fatalf("queued Act must still commit despite the concurrent handoff: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Act never completed — appears lost behind the handoff")
	}

	final, err := store.LoadTable(context.Background(), tableID)
	if err != nil {
		t.Fatalf("load table: %v", err)
	}
	tbl := hand.NewTableFromState(final.State)
	folded := findPlayer(tbl, current)
	if folded == nil || folded.State != hand.Folded {
		t.Fatalf("expected %s folded exactly once, got %+v", current, folded)
	}
	var total int64
	for _, p := range tbl.PlayersForActor() {
		total += p.Stack
	}
	// Chip conservation is the surest sign the fold applied exactly once: a
	// lost Act would leave chips uncommitted from the current betting round,
	// and a duplicated one has no way to apply twice without desyncing the
	// pot/stack math the engine already enforces per-action.
	if total != 2000 {
		t.Fatalf("chip conservation broken: stacks sum to %d, want 2000", total)
	}
}
