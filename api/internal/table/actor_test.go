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

func newTestActor(t *testing.T, store *tablestore.Store) *Actor {
	t.Helper()
	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	if err := store.SeedTable(context.Background(), "table-1", seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a := New("table-1", store, true, func(string, hand.Snapshot) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)
	return a
}

func TestActorCommitsReadyThenAct(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "test")
	mustCreateTestTables(t, db, "test")
	a := newTestActor(t, store)

	reply := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply}); err != nil {
		t.Fatalf("ready p1: %v", err)
	}
	reply2 := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ready p2: %v", err)
	}

	stored, err := store.LoadTable(context.Background(), "table-1")
	if err != nil || stored == nil || stored.State.Stage == hand.WaitingForPlayers {
		t.Fatalf("expected hand to have started and committed, got %+v err=%v", stored, err)
	}

	var seat string
	for _, s := range stored.State.Players {
		if s.State == hand.Active {
			seat = s.ID
			break
		}
	}
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: seat, ActionID: "a1", Action: betting.ActionCall, Reply: reply3}); err != nil {
		t.Fatalf("act: %v", err)
	}

	stored, err = store.LoadTable(context.Background(), "table-1")
	if err != nil || stored.Version < 3 {
		t.Fatalf("expected version to have advanced past ready+ready+act, got %+v err=%v", stored, err)
	}
}

func TestActorRecoversFromVersionConflictAndRetriesOnce(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "test")
	mustCreateTestTables(t, db, "test")
	a := newTestActor(t, store)

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	stored, _ := store.LoadTable(context.Background(), "table-1")
	_ = store.CommitAction(context.Background(), "table-1", stored.HandID, "", stored.Version, stored.State, tablestore.ActionLogEntry{TableID: "table-1", HandID: stored.HandID, Version: stored.Version + 1})

	var seat string
	for _, s := range stored.State.Players {
		if s.State == hand.Active {
			seat = s.ID
			break
		}
	}
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: seat, ActionID: "a1", Action: betting.ActionCall, Reply: reply3}); err != nil {
		t.Fatalf("expected the Actor to reload and retry past the version conflict, got: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
}
