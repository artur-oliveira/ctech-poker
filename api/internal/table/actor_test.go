//go:build integration

package table

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
	go a.Run(ctx)
	stopActor(t, a, cancel)
	return a, tableID
}

// newTestActorSandbox is newTestActor with currencyMode explicitly set to
// "sandbox" — needed for RequestRabbitHuntCmd, which rejects any table whose
// currencyMode isn't "sandbox" (the zero-value newTestActor seeds is "").
func newTestActorSandbox(t *testing.T, store *tablestore.Store) (*Actor, string) {
	t.Helper()
	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000}, {ID: "p2", Stack: 1000}}, 10, 20)
	seed.ConfigureRake("sandbox")
	if err := store.SeedTable(context.Background(), tableID, seed.ExportState()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	ctx, cancel := context.WithCancel(context.Background())
	go a.Run(ctx)
	stopActor(t, a, cancel)
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
	_ = store.CommitAction(context.Background(), tableID, stored.HandID, "", stored.Version, stored.State, stored.Activity, 0, 0, tablestore.ActionLogEntry{TableID: tableID, HandID: stored.HandID, Version: stored.Version + 1})

	seat := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	reply3 := make(chan error, 1)
	if err := a.Dispatch(ActCmd{PlayerID: seat, ActionID: "a1", Action: betting.ActionCall, Reply: reply3}); err != nil {
		t.Fatalf("expected the Actor to reload and retry past the version conflict, got: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
}

func TestDuplicateActionReloadsAuthoritativeStateBeforeBroadcast(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000},
		{ID: "p2", Stack: 1000},
	}, 10, 20)
	if err := store.SeedTable(context.Background(), tableID, seed.ExportState()); err != nil {
		t.Fatal(err)
	}
	broadcastB := make(chan hand.Snapshot, 16)
	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	b := New(tableID, store, true, func(_ string, snapshot hand.Snapshot) { broadcastB <- snapshot })
	ctx, cancel := context.WithCancel(context.Background())
	go a.Run(ctx)
	go b.Run(ctx)
	stopActor(t, a, cancel)
	stopActor(t, b, cancel)

	if err := a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}
	if err := a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}
	snapCh := make(chan hand.Snapshot, 1)
	if err := b.Dispatch(SnapshotCmd{PlayerID: "p1", Snapshot: snapCh, Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}
	stale := <-snapCh
	current := stale.CurrentPlayerID

	cmd := ActCmd{PlayerID: current, ActionID: "same-action", Action: betting.ActionFold, Reply: make(chan error, 1)}
	if err := a.Dispatch(cmd); err != nil {
		t.Fatal(err)
	}
	if err := b.Dispatch(cmd); err != nil {
		t.Fatalf("duplicate must be acknowledged as a successful no-op: %v", err)
	}
	stored, err := store.LoadTable(context.Background(), tableID)
	if err != nil {
		t.Fatal(err)
	}
	var last hand.Snapshot
	for len(broadcastB) > 0 {
		last = <-broadcastB
	}
	if int(last.SnapshotVersion) != stored.Version {
		t.Fatalf("duplicate broadcast used stale version %d; authoritative=%d", last.SnapshotVersion, stored.Version)
	}
	if b.version != stored.Version {
		t.Fatalf("actor cache stayed stale after duplicate: actor=%d store=%d", b.version, stored.Version)
	}
}

func TestSnapshotForcesAuthoritativeReloadAcrossActors(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000},
		{ID: "p2", Stack: 1000},
	}, 10, 20)
	if err := store.SeedTable(context.Background(), tableID, seed.ExportState()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := New(tableID, store, true, nil)
	b := New(tableID, store, true, nil)
	go a.Run(ctx)
	go b.Run(ctx)
	stopActor(t, a, cancel)
	stopActor(t, b, cancel)

	first := make(chan hand.Snapshot, 1)
	if err := b.Dispatch(SnapshotCmd{PlayerID: "p1", Snapshot: first, Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}
	before := <-first
	if err := a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}

	second := make(chan hand.Snapshot, 1)
	if err := b.Dispatch(SnapshotCmd{PlayerID: "p1", Snapshot: second, Reply: make(chan error, 1)}); err != nil {
		t.Fatal(err)
	}
	after := <-second
	if after.SnapshotVersion <= before.SnapshotVersion {
		t.Fatalf("snapshot returned stale cache: before=%d after=%d", before.SnapshotVersion, after.SnapshotVersion)
	}
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
	go a.Run(runCtx)
	stopActor(t, a, cancel)

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

func TestRequestRabbitHuntCmdRevealsOnlyToPayer(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActorSandbox(t, store)
	ctx := context.Background()

	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})

	stored, _ := store.LoadTable(ctx, tableID)
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	if err := a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("fold: %v", err)
	}

	if err := a.Dispatch(RequestRabbitHuntCmd{PlayerID: winnerID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("RequestRabbitHuntCmd: %v", err)
	}

	stored, _ = store.LoadTable(ctx, tableID)
	table := hand.NewTableFromState(stored.State)
	if len(table.ViewFor(winnerID).RunoutCards) == 0 {
		t.Fatal("expected the payer's own view to reveal the runout")
	}
	if len(table.ViewFor(toAct).RunoutCards) != 0 {
		t.Fatal("expected the non-paying viewer's view to stay masked")
	}
}

