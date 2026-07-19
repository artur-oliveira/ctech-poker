package table

import (
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

func TestStartEscalationPostsTickToActorQueue(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), escalationInterval: time.Millisecond}
	t.Cleanup(func() { close(a.done) })
	a.StartEscalation(roomstore.BlindEscalation{Multiplier: 150, Max: 1000})

	select {
	case cmd := <-a.cmds:
		if _, ok := cmd.(escalateCmd); !ok {
			t.Fatalf("got command %T, want escalateCmd", cmd)
		}
		cmd.reply() <- nil
	case <-time.After(100 * time.Millisecond):
		t.Fatal("escalation timer did not enqueue an actor command")
	}
}

func TestStartEscalationIgnoresInvalidConfig(t *testing.T) {
	a := &Actor{cmds: make(chan Command, 1), done: make(chan struct{}), escalationInterval: time.Millisecond}
	t.Cleanup(func() { close(a.done) })
	a.StartEscalation(roomstore.BlindEscalation{Multiplier: 100, Max: 1000})
	select {
	case cmd := <-a.cmds:
		t.Fatalf("invalid escalation enqueued %T", cmd)
	case <-time.After(5 * time.Millisecond):
	}
}
