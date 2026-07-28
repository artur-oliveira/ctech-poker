package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestProductTimingDefaults(t *testing.T) {
	if got := TurnTimeoutFor(0); got != 15*time.Second {
		t.Fatalf("default action timeout = %s, want 15s", got)
	}
	if NextHandDelay != 12*time.Second {
		t.Fatalf("between-hands delay = %s, want 12s", NextHandDelay)
	}
	if got := TurnTimeoutFor(27); got != 27*time.Second {
		t.Fatalf("explicit room timeout = %s, want 27s", got)
	}
}

func TestStaleTurnTimeoutCannotFoldAPlayerOutsideTheirTurn(t *testing.T) {
	game := hand.NewTable([]*hand.Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := game.StartHand(); err != nil {
		t.Fatalf("start hand: %v", err)
	}
	stalePlayer := game.CurrentPlayerIDForActor()
	if err := game.Act(stalePlayer, betting.ActionCall, 0); err != nil {
		t.Fatalf("first call: %v", err)
	}
	raiser := game.CurrentPlayerIDForActor()
	if err := game.Act(raiser, betting.ActionRaise, 40); err != nil {
		t.Fatalf("raise reopening action: %v", err)
	}
	current := game.CurrentPlayerIDForActor()
	if current == stalePlayer {
		t.Fatal("test setup did not leave another player on the clock")
	}
	if !game.CurrentPlayerCanActForActor(stalePlayer) {
		t.Fatal("test setup did not reopen the stale player's action")
	}

	a := New("table", nil, true, nil)
	a.SetCachedForTest(game)
	if err := a.handleTurnTimeout(context.Background(), turnTimeoutCmd{PlayerID: stalePlayer, Reply: make(chan error, 1)}); err != nil {
		t.Fatalf("stale timeout: %v", err)
	}
	if got := game.CurrentPlayerIDForActor(); got != current {
		t.Fatalf("stale timeout changed current player from %s to %s", current, got)
	}
	for _, seat := range game.ViewFor(stalePlayer).Seats {
		if seat.PlayerID == stalePlayer && seat.State == "folded" {
			t.Fatal("stale timeout folded a player outside their turn")
		}
	}
}

func TestArmTurnTimerEnqueuesTurnTimeoutCmdOnExpiry(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Millisecond}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1", hand.PreFlop, 0)

	select {
	case cmd := <-a.cmds:
		c, ok := cmd.(turnTimeoutCmd)
		if !ok {
			t.Fatalf("got command %T, want turnTimeoutCmd", cmd)
		}
		if c.PlayerID != "p1" {
			t.Fatalf("expected PlayerID p1, got %s", c.PlayerID)
		}
		cmd.reply() <- nil
	case <-time.After(200 * time.Millisecond):
		t.Fatal("turn timer did not enqueue turnTimeoutCmd")
	}
}

func TestArmTurnTimerIsIdempotentForTheSameCurrentPlayerAndStage(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Hour}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1", hand.PreFlop, 0)
	firstDeadline := a.turnDeadline
	a.armTurnTimer("p1", hand.PreFlop, 0) // same current player, same street — must not reset the deadline
	if !a.turnDeadline.Equal(firstDeadline) {
		t.Fatal("re-arming for the same current player on the same street must not restart its deadline")
	}
}

func TestArmTurnTimerRearmsForSamePlayerOnANewStage(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Hour}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1", hand.PreFlop, 0)
	firstDeadline := a.turnDeadline
	time.Sleep(time.Millisecond)
	a.armTurnTimer("p1", hand.Flop, 0) // same player, but a new street — must reset the deadline
	if a.turnDeadline.Equal(firstDeadline) {
		t.Fatal("re-arming the same player on a new street must restart its deadline")
	}
}

func TestArmTurnTimerClearsWhenNoCurrentPlayer(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Hour}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1", hand.PreFlop, 0)
	a.armTurnTimer("", hand.PreFlop, 0)
	if a.turnDeadlineFor != "" {
		t.Fatal("expected turnDeadlineFor cleared when there is no current player")
	}
}
