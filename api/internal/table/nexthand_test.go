package table

import (
	"testing"
	"time"
)

func TestArmNextHandTimerEnqueuesNextHandCmdWhenComplete(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Millisecond, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)

	select {
	case cmd := <-a.cmds:
		if _, ok := cmd.(nextHandCmd); !ok {
			t.Fatalf("got command %T, want nextHandCmd", cmd)
		}
		cmd.reply() <- nil
	case <-time.After(200 * time.Millisecond):
		t.Fatal("next-hand timer did not enqueue nextHandCmd")
	}
}

func TestArmNextHandTimerIsIdempotentForTheSameHandID(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Hour, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)
	first := a.nextHandDeadline
	a.armNextHandTimer(true) // same handID — must not restart the countdown
	if !a.nextHandDeadline.Equal(first) {
		t.Fatal("re-arming for the same handID must not restart the 5s countdown")
	}
}

func TestArmNextHandTimerClearsWhenNotComplete(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), nextHandDelay: time.Hour, handID: "h1"}
	t.Cleanup(func() { close(a.done) })
	a.armNextHandTimer(true)
	a.armNextHandTimer(false)
	if a.nextHandArmedFor != "" {
		t.Fatal("expected nextHandArmedFor cleared once the hand is no longer Complete")
	}
}
