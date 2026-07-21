package table

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestActorNotifiesHandCompletionOnlyOncePerHandID(t *testing.T) {
	table := hand.NewTableFromState(hand.State{
		Stage:       hand.Complete,
		LastOutcome: &hand.HandOutcome{Winners: []string{"p1"}, Participants: []string{"p1", "p2"}},
	})
	calls := 0
	a := &Actor{cached: table, handID: "hand-1", onHandComplete: func(hand.HandOutcome) { calls++ }}
	a.notifyHandComplete()
	a.notifyHandComplete()
	if calls != 1 {
		t.Fatalf("expected one notification, got %d", calls)
	}
	a.handID = "hand-2"
	a.notifyHandComplete()
	if calls != 2 {
		t.Fatalf("expected new hand ID to notify, got %d", calls)
	}
}
