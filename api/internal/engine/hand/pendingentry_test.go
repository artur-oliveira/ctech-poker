package hand

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

func pendingEntryTable(t *testing.T) (*Table, *Player) {
	t.Helper()
	table := NewTable([]*Player{
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	joiner := &Player{ID: "p3", Stack: 1000, Ready: true}
	table.AddMidHandJoiner(joiner)
	return table, joiner
}

func TestPendingEntryStaysUndealtWithoutPosting(t *testing.T) {
	table, joiner := pendingEntryTable(t)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if joiner.State != PendingEntry || joiner.HoleCards != [2]deck.Card{} {
		t.Fatalf("pending entrant was dealt without opting in: %+v", joiner)
	}
}

func TestPendingEntryPostsBigBlindWhenDealtIn(t *testing.T) {
	table, joiner := pendingEntryTable(t)
	table.MarkReadyToPost(joiner.ID)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if joiner.State == PendingEntry {
		t.Fatal("opted-in entrant was not dealt in")
	}
	if joiner.Contributed < 20 {
		t.Fatalf("entrant must post at least one big blind, contributed %d", joiner.Contributed)
	}
}

func TestMarkReadyToPostIsNoOpForNonPendingPlayer(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 1000, Ready: true}}, 10, 20)
	table.MarkReadyToPost("p1")
	if table.readyToPost["p1"] {
		t.Fatal("non-pending player was marked to post")
	}
}

func TestJoiningSamePlayerTwiceIsRejected(t *testing.T) {
	table := NewTable([]*Player{{ID: "p1", Stack: 1000}}, 10, 20)
	if err := table.AddWaitingPlayer(&Player{ID: "p1", Stack: 1000}); err == nil {
		t.Fatal("duplicate waiting player was accepted")
	}
	if err := table.AddMidHandJoiner(&Player{ID: "p1", Stack: 1000}); err == nil {
		t.Fatal("duplicate pending player was accepted")
	}
}
