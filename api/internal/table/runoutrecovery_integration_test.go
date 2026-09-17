//go:build integration

package table

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// TestAllInRunoutSurvivesAFailedStep is the 2026-09-17 production freeze
// (docs/specs/2026-09-17-frozen-table-runout-and-sitout-fold.md): the flop is
// dealt inside the all-in call, the paced timer fires, its step fails to
// reach a verdict, and the hand sat on a three-card board forever because
// nothing ever re-armed the only scheduler the runout has — no player is on
// the clock mid-runout, so there is no turn timer and no player action to
// carry a recovery.
//
// The store failure is injected the bluntest way DynamoDB Local allows: the
// table item is removed so the step's forced reload fails, then restored. The
// hand must finish on its own, with no new command from any player.
func TestAllInRunoutSurvivesAFailedStep(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")
	stateTable := aws.String("table_test_poker_table_state")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 30, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	state := seed.ExportState()
	state.DealerSeat = 0
	state.DealerDrawn = true
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var mu sync.Mutex
	firstSeenAt := map[int]time.Time{}
	a := New(tableID, store, true, func(_ string, snapshot hand.Snapshot) {
		mu.Lock()
		if _, ok := firstSeenAt[len(snapshot.Board)]; !ok {
			firstSeenAt[len(snapshot.Board)] = time.Now()
		}
		mu.Unlock()
	})
	a.runoutStreetDelay = 400 * time.Millisecond
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	stopActor(t, a, cancel)

	mustDispatch(t, a, ReadyCmd{PlayerID: "p1", Ready: true, Reply: make(chan error, 1)})
	mustDispatch(t, a, ReadyCmd{PlayerID: "p2", Ready: true, Reply: make(chan error, 1)})

	stored, err := store.LoadTable(ctx, tableID)
	if err != nil || stored == nil {
		t.Fatalf("expected a hand to have started, got %+v err=%v", stored, err)
	}
	first := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	mustDispatch(t, a, ActCmd{PlayerID: first, ActionID: "shove", Action: betting.ActionRaise, Amount: 30, Reply: make(chan error, 1)})

	stored, _ = store.LoadTable(ctx, tableID)
	second := hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()

	mustDispatch(t, a, ActCmd{PlayerID: second, ActionID: "call", Action: betting.ActionCall, Reply: make(chan error, 1)})
	mu.Lock()
	_, gotFlop := firstSeenAt[3]
	mu.Unlock()
	if !gotFlop {
		t.Fatal("expected the flop dealt in the same broadcast as the all-in call")
	}

	// Pull the committed table item aside, so the runout step armed by that
	// broadcast cannot reach a verdict — the failure the original code
	// swallowed silently (tablestore reports every TransactionCanceledException
	// as a version conflict, a plain transaction conflict between this
	// instance's two processes included) and then never retried.
	key := map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: tableID}}
	got, err := db.GetItem(ctx, &dynamodb.GetItemInput{TableName: stateTable, Key: key, ConsistentRead: aws.Bool(true)})
	if err != nil || got.Item == nil {
		t.Fatalf("read the committed table item: %+v err=%v", got, err)
	}
	if _, err := db.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: stateTable, Key: key}); err != nil {
		t.Fatalf("delete table item: %v", err)
	}

	// Let at least one step fire and fail against the missing item.
	time.Sleep(3 * a.runoutStreetDelay)
	mu.Lock()
	_, dealtTurn := firstSeenAt[4]
	mu.Unlock()
	if dealtTurn {
		t.Fatal("test is not exercising a failed step: the turn was dealt with no table item to commit against")
	}

	if _, err := db.PutItem(ctx, &dynamodb.PutItemInput{TableName: stateTable, Item: got.Item}); err != nil {
		t.Fatalf("restore table item: %v", err)
	}

	// No further command is dispatched: the bounded re-arm is the only thing
	// that can finish this hand.
	waitForBoardLen(t, &mu, firstSeenAt, 5, 8*time.Second)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored, _ = store.LoadTable(ctx, tableID)
		if hand.NewTableFromState(stored.State).Stage() == hand.Complete {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected the recovered runout to reach Complete, stage is %v",
		hand.NewTableFromState(stored.State).Stage())
}

