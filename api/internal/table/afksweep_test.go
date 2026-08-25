package table

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/betting"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func lastActionAtFor(table *hand.Table, id string) int64 {
	for _, p := range table.PlayersForActor() {
		if p.ID == id {
			return p.LastActionAt
		}
	}
	return -1
}

// TestGenuineActionRefreshesLastActionButTurnTimeoutFoldDoesNot guards the
// one non-obvious invariant the AFK sweep depends on: a real inbound Act
// bumps LastActionAt, but the server's own turn-timeout auto-fold (driven by
// the same applyActAndCommit path) must not — otherwise a truly-silent
// player's own auto-folds would keep resetting their staleness clock forever.
func TestGenuineActionRefreshesLastActionButTurnTimeoutFoldDoesNot(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer actor.afkSweepTimer.Stop()
	actor.cached = table
	actor.SetEquityEnabledForActor(false)

	current := table.CurrentPlayerIDForActor()
	before := timeNowFunc().UnixMilli()
	if err := actor.handleAct(context.Background(), ActCmd{PlayerID: current, ActionID: "a1", Action: betting.ActionCall, Amount: 0, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("handleAct: %v", err)
	}
	if got := lastActionAtFor(table, current); got < before {
		t.Fatalf("expected genuine action to refresh LastActionAt to >= %d, got %d", before, got)
	}

	other := table.CurrentPlayerIDForActor()
	staleBefore := lastActionAtFor(table, other)
	if err := actor.handleTurnTimeout(context.Background(), turnTimeoutCmd{PlayerID: other, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("handleTurnTimeout: %v", err)
	}
	if staleAfter := lastActionAtFor(table, other); staleAfter != staleBefore {
		t.Fatalf("turn-timeout auto-fold must not refresh LastActionAt: before=%d after=%d", staleBefore, staleAfter)
	}
}

// TestAFKSweepKicksOnlyStalePlayers checks the sweep only removes players
// whose LastActionAt is older than kickGrace, leaving recently-active seats
// alone — and that it runs independent of whose turn it currently is (there
// is no live hand here at all).
func TestAFKSweepKicksOnlyStalePlayers(t *testing.T) {
	now := time.Now()
	table := hand.NewTable([]*hand.Player{
		{ID: "fresh", Stack: 1000, LastActionAt: now.UnixMilli()},
		{ID: "stale", Stack: 1000, LastActionAt: now.Add(-10 * time.Minute).UnixMilli()},
	}, 10, 20)

	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer actor.afkSweepTimer.Stop()
	actor.cached = table
	actor.kickGrace = 5 * time.Minute

	if err := actor.handleAFKSweep(context.Background(), afkSweepCmd{Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("handleAFKSweep: %v", err)
	}
	defer actor.afkSweepTimer.Stop() // handleAFKSweep re-armed a new one

	seated := map[string]bool{}
	for _, p := range table.PlayersForActor() {
		seated[p.ID] = true
	}
	if seated["stale"] {
		t.Fatal("expected the stale player (10 min silent, 5 min grace) to be removed")
	}
	if !seated["fresh"] {
		t.Fatal("fresh player should not have been removed")
	}
}

func TestAFKSweepKeepsSeatWhenSettlementIntentCannotBeBuilt(t *testing.T) {
	now := time.Now()
	game := hand.NewTable([]*hand.Player{
		{ID: "fresh", Stack: 1000, LastActionAt: now.UnixMilli()},
		{ID: "stale", Stack: 1000, LastActionAt: now.Add(-10 * time.Minute).UnixMilli()},
	}, 10, 20)

	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer actor.afkSweepTimer.Stop()
	actor.cached = game
	actor.kickGrace = 5 * time.Minute
	called := false
	actor.SetSystemSettlementIntentForActor(func(context.Context, string, string, int64, string) (types.TransactWriteItem, error) {
		called = true
		return types.TransactWriteItem{}, errors.New("dynamodb unavailable")
	})

	if err := actor.handleAFKSweep(context.Background(), afkSweepCmd{Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("handleAFKSweep: %v", err)
	}
	defer actor.afkSweepTimer.Stop()
	if !called {
		t.Fatal("expected the durable settlement builder to run")
	}
	for _, p := range actor.cached.PlayersForActor() {
		if p.ID == "stale" {
			return
		}
	}
	t.Fatal("stale player's seat disappeared despite settlement-intent failure")
}

func TestNextHandRemovesIdlePlayerBeforeDeal(t *testing.T) {
	now := time.Now()
	game := hand.NewTableFromState(hand.State{
		Players: []*hand.Player{
			{ID: "stale", Stack: 1000, Ready: true, LastActionAt: now.Add(-10 * time.Minute).UnixMilli()},
			{ID: "fresh-1", Stack: 1000, Ready: true, LastActionAt: now.UnixMilli()},
			{ID: "fresh-2", Stack: 1000, Ready: true, LastActionAt: now.UnixMilli()},
		},
		SmallBlind: 10,
		BigBlind:   20,
		Stage:      hand.Complete,
	})
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer actor.afkSweepTimer.Stop()
	actor.cached = game
	actor.kickGrace = 5 * time.Minute
	var removed string
	actor.SetOnPlayerRemovedForActor(func(playerID, reason string, _ int64, _ string) {
		if reason == "idle" {
			removed = playerID
		}
	})

	if err := actor.handleNextHand(context.Background(), nextHandCmd{Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("handleNextHand: %v", err)
	}
	if removed != "stale" {
		t.Fatalf("removed player = %q, want stale", removed)
	}
	for _, p := range game.PlayersForActor() {
		if p.ID == "stale" {
			t.Fatal("stale player was dealt into the next hand")
		}
	}
	if game.Stage() != hand.PreFlop {
		t.Fatalf("stage = %v, want pre_flop", game.Stage())
	}
}

func TestKickTimeoutRearmsWhenPlayerIsStillDealtIn(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "stale", Stack: 1000, Ready: true},
		{ID: "other", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer actor.afkSweepTimer.Stop()
	actor.cached = game
	actor.afkSweepInterval = time.Hour
	actor.disconnectedSince["stale"] = time.Now().Add(-10 * time.Minute)

	err := actor.handleKickTimeout(context.Background(), kickTimeoutCmd{
		PlayerID: "stale",
		Reply:    make(chan error, 1),
	})
	if err == nil {
		t.Fatal("expected removal to be rejected while stale is still dealt in")
	}
	retry := actor.kickTimers["stale"]
	if retry == nil {
		t.Fatal("expected failed kick to arm a retry")
	}
	retry.Stop()
}

// A player whose socket dropped on this instance and reconnected through
// another one leaves this actor's in-memory disconnect mark uncleared — it
// never sees the other instance's connect. The persisted LastActionAt is the
// only evidence that crosses instances, so a player who acted inside
// kickGrace must survive this actor's kick timer, and the stale mark must be
// dropped instead of firing again.
func TestKickTimeoutSparesAPlayerActiveOnAnotherInstance(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "moved", Stack: 1000, Ready: true},
		{ID: "other", Stack: 1000, Ready: true},
	}, 10, 20)
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer func() { actor.afkSweepTimer.Stop() }()
	actor.cached = game
	actor.kickGrace = 5 * time.Minute
	actor.disconnectedSince["moved"] = time.Now().Add(-10 * time.Minute)
	// What the instance they actually reconnected to persisted for them.
	for _, p := range game.PlayersForActor() {
		if p.ID == "moved" {
			p.LastActionAt = time.Now().Add(-time.Minute).UnixMilli()
		}
	}
	var removed []string
	actor.SetOnPlayerRemovedForActor(func(playerID, _ string, _ int64, _ string) {
		removed = append(removed, playerID)
	})

	if err := actor.handleKickTimeout(context.Background(), kickTimeoutCmd{
		PlayerID: "moved",
		Reply:    make(chan error, 1),
	}); err != nil {
		t.Fatalf("handleKickTimeout: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed %v, want nobody", removed)
	}
	if findSeated(game, "moved") == nil {
		t.Fatal("expected the player to keep their seat")
	}
	if _, marked := actor.disconnectedSince["moved"]; marked {
		t.Fatal("expected the stale disconnect mark to be cleared")
	}
}

// The same timer must still free a seat whose player really is gone: no
// activity recorded anywhere inside kickGrace.
func TestKickTimeoutStillRemovesATrulySilentPlayer(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "gone", Stack: 1000, Ready: true},
		{ID: "other", Stack: 1000, Ready: true},
	}, 10, 20)
	actor := New("table-1", nil, true, func(string, hand.Snapshot) {})
	defer func() { actor.afkSweepTimer.Stop() }()
	actor.cached = game
	actor.kickGrace = 5 * time.Minute
	actor.disconnectedSince["gone"] = time.Now().Add(-10 * time.Minute)
	for _, p := range game.PlayersForActor() {
		if p.ID == "gone" {
			p.LastActionAt = time.Now().Add(-10 * time.Minute).UnixMilli()
		}
	}
	var removed []string
	actor.SetOnPlayerRemovedForActor(func(playerID, reason string, _ int64, _ string) {
		if reason == "disconnected" {
			removed = append(removed, playerID)
		}
	})

	if err := actor.handleKickTimeout(context.Background(), kickTimeoutCmd{
		PlayerID: "gone",
		Reply:    make(chan error, 1),
	}); err != nil {
		t.Fatalf("handleKickTimeout: %v", err)
	}
	if len(removed) != 1 || removed[0] != "gone" {
		t.Fatalf("removed = %v, want [gone]", removed)
	}
	if findSeated(game, "gone") != nil {
		t.Fatal("expected the seat to be freed")
	}
}

func findSeated(table *hand.Table, id string) *hand.Player {
	for _, p := range table.PlayersForActor() {
		if p.ID == id {
			return p
		}
	}
	return nil
}
