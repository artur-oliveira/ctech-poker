package table

import (
	"context"
	"strings"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestConnectionIDsMakePresenceIdempotentAcrossTabs(t *testing.T) {
	snapshots := make(chan hand.Snapshot, 16)
	a := New("presence", nil, true, func(_ string, snapshot hand.Snapshot) {
		snapshots <- snapshot
	})
	a.SetCachedForTest(hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000},
		{ID: "p2", Stack: 1000},
	}, 10, 20))

	if err := a.handleConnect(ConnectCmd{PlayerID: "p1", ConnID: "tab-a"}); err != nil {
		t.Fatal(err)
	}
	if err := a.handleConnect(ConnectCmd{PlayerID: "p1", ConnID: "tab-b"}); err != nil {
		t.Fatal(err)
	}
	// Re-registering the same physical connection after actor resolution is
	// idempotent and must not inflate the connection count.
	if err := a.handleConnect(ConnectCmd{PlayerID: "p1", ConnID: "tab-b"}); err != nil {
		t.Fatal(err)
	}
	if err := a.handleDisconnect(DisconnectCmd{PlayerID: "p1", ConnID: "tab-a"}); err != nil {
		t.Fatal(err)
	}
	if _, disconnected := a.disconnectedSince["p1"]; disconnected {
		t.Fatal("closing one tab must not disconnect a player with another live tab")
	}
	if err := a.handleDisconnect(DisconnectCmd{PlayerID: "p1", ConnID: "tab-b"}); err != nil {
		t.Fatal(err)
	}
	if _, disconnected := a.disconnectedSince["p1"]; !disconnected {
		t.Fatal("closing the final tab must mark the player disconnected")
	}
	firstDisconnect := a.disconnectedSince["p1"]
	if err := a.handleDisconnect(DisconnectCmd{PlayerID: "p1", ConnID: "tab-b"}); err != nil {
		t.Fatal(err)
	}
	if !a.disconnectedSince["p1"].Equal(firstDisconnect) {
		t.Fatal("duplicate disconnect must not restart the player's grace period")
	}
	t.Cleanup(func() {
		if timer := a.kickTimers["p1"]; timer != nil {
			timer.Stop()
		}
	})

	var last hand.Snapshot
	for len(snapshots) > 0 {
		last = <-snapshots
	}
	for _, seat := range last.Seats {
		if seat.PlayerID == "p1" && seat.ConnectionState != "disconnected" {
			t.Fatalf("presence not exposed separately in snapshot: %+v", seat)
		}
	}
}

func TestStaleActionPreconditionDoesNotMutateTable(t *testing.T) {
	tableState := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := tableState.StartHand(); err != nil {
		t.Fatal(err)
	}
	a := New("precondition", nil, true, nil)
	a.SetCachedForTest(tableState)
	a.version = 7
	a.handID = "hand-current"
	current := tableState.CurrentPlayerIDForActor()

	err := a.handleAct(context.Background(), ActCmd{
		PlayerID: current, ActionID: "stale",
		ExpectedSnapshotVersion: 6, ExpectedHandID: "hand-old",
		Action: betting.ActionFold,
	})
	if err == nil || !strings.Contains(err.Error(), "stale action state") {
		t.Fatalf("expected stale-state rejection, got %v", err)
	}
	if tableState.CurrentPlayerIDForActor() != current {
		t.Fatal("stale action mutated the betting round")
	}
}
