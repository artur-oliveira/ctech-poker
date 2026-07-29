package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func newTimeBankActor(t *testing.T) (*Actor, *hand.Table, string) {
	t.Helper()
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	current := game.CurrentPlayerIDForActor()
	actor := New("time-bank", nil, true, nil)
	actor.cached = game
	actor.handID = "hand-1"
	return actor, game, current
}

func TestTimeBankConsumesOnlyAfterBaseDeadline(t *testing.T) {
	actor, game, current := newTimeBankActor(t)
	baseNow := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	oldNow := timeNowFunc
	timeNowFunc = func() time.Time { return baseNow }
	t.Cleanup(func() {
		timeNowFunc = oldNow
		if actor.turnTimer != nil {
			actor.turnTimer.Stop()
		}
	})

	actor.armTurnTimer(current, hand.PreFlop, 0)
	timeNowFunc = func() time.Time { return actor.turnBaseDeadline.Add(7 * time.Second) }
	if _, err := actor.applyActAndCommit(context.Background(), ActCmd{
		PlayerID: current, ActionID: "act-1", Action: betting.ActionFold,
	}); err != nil {
		t.Fatal(err)
	}
	if got := game.TimeBankForActor(current); got != hand.DefaultTimeBankMs-7000 {
		t.Fatalf("time bank=%d, want %d", got, hand.DefaultTimeBankMs-7000)
	}
}

func TestTimeBankExhaustsBeforeTimeoutFold(t *testing.T) {
	actor, game, current := newTimeBankActor(t)
	baseNow := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	oldNow := timeNowFunc
	timeNowFunc = func() time.Time { return baseNow }
	t.Cleanup(func() {
		timeNowFunc = oldNow
		if actor.turnTimer != nil {
			actor.turnTimer.Stop()
		}
	})

	actor.armTurnTimer(current, hand.PreFlop, 0)
	timeNowFunc = func() time.Time { return actor.turnDeadline.Add(time.Millisecond) }
	if err := actor.handleTurnTimeout(context.Background(), turnTimeoutCmd{
		PlayerID: current, Reply: make(chan error, 1),
	}); err != nil {
		t.Fatal(err)
	}
	if got := game.TimeBankForActor(current); got != 0 {
		t.Fatalf("time bank=%d, want exhausted", got)
	}
}

func TestTimeBankNeverChargesAnotherPlayer(t *testing.T) {
	actor, game, current := newTimeBankActor(t)
	other := "p1"
	if other == current {
		other = "p2"
	}
	baseNow := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	oldNow := timeNowFunc
	timeNowFunc = func() time.Time { return baseNow }
	t.Cleanup(func() {
		timeNowFunc = oldNow
		if actor.turnTimer != nil {
			actor.turnTimer.Stop()
		}
	})

	actor.armTurnTimer(current, hand.PreFlop, 0)
	timeNowFunc = func() time.Time { return actor.turnBaseDeadline.Add(4 * time.Second) }
	actor.consumeTimeBank(other)
	if got := game.TimeBankForActor(other); got != hand.DefaultTimeBankMs {
		t.Fatalf("other player's bank changed to %d", got)
	}
	if got := game.TimeBankForActor(current); got != hand.DefaultTimeBankMs {
		t.Fatalf("current player's bank changed without their action to %d", got)
	}
}

func TestPersistedDeadlineCannotLeakToNextPlayer(t *testing.T) {
	actor, game, current := newTimeBankActor(t)
	next := "p1"
	if next == current {
		next = "p2"
	}
	baseNow := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	oldNow := timeNowFunc
	timeNowFunc = func() time.Time { return baseNow }
	t.Cleanup(func() {
		timeNowFunc = oldNow
		if actor.turnTimer != nil {
			actor.turnTimer.Stop()
		}
	})

	actor.turnDeadlineFor = current
	actor.turnDeadlineForStage = hand.PreFlop
	actor.pendingPersistedDeadline = baseNow.Add(3 * time.Second).UnixMilli()
	actor.pendingDeadlineFor = current
	actor.pendingDeadlineForStage = hand.PreFlop
	actor.armTurnTimer(current, hand.PreFlop, 0)
	actor.armTurnTimer(next, hand.PreFlop, 0)

	want := baseNow.Add(actor.turnTimeout + time.Duration(game.TimeBankForActor(next))*time.Millisecond)
	if !actor.turnDeadline.Equal(want) {
		t.Fatalf("next player inherited persisted deadline: got %v want %v", actor.turnDeadline, want)
	}
}
