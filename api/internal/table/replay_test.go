package table

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestReplayFrameKeepsPublicStateAndNoCards(t *testing.T) {
	frame := replayFrameFor(hand.Snapshot{
		Stage: "flop", Board: []string{"Ah", "Kd", "2c"},
		CurrentPlayerID: "p2",
		Pots:            []hand.PotView{{Amount: 120}, {Amount: 40}},
		Seats: []hand.SeatView{{
			PlayerID: "p1", Name: "Ana", Stack: 900, State: "active",
			Contributed: 100, DealtIn: true, HoleCards: []string{"As", "Ad"},
		}},
	})
	if frame.Pot != 160 || frame.Stage != "flop" || len(frame.Board) != 3 {
		t.Fatalf("unexpected replay frame: %+v", frame)
	}
	if len(frame.Seats) != 1 || frame.Seats[0].PlayerID != "p1" {
		t.Fatalf("seat projection missing: %+v", frame.Seats)
	}
}
