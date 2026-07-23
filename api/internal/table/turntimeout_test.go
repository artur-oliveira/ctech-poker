package table

import (
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

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
