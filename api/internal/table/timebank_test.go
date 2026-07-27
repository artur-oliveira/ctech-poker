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
