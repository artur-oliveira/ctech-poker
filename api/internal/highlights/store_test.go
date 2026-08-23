package highlights

import (
	"reflect"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestRevealedHandsOf_OnlyCopiesRevealedHands(t *testing.T) {
	outcome := hand.HandOutcome{
		Participants: []string{"p1", "p2"},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p1": {HoleCards: [2]string{"Ah", "Kd"}, Revealed: true},
			"p2": {HoleCards: [2]string{"2c", "7s"}, Revealed: false},
		},
	}
	names := map[string]string{"p1": "Alice", "p2": "Bob"}

	got := revealedHandsOf(outcome, names)

	if len(got) != 1 {
		t.Fatalf("expected 1 revealed hand, got %d: %+v", len(got), got)
	}
	if got[0].PlayerID != "p1" || got[0].Name != "Alice" {
		t.Fatalf("unexpected revealed hand: %+v", got[0])
	}
	if !reflect.DeepEqual(got[0].HoleCards, []string{"Ah", "Kd"}) {
		t.Fatalf("unexpected hole cards: %+v", got[0].HoleCards)
	}
	for _, entry := range got {
		if entry.PlayerID == "p2" {
			t.Fatalf("folded participant p2's hole cards leaked into highlight: %+v", got)
		}
	}
}

func TestRevealedHandsOf_NoneRevealed(t *testing.T) {
	outcome := hand.HandOutcome{
		Participants: []string{"p1"},
		PlayerHands: map[string]hand.PlayerHandInfo{
			"p1": {HoleCards: [2]string{"Ah", "Kd"}, Revealed: false},
		},
	}
	if got := revealedHandsOf(outcome, nil); len(got) != 0 {
		t.Fatalf("expected no revealed hands, got %+v", got)
	}
}