// TestPausedSeatMidHandStillRunsTheBoardOut replays the 2026-09-17 incident
// through the real command/commit path: the small blind pauses (ready:false)
// and asks to exit while another seat is on the clock, the big blind shoves,
// one seat folds and the button calls the all-in. The paused seat must be
// folded by the auto-fold sweep once the action reaches it — never from
// off-turn — the board must run out in full, and the paused seat must be paid
// nothing from the chips it folded.
func TestPausedSeatMidHandStillRunsTheBoardOut(t *testing.T) {
	db := testClient(t)
	store := tablestore.NewStore(db, "table_test")
	mustCreateTestTables(t, db, "table_test")

	tableID := uniqueTableID(t)
	seed := hand.NewTable([]*hand.Player{
		{ID: "utg", Stack: 529625, Ready: true},
		{ID: "button", Stack: 75000, Ready: true},
		{ID: "sb", Stack: 95900, Ready: true},
		{ID: "bb", Stack: 10275, Ready: true},
	}, 500, 1000)
	state := seed.ExportState()
	state.DealerSeat = 1
	state.DealerDrawn = true
	ctx := context.Background()
	if err := store.SeedTable(ctx, tableID, state); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var mu sync.Mutex
	firstSeenAt := map[int]time.Time{}
	a := New(tableID, store, true, func(_ string, snapshot hand.Snapshot) {
		mu.Lock()
		if _, ok := firstSeenAt[len(snapshot.Board)]; !ok {
			firstSeenAt[len(snapshot.Board)] = time.Now()
		}
		mu.Unlock()
	})
	a.runoutStreetDelay = 300 * time.Millisecond
	runCtx, cancel := context.WithCancel(ctx)
	go a.Run(runCtx)
	stopActor(t, a, cancel)

	for _, id := range []string{"utg", "button", "sb", "bb"} {
		mustDispatch(t, a, ReadyCmd{PlayerID: id, Ready: true, ActionID: "ready-" + id, Reply: make(chan error, 1)})
	}
	current := func() string {
		stored, err := store.LoadTable(ctx, tableID)
		if err != nil || stored == nil {
			t.Fatalf("load: %+v err=%v", stored, err)
		}
		return hand.NewTableFromState(stored.State).CurrentPlayerIDForActor()
	}
	if got := current(); got != "utg" {
		t.Fatalf("expected utg first to act pre-flop, got %q", got)
	}

	// The small blind pauses and asks to exit, both while utg is on the clock.
	mustDispatch(t, a, ReadyCmd{PlayerID: "sb", Ready: false, ActionID: "sb-pause", Reply: make(chan error, 1)})
	mustDispatch(t, a, RequestExitCmd{PlayerID: "sb", ActionID: "sb-exit", Reply: make(chan error, 1)})
	if got := current(); got != "utg" {
		t.Fatalf("a pause from off-turn must not move the action, current is %q", got)
	}

	mustDispatch(t, a, ActCmd{PlayerID: "utg", ActionID: "utg-fold", Action: betting.ActionFold, Reply: make(chan error, 1)})
	mustDispatch(t, a, ActCmd{PlayerID: "button", ActionID: "btn-call", Action: betting.ActionCall, Amount: 1000, Reply: make(chan error, 1)})
	// The sweep runs inside those commits' broadcasts, so by now the paused
	// small blind has been folded on its own turn and the shove is up.
	if got := current(); got != "bb" {
		t.Fatalf("expected the big blind on the clock after the paused seat was swept, got %q", got)
	}
	mustDispatch(t, a, ActCmd{PlayerID: "bb", ActionID: "bb-shove", Action: betting.ActionRaise, Amount: 10275, Reply: make(chan error, 1)})
	if got := current(); got != "button" {
		t.Fatalf("the shove must reopen action for the caller, got %q", got)
	}
	mustDispatch(t, a, ActCmd{PlayerID: "button", ActionID: "btn-allin-call", Action: betting.ActionCall, Amount: 10275, Reply: make(chan error, 1)})

	waitForBoardLen(t, &mu, firstSeenAt, 5, 5*time.Second)
	deadline := time.Now().Add(3 * time.Second)
	for {
		stored, _ := store.LoadTable(ctx, tableID)
		table := hand.NewTableFromState(stored.State)
		if table.Stage() == hand.Complete {
			if payout, ok := table.Payouts()["sb"]; ok && payout != 0 {
				t.Fatalf("the paused seat folded pre-flop and must be paid nothing, got %d", payout)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected the all-in runout to complete, stage is %v", table.Stage())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