// winnerCardsActorSetup deals a heads-up hand through the actor, folds the
// player to act, and returns the resulting requester/winner pair.
func winnerCardsActorSetup(t *testing.T) (*Actor, *tablestore.Store, string, string, string) {
	t.Helper()
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActorSandbox(t, store)
	ctx := context.Background()

	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})
	stored, _ := store.LoadTable(ctx, tableID)
	requesterID := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	winnerID := "p2"
	if requesterID == "p2" {
		winnerID = "p1"
	}
	if err := a.Dispatch(ActCmd{PlayerID: requesterID, ActionID: "a1", Action: betting.ActionFold, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("fold: %v", err)
	}
	return a, store, tableID, requesterID, winnerID
}

func TestWinnerCardsConsentFlowChargesOnRequestAndPaysOnAccept(t *testing.T) {
	a, store, tableID, requesterID, winnerID := winnerCardsActorSetup(t)
	ctx := context.Background()

	stored, _ := store.LoadTable(ctx, tableID)
	before := hand.NewTableFromState(stored.State)
	requesterBefore := stackForView(t, before.ViewFor(requesterID), requesterID)
	winnerBefore := stackForView(t, before.ViewFor(requesterID), winnerID)
	rakeBefore := before.RakeCollected()

	if err := a.Dispatch(RequestWinnerCardsCmd{PlayerID: requesterID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("RequestWinnerCardsCmd: %v", err)
	}

	stored, _ = store.LoadTable(ctx, tableID)
	pending := hand.NewTableFromState(stored.State)
	if got := stackForView(t, pending.ViewFor(requesterID), requesterID); got != requesterBefore-20 {
		t.Fatalf("requester stack = %d, want %d", got, requesterBefore-20)
	}
	if got := stackForView(t, pending.ViewFor(requesterID), winnerID); got != winnerBefore {
		t.Fatalf("winner must not be paid before accepting, stack = %d", got)
	}
	if pending.RakeCollected() != rakeBefore {
		t.Fatalf("rake must not move before accepting, got %d", pending.RakeCollected())
	}
	if prompt := pending.ViewFor(winnerID).PendingWinnerCards; prompt == nil || prompt.RequesterID != requesterID {
		t.Fatalf("winner should have been prompted, got %+v", prompt)
	}
	for _, seat := range pending.ViewFor(requesterID).Seats {
		if seat.PlayerID == winnerID && len(seat.HoleCards) == 2 && seat.HoleCards[0] != "back" {
			t.Fatal("nothing may be revealed while consent is pending")
		}
	}

	// The requester must not be able to answer on the winner's behalf.
	if err := a.Dispatch(AcceptWinnerCardsCmd{PlayerID: requesterID, ActionID: "a3", Reply: make(chan error, 1)}); err == nil {
		t.Fatal("expected the requester's own accept to be rejected")
	}

	if err := a.Dispatch(AcceptWinnerCardsCmd{PlayerID: winnerID, ActionID: "a4", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("AcceptWinnerCardsCmd: %v", err)
	}

	stored, _ = store.LoadTable(ctx, tableID)
	table := hand.NewTableFromState(stored.State)
	if got := stackForView(t, table.ViewFor(requesterID), winnerID); got != winnerBefore+10 {
		t.Fatalf("winner stack = %d, want %d", got, winnerBefore+10)
	}
	if table.RakeCollected() != rakeBefore+10 {
		t.Fatalf("rake = %d, want %d", table.RakeCollected(), rakeBefore+10)
	}
	if table.ViewFor(winnerID).PendingWinnerCards != nil {
		t.Fatal("an answered request must be cleared from durable state")
	}
	for _, seat := range table.ViewFor(requesterID).Seats {
		if seat.PlayerID == winnerID && (len(seat.HoleCards) != 2 || seat.HoleCards[0] == "back") {
			t.Fatal("requester should see the winner's cards after consent")
		}
	}
	for _, seat := range table.ViewFor(winnerID).Seats {
		if seat.PlayerID == requesterID && len(seat.HoleCards) != 0 {
			t.Fatal("winner must not gain visibility into the folded requester's cards")
		}
	}
}

func TestWinnerCardsDeclineRefundsTheRequesterAndRevealsNothing(t *testing.T) {
	a, store, tableID, requesterID, winnerID := winnerCardsActorSetup(t)
	ctx := context.Background()

	stored, _ := store.LoadTable(ctx, tableID)
	before := hand.NewTableFromState(stored.State)
	requesterBefore := stackForView(t, before.ViewFor(requesterID), requesterID)
	rakeBefore := before.RakeCollected()

	if err := a.Dispatch(RequestWinnerCardsCmd{PlayerID: requesterID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("RequestWinnerCardsCmd: %v", err)
	}
	if err := a.Dispatch(DeclineWinnerCardsCmd{PlayerID: winnerID, ActionID: "a3", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("DeclineWinnerCardsCmd: %v", err)
	}

	stored, _ = store.LoadTable(ctx, tableID)
	table := hand.NewTableFromState(stored.State)
	if got := stackForView(t, table.ViewFor(requesterID), requesterID); got != requesterBefore {
		t.Fatalf("decline must refund in full, stack = %d want %d", got, requesterBefore)
	}
	if table.RakeCollected() != rakeBefore {
		t.Fatalf("decline must not rake anything, got %d", table.RakeCollected())
	}
	if table.ViewFor(requesterID).PendingWinnerCards != nil {
		t.Fatal("a declined request must be cleared")
	}
	for _, seat := range table.ViewFor(requesterID).Seats {
		if seat.PlayerID == winnerID && len(seat.HoleCards) == 2 && seat.HoleCards[0] != "back" {
			t.Fatal("a declined request must reveal nothing")
		}
	}
}

// The consent window is enforced server-side, not by the client's countdown:
// an expiry commits the refund to durable state so a requester's fee is never
// stranded by a client that simply stopped asking.
func TestWinnerCardsExpiryRefundsTheRequester(t *testing.T) {
	a, store, tableID, requesterID, winnerID := winnerCardsActorSetup(t)
	ctx := context.Background()

	stored, _ := store.LoadTable(ctx, tableID)
	requesterBefore := stackForView(t, hand.NewTableFromState(stored.State).ViewFor(requesterID), requesterID)

	if err := a.Dispatch(RequestWinnerCardsCmd{PlayerID: requesterID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("RequestWinnerCardsCmd: %v", err)
	}

	restore := timeNowFunc
	timeNowFunc = func() time.Time { return restore().Add(hand.WinnerCardsConsentWindow + time.Second) }
	t.Cleanup(func() { timeNowFunc = restore })

	if err := a.Dispatch(expireWinnerCardsCmd{Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("expireWinnerCardsCmd: %v", err)
	}

	stored, _ = store.LoadTable(ctx, tableID)
	table := hand.NewTableFromState(stored.State)
	if got := stackForView(t, table.ViewFor(requesterID), requesterID); got != requesterBefore {
		t.Fatalf("expiry must refund in full, stack = %d want %d", got, requesterBefore)
	}
	if table.ViewFor(requesterID).PendingWinnerCards != nil {
		t.Fatal("an expired request must be cleared")
	}
	// Too late to accept: the window is closed and the fee is already back.
	if err := a.Dispatch(AcceptWinnerCardsCmd{PlayerID: winnerID, ActionID: "a3", Reply: make(chan error, 1)}); err == nil {
		t.Fatal("expected accepting an expired request to be rejected")
	}
}

func stackForView(t *testing.T, view hand.Snapshot, playerID string) int64 {
	t.Helper()
	for _, seat := range view.Seats {
		if seat.PlayerID == playerID {
			return seat.Stack
		}
	}
	t.Fatalf("missing seat for %s", playerID)
	return 0
}

func TestRequestRabbitHuntCmdRejectsDoublePaymentSameHand(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActorSandbox(t, store)
	ctx := context.Background()

	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})
	stored, _ := store.LoadTable(ctx, tableID)
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: make(chan error, 1)})

	if err := a.Dispatch(RequestRabbitHuntCmd{PlayerID: winnerID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("first RequestRabbitHuntCmd: %v", err)
	}
	if err := a.Dispatch(RequestRabbitHuntCmd{PlayerID: winnerID, ActionID: "a3", Reply: make(chan error, 1)}); err == nil {
		t.Fatal("expected the second, distinctly-actioned request this hand to be rejected")
	}
}

func TestRabbitHuntVerifyFailedCmdRefundsAndRemasks(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActorSandbox(t, store)
	ctx := context.Background()

	_ = a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
	_ = a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})
	stored, _ := store.LoadTable(ctx, tableID)
	toAct := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	winnerID := "p1"
	if toAct == "p1" {
		winnerID = "p2"
	}
	_ = a.Dispatch(ActCmd{PlayerID: toAct, ActionID: "a1", Action: betting.ActionFold, Reply: make(chan error, 1)})
	if err := a.Dispatch(RequestRabbitHuntCmd{PlayerID: winnerID, ActionID: "a2", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("RequestRabbitHuntCmd: %v", err)
	}

	stored, _ = store.LoadTable(ctx, tableID)
	var chargedStack int64
	for _, s := range hand.NewTableFromState(stored.State).ViewFor(winnerID).Seats {
		if s.PlayerID == winnerID {
			chargedStack = s.Stack
		}
	}

	if err := a.Dispatch(RabbitHuntVerifyFailedCmd{PlayerID: winnerID, ActionID: "a3", Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("RabbitHuntVerifyFailedCmd: %v", err)
	}

	stored, _ = store.LoadTable(ctx, tableID)
	table := hand.NewTableFromState(stored.State)
	view := table.ViewFor(winnerID)
	for _, s := range view.Seats {
		if s.PlayerID == winnerID && s.Stack != chargedStack+20 {
			t.Fatalf("expected the fee refunded, stack = %d want %d", s.Stack, chargedStack+20)
		}
	}
	if len(view.RunoutCards) != 0 {
		t.Fatal("expected the refunded viewer's view to be masked again")
	}
}

func TestOnHandCompleteReceivesNonEmptyHandID(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, tableID := newTestActor(t, store)
	var gotHandID string
	a.SetOnHandCompleteForActor(func(handID string, outcome hand.HandOutcome, names map[string]string) {
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

func TestHandleReactionRejectsUnknownReactionBeforeSeatCheck(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, _ := newTestActor(t, store)

	reply := make(chan error, 1)
	err := a.Dispatch(ReactionCmd{PlayerID: "not-seated", ActionID: "a1", ReactionID: "not-a-reaction", Reply: reply})
	if err == nil || err.Error() != "table: unknown reaction_id" {
		t.Fatalf("expected unknown reaction_id rejection (not a not-seated error), got %v", err)
	}
}

func TestHandleReactionPremiumNotOwnedRejected(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, _ := newTestActor(t, store)
	a.SetReactionOwnershipForActor(func(context.Context, string, string) (bool, error) { return false, nil })

	reply := make(chan error, 1)
	err := a.Dispatch(ReactionCmd{PlayerID: "p1", ActionID: "a1", ReactionID: "cold", Reply: reply})
	if err == nil || err.Error() != "table: reaction not owned" {
		t.Fatalf("expected reaction-not-owned rejection, got %v", err)
	}
}

func TestHandleReactionPremiumOwnedAcceptedAndMarksUsed(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, _ := newTestActor(t, store)
	a.SetReactionOwnershipForActor(func(context.Context, string, string) (bool, error) { return true, nil })
	markUsedCalls := make(chan string, 1)
	a.SetReactionMarkUsedForActor(func(_ context.Context, playerID, reactionID string) (*types.TransactWriteItem, error) {
		markUsedCalls <- playerID + ":" + reactionID
		return nil, nil
	})

	reply := make(chan error, 1)
	if err := a.Dispatch(ReactionCmd{PlayerID: "p1", ActionID: "a1", ReactionID: "cold", Reply: reply}); err != nil {
		t.Fatalf("expected owned premium reaction to succeed, got %v", err)
	}
	select {
	case got := <-markUsedCalls:
		if got != "p1:cold" {
			t.Fatalf("unexpected markUsed call: %q", got)
		}
	default:
		t.Fatal("expected reactionMarkUsed to have been called")
	}
}

func TestHandleReactionPremiumRejectedWhenConcurrentRefundWins(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, _ := newTestActor(t, store)
	a.SetReactionOwnershipForActor(func(context.Context, string, string) (bool, error) { return true, nil })
	a.SetReactionMarkUsedForActor(func(context.Context, string, string) (*types.TransactWriteItem, error) {
		return nil, errors.New("entitlement was revoked")
	})

	reply := make(chan error, 1)
	err := a.Dispatch(ReactionCmd{PlayerID: "p1", ActionID: "a-refund-race", ReactionID: "cold", Reply: reply})
	if err == nil || !strings.Contains(err.Error(), "build premium reaction usage") {
		t.Fatalf("expected concurrent refund to block reaction, got %v", err)
	}
}

func TestHandleReactionFreeReactionWorksWithoutOwnershipHook(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	a, _ := newTestActor(t, store)
	// reactionOwnership left nil on purpose — a free reaction must never call it.

	reply := make(chan error, 1)
	if err := a.Dispatch(ReactionCmd{PlayerID: "p1", ActionID: "a1", ReactionID: "clap", Reply: reply}); err != nil {
		t.Fatalf("expected free reaction to succeed without an ownership hook, got %v", err)
	}
}

// TestRequestExitAsBlindStillPaysOutOnUncontestedWin exercises the full
// command -> commit -> sweep path: exit requested as the player not
// currently on the clock, the other player folds, the hand completes, and
// the exiting player is both credited the pot AND actually removed by the
// sweep — no second leave call needed. This is also the exact regression
// case that caught the original RequestExit design being wrong (see the
// design doc's "Correction" note): a naive SitOutForActor(playerID) call
// here would fold the "waiting" player immediately since Round.Act has no
// turn-order check, breaking this uncontested win entirely.
func TestRequestExitAsBlindStillPaysOutOnUncontestedWin(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := seed.StartHand(); err != nil {
		t.Fatalf("seed StartHand: %v", err)
	}
	state := seed.ExportState()
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	defer stopActor(t, a, cancel)

	current := seed.CurrentPlayerIDForActor()
	waiting := "p1"
	if current == "p1" {
		waiting = "p2"
	}

	exitReply := make(chan error, 1)
	if err := a.Dispatch(RequestExitCmd{PlayerID: waiting, ActionID: "exit-1", Reply: exitReply}); err != nil {
		t.Fatalf("RequestExitCmd: %v", err)
	}

	actReply := make(chan error, 1)
	// ExpectedSnapshotVersion/ExpectedHandID left zero: validateActionPrecondition
	// treats that as an internal/system-originated act and skips the staleness
	// check (actor.go:836-838) — this test isn't exercising that gate.
	if err := a.Dispatch(ActCmd{
		PlayerID: current, Action: betting.ActionFold, ActionID: "fold-1", Reply: actReply,
	}); err != nil {
		t.Fatalf("ActCmd: %v", err)
	}

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected the hand to complete uncontested, got stage %v", stored.State.Stage)
	}
	if stored.State.Payouts[waiting] == 0 {
		t.Fatalf("expected %s to be credited the uncontested win, got payouts %+v", waiting, stored.State.Payouts)
	}
	// dealtIntoCurrentHand (via handOrder) stays true through the whole
	// Complete-stage window — only the NEXT hand's StartHand clears it — so
	// the sweep cannot remove them until that transition actually runs.
	var stillSeated bool
	for _, p := range stored.State.Players {
		if p.ID == waiting {
			stillSeated = true
		}
	}
	if !stillSeated {
		t.Fatalf("expected %s to still be seated immediately after the hand completes (removal waits for next_hand)", waiting)
	}

	nextHandReply := make(chan error, 1)
	if err := a.Dispatch(nextHandCmd{Reply: nextHandReply}); err != nil {
		t.Fatalf("nextHandCmd: %v", err)
	}

	stored, err = store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable (after next_hand): %v", err)
	}
	for _, p := range stored.State.Players {
		if p.ID == waiting {
			t.Fatalf("expected %s to be swept off the table once the next hand started, still found: %+v", waiting, p)
		}
	}
}

// TestRequestExitOnCurrentActorFoldsImmediately covers the other half of
// RequestExit: when the exiting player IS the one currently on the clock,
// they fold right away (same as a disconnect timeout) rather than waiting
// for processPendingExitAutoFolds — there's no "later turn" to wait for.
func TestRequestExitOnCurrentActorFoldsImmediately(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := seed.StartHand(); err != nil {
		t.Fatalf("seed StartHand: %v", err)
	}
	state := seed.ExportState()
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	defer stopActor(t, a, cancel)

	current := seed.CurrentPlayerIDForActor()
	other := "p1"
	if current == "p1" {
		other = "p2"
	}

	exitReply := make(chan error, 1)
	if err := a.Dispatch(RequestExitCmd{PlayerID: current, ActionID: "exit-1", Reply: exitReply}); err != nil {
		t.Fatalf("RequestExitCmd: %v", err)
	}

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected the hand to complete uncontested, got stage %v", stored.State.Stage)
	}
	if stored.State.Payouts[other] == 0 {
		t.Fatalf("expected %s to be credited the uncontested win, got payouts %+v", other, stored.State.Payouts)
	}

	nextHandReply := make(chan error, 1)
	if err := a.Dispatch(nextHandCmd{Reply: nextHandReply}); err != nil {
		t.Fatalf("nextHandCmd: %v", err)
	}

	stored, err = store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable (after next_hand): %v", err)
	}
	for _, p := range stored.State.Players {
		if p.ID == current {
			t.Fatalf("expected %s to be swept off the table once the next hand started, still found: %+v", current, p)
		}
	}
	var otherFound bool
	for _, p := range stored.State.Players {
		if p.ID == other {
			otherFound = true
		}
	}
	if !otherFound {
		t.Fatalf("expected %s to still be seated", other)
	}
}

// TestPendingExitAutoFoldsOnTurnArrival covers the case RequestExit itself
// deliberately does NOT handle: exit requested while NOT the exiting
// player's turn, then a later commit brings the turn back around to them —
// Actor.processPendingExitAutoFolds (run from broadcastAll) must fold them
// automatically at that point, without a further client action. Heads-up on
// purpose: with only two players, "whoever isn't current" has an
// unambiguous, single, deterministic next turn once current acts.
func TestPendingExitAutoFoldsOnTurnArrival(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := seed.StartHand(); err != nil {
		t.Fatalf("seed StartHand: %v", err)
	}
	state := seed.ExportState()
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	a := New(tableID, store, true, func(string, hand.Snapshot) {})
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	defer stopActor(t, a, cancel)

	current := seed.CurrentPlayerIDForActor()
	waiting := "p1"
	if current == "p1" {
		waiting = "p2"
	}

	exitReply := make(chan error, 1)
	if err := a.Dispatch(RequestExitCmd{PlayerID: waiting, ActionID: "exit-1", Reply: exitReply}); err != nil {
		t.Fatalf("RequestExitCmd: %v", err)
	}

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	for _, p := range stored.State.Players {
		if p.ID == waiting && p.State == hand.Folded {
			t.Fatal("expected the exiting player to still be live — it is not yet their turn")
		}
	}

	// current calls, which in heads-up hands the turn straight to waiting.
	actReply := make(chan error, 1)
	if err := a.Dispatch(ActCmd{
		PlayerID: current, Action: betting.ActionCall, Amount: 20, ActionID: "act-1", Reply: actReply,
	}); err != nil {
		t.Fatalf("ActCmd: %v", err)
	}

	stored, err = store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable (after): %v", err)
	}
	if stored.State.Stage != hand.Complete {
		t.Fatalf("expected the auto-fold to complete the hand uncontested, got stage %v", stored.State.Stage)
	}
	if stored.State.Payouts[current] == 0 {
		t.Fatalf("expected %s to be credited the win after waiting's auto-fold, got payouts %+v", current, stored.State.Payouts)
	}

	nextHandReply := make(chan error, 1)
	if err := a.Dispatch(nextHandCmd{Reply: nextHandReply}); err != nil {
		t.Fatalf("nextHandCmd: %v", err)
	}

	stored, err = store.LoadTable(ctx, tableID)
	if err != nil {
		t.Fatalf("LoadTable (after next_hand): %v", err)
	}
	for _, p := range stored.State.Players {
		if p.ID == waiting {
			t.Fatalf("expected %s to have been auto-folded and then swept off once the next hand started, still found: %+v", waiting, p)
		}
	}
}
