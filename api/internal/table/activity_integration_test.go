//go:build integration

package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func TestActivityAndPreselectionSurviveFreshActorSnapshot(t *testing.T) {
	db := testClient(t)
	env := "activity_test"
	mustCreateTestTables(t, db, env)
	store := tablestore.NewStore(db, env)
	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000},
	}, 10, 20)
	if err := store.SeedTable(context.Background(), tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go a.Run(ctx)
	stopActor(t, a, cancel)
	for _, playerID := range []string{"p1", "p2"} {
		if err := a.Dispatch(ReadyCmd{PlayerID: playerID, ActionID: "ready-" + playerID, Ready: true, Reply: make(chan error, 1)}); err != nil {
			t.Fatalf("ready %s: %v", playerID, err)
		}
	}
	snapshotCh := make(chan hand.Snapshot, 1)
	if err := a.Dispatch(SnapshotCmd{PlayerID: "p1", Snapshot: snapshotCh, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	initial := <-snapshotCh
	if err := a.Dispatch(ChatCmd{PlayerID: "p1", ActionID: "chat-1", Message: "olá", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if err := a.Dispatch(ReactionCmd{PlayerID: "p1", ActionID: "reaction-1", ReactionID: "coffee", TargetPlayerID: "p2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("reaction: %v", err)
	}
	stored, err := store.LoadTable(context.Background(), tableID)
	if err != nil || stored == nil {
		t.Fatalf("load before preselection: %+v, %v", stored, err)
	}
	if err := a.Dispatch(PreselectCmd{
		PlayerID: "p1", ActionID: "preselect-1", Selection: "check_fold",
		ExpectedSnapshotVersion: uint64(stored.Version), ExpectedHandID: initial.HandID,
		Reply: make(chan error, 1),
	}); err != nil {
		t.Fatalf("preselection: %v", err)
	}

	// A new instance has no in-memory chat/reaction/preselection state. Its
	// forced SnapshotCmd must rebuild all three from the authoritative item.
	b := New(tableID, store, false, nil)
	bctx, bcancel := context.WithCancel(context.Background())
	go b.Run(bctx)
	stopActor(t, b, bcancel)
	restoredCh := make(chan hand.Snapshot, 1)
	if err := b.Dispatch(SnapshotCmd{PlayerID: "p1", Snapshot: restoredCh, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("restored snapshot: %v", err)
	}
	restored := <-restoredCh
	if len(restored.ChatMessages) != 1 || restored.ChatMessages[0].ID != "chat-1" || restored.ChatMessages[0].Message != "olá" {
		t.Fatalf("chat was not restored: %+v", restored.ChatMessages)
	}
	if len(restored.Reactions) != 1 || restored.Reactions[0].ID != "reaction-1" || restored.Reactions[0].ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("reaction was not restored: %+v", restored.Reactions)
	}
	if restored.ActionPreselection != "check_fold" {
		t.Fatalf("preselection was not restored: %q", restored.ActionPreselection)
	}

	actions, err := store.LoadActionsSince(context.Background(), tableID, initial.HandID, 0)
	if err != nil {
		t.Fatalf("load actions: %v", err)
	}
	seen := map[string]tablestore.ActionLogEntry{}
	for _, action := range actions {
		seen[action.ActionID] = action
	}
	if seen["chat-1"].Message != "olá" || seen["reaction-1"].ReactionID != "coffee" ||
		seen["preselect-1"].Selection != "check_fold" {
		t.Fatalf("activity payloads missing from audit log: %+v", seen)
	}
}
