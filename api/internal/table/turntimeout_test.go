package table

import (
	"testing"
	"time"
)

func TestArmTurnTimerEnqueuesTurnTimeoutCmdOnExpiry(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Millisecond}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1")

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

func TestArmTurnTimerIsIdempotentForTheSameCurrentPlayer(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Hour}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1")
	firstDeadline := a.turnDeadline
	a.armTurnTimer("p1") // same current player again — must not reset the deadline
	if !a.turnDeadline.Equal(firstDeadline) {
		t.Fatal("re-arming for the same current player must not restart its deadline")
	}
}

func TestArmTurnTimerClearsWhenNoCurrentPlayer(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), turnTimeout: time.Hour}
	t.Cleanup(func() { close(a.done) })
	a.armTurnTimer("p1")
	a.armTurnTimer("")
	if a.turnDeadlineFor != "" {
		t.Fatal("expected turnDeadlineFor cleared when there is no current player")
	}
}
