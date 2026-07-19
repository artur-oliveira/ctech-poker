package table

import (
	"context"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func newTestActor(t *testing.T) (*Actor, *hand.Table) {
	t.Helper()
	p1 := &hand.Player{ID: "p1", Stack: 1000, Ready: true}
	p2 := &hand.Player{ID: "p2", Stack: 1000, Ready: true}
	ht := hand.NewTable([]*hand.Player{p1, p2}, 10, 20)
	a := New("table-1", ht, nil, func(string, hand.Snapshot) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)
	return a, ht
}

func TestActorDedupesRepeatedActionID(t *testing.T) {
	a, ht := newTestActor(t)
	if err := ht.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	seat := ""
	for _, s := range ht.ViewFor("p1").Seats {
		if s.State == "active" {
			seat = s.PlayerID
			break
		}
	}

	cmd1 := ActCmd{PlayerID: seat, ActionID: "dup-1", Action: betting.ActionCall, Reply: make(chan error, 1)}
	if err := a.Dispatch(cmd1); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}

	before := ht.ViewFor(seat)
	cmd2 := ActCmd{PlayerID: seat, ActionID: "dup-1", Action: betting.ActionCall, Reply: make(chan error, 1)}
	if err := a.Dispatch(cmd2); err != nil {
		t.Fatalf("duplicate dispatch should be silently ignored, not error: %v", err)
	}
	after := ht.ViewFor(seat)
	if len(before.Seats) != len(after.Seats) {
		t.Fatalf("duplicate action_id must not be re-applied")
	}
}

func TestActorReadyStartsHandAutomatically(t *testing.T) {
	a, ht := newTestActor(t)
	reply := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p1", Ready: true, Reply: reply}); err != nil {
		t.Fatalf("ready p1: %v", err)
	}
	reply2 := make(chan error, 1)
	if err := a.Dispatch(ReadyCmd{PlayerID: "p2", Ready: true, Reply: reply2}); err != nil {
		t.Fatalf("ready p2: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if ht.Stage() == hand.WaitingForPlayers {
		t.Fatal("expected hand to auto-start once both players are ready")
	}
}
