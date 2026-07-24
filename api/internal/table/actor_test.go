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

// newTestActor seeds a fresh 2-player table under a tableID derived from the
// calling test's name (t.Name()), never a shared literal — tablestore.SeedTable
// is put-if-absent, so a hardcoded ID silently reuses whatever state a
// PREVIOUSLY run test left behind against a persistent DynamoDB Local
// instance, instead of the fresh state this test thinks it just seeded.
func newTestActor(t *testing.T, store *tablestore.Store) (*Actor, string) {
	t.Helper()
	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	if err := store.SeedTable(context.Background(), tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)
	return a, tableID
}

func TestActorCommitsReadyThenAct(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActor(t, store)

	reply := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply}); err != nil {
		t.Fatalf("ready p1: %v", err)
	}
	reply2 := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ready p2: %v", err)
	}

	stored, err := store.LoadTable(context.Background(), tableID)
	if err != nil || stored == nil || stored.State.Stage == hand.WaitingForPlayers {
		t.Fatalf("expected hand to have started and committed, got %+v err=%v", stored, err)
	}

	seat := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: seat, ActionID: "a1", Action: betting.ActionCall, Reply: reply3}); err != nil {
		t.Fatalf("act: %v", err)
	}

	stored, err = store.LoadTable(context.Background(), tableID)
	if err != nil || stored.Version < 3 {
		t.Fatalf("expected version to have advanced past ready+ready+act, got %+v err=%v", stored, err)
	}
}

func TestActorRecoversFromVersionConflictAndRetriesOnce(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActor(t, store)

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	stored, _ := store.LoadTable(context.Background(), tableID)
	_ = store.CommitAction(context.Background(), tableID, stored.HandID, "", stored.Version, stored.State, tablestore.ActionLogEntry{TableID: tableID, HandID: stored.HandID, Version: stored.Version + 1})

	seat := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: seat, ActionID: "a1", Action: betting.ActionCall, Reply: reply3}); err != nil {
		t.Fatalf("expected the Actor to reload and retry past the version conflict, got: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
}

// TestReadyFalseMarksSittingOutAndReadyTrueReturnsFree seeds a 4-handed table
// with a fixed dealer (p1) so the projected SB/BB (p2, p3) are deterministic —
// p4 is neither, so its return from sitting-out must be free and immediate.
func TestReadyFalseMarksSittingOutAndReadyTrueReturnsFree(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
		{ID: "p4", Stack: 1000, Ready: true},
	}, 10, 20)
	state := seed.ExportState()
	state.DealerSeat = 0
	state.DealerDrawn = true
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go a.Run(runCtx)

	reply := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p4", Ready: false, Reply: reply}); err != nil {
		t.Fatalf("ReadyCmd(false): %v", err)
	}
	stored, _ := store.LoadTable(ctx, tableID)
	for _, s := range stored.State.Players {
		if s.ID == "p4" && s.State != hand.SittingOut {
			t.Fatalf("expected p4 to be SittingOut after ready:false, got %v", s.State)
		}
	}

	reply2 := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p4", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ReadyCmd(true): %v", err)
	}
	stored, _ = store.LoadTable(ctx, tableID)
	for _, s := range stored.State.Players {
		if s.ID == "p4" && s.State == hand.SittingOut {
			t.Fatal("expected p4's free return (not projected SB/BB) to clear SittingOut immediately")
		}
	}
}

func TestShowCardsCmdRevealsFoldedWinnerToEveryone(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActor(t, store)
	ctx := context.Background()

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})

	stored, _ := store.LoadTable(ctx, tableID)
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply3}); err != nil {
		t.Fatalf("fold: %v", err)
	}

	reply4 := make(chan error, 1)
	if err := a.Dispatch(ShowCardsCmd{PlayerID: winnerID, Reply: reply4}); err != nil {
		t.Fatalf("ShowCardsCmd: %v", err)
	}
	stored, _ = store.LoadTable(ctx, tableID)
	table := hand.NewTableFromState(stored.State)
	view := table.ViewFor(toAct)
	for _, s := range view.Seats {
		if s.PlayerID == winnerID && len(s.HoleCards) != 2 {
			t.Fatal("expected winner's cards visible to the other player after ShowCardsCmd")
		}
	}
}

func TestOnHandCompleteReceivesNonEmptyHandID(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActor(t, store)
	var gotHandID string
	a.SetOnHandCompleteForActor(func(handID string, outcome hand.HandOutcome) {
		gotHandID = handID
	})

	reply := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply})
	reply2 := make(chan error, 1)
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2})
	stored, _ := store.LoadTable(context.Background(), tableID)
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	reply3 := make(chan error, 1)
	_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: reply3})

	if gotHandID == "" {
		t.Fatal("expected onHandComplete to receive a non-empty handID")
	}
}
