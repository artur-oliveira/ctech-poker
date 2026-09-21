//go:build integration

package table

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// The drift that actually bit in production is cross-instance: any node can
// serve any table (ARCHITECTURE.md §2), so the peek that bumped the version
// and the act that gets rejected routinely run in different processes. The
// tolerance is therefore only worth anything if gameplay_version round-trips
// through DynamoDB rather than living in one actor's memory.
func TestGameplayVersionSurvivesAFreshActor(t *testing.T) {
	db := testClient(t)
	env := "gameplay_version_test"
	mustCreateTestTables(t, db, env)
	store := tablestore.NewStore(db, env)
	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}, {ID: "p3", Stack: 1000},
	}, 10, 20)
	if err := store.SeedTable(context.Background(), tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go a.Run(ctx)
	stopActor(t, a, cancel)
	for _, playerID := range []string{"p1", "p2", "p3"} {
		if err := a.Dispatch(ReadyCmd{PlayerID: playerID, ActionID: "ready-" + playerID, Ready: true, Reply: make(chan error, 1)}); err != nil {
			t.Fatalf("ready %s: %v", playerID, err)
		}
	}
	snapshotCh := make(chan hand.Snapshot, 1)
	if err := a.Dispatch(SnapshotCmd{PlayerID: "p1", Snapshot: snapshotCh, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	seen := <-snapshotCh
	if seen.CurrentPlayerID == "" {
		t.Fatalf("no hand in progress: %+v", seen.Stage)
	}
	peeker := "p1"
	if peeker == seen.CurrentPlayerID {
		peeker = "p2"
	}

	// The version the acting client is holding is the one it was last told.
	held := seen.SnapshotVersion
	if err := a.Dispatch(PeekCardsCmd{PlayerID: peeker, ActionID: "peek-1", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("peek: %v", err)
	}
	stored, err := store.LoadTable(context.Background(), tableID)
	if err != nil || stored == nil {
		t.Fatalf("load: %+v, %v", stored, err)
	}
	if uint64(stored.Version) == held {
		t.Fatal("peek_cards did not bump the stored version; this test no longer covers the drift")
	}
	if uint64(stored.GameplayVersion) != held {
		t.Fatalf("peek_cards moved gameplay_version: held %d, stored %d", held, stored.GameplayVersion)
	}

	// A different instance, with nothing in memory, must still accept the act.
	b := New(tableID, store, false, nil)
	bctx, bcancel := context.WithCancel(context.Background())
	go b.Run(bctx)
	stopActor(t, b, bcancel)
	if err := b.Dispatch(ActCmd{
		PlayerID: seen.CurrentPlayerID, ActionID: "act-1", Action: betting.ActionFold,
		ExpectedSnapshotVersion: held, ExpectedHandID: seen.HandID, Reply: make(chan error, 1),
	}); err != nil {
		t.Fatalf("act rejected on a fresh instance after a cosmetic bump: %v", err)
	}
}
