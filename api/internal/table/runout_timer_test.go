package table

import (
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestRunoutTimerIdempotencyIncludesTheRunoutPhase(t *testing.T) {
	a := New("table", nil, true, nil)
	a.handID = "hand"
	a.runoutStreetDelay = time.Hour
	a.SetCachedForTest(hand.NewTableFromState(hand.State{Stage: hand.Flop, RunoutPhase: 1}))
	a.armRunoutTimer(true, hand.Flop)
	first := a.runoutTimer
	t.Cleanup(func() {
		if a.runoutTimer != nil {
			a.runoutTimer.Stop()
		}
	})

	a.SetCachedForTest(hand.NewTableFromState(hand.State{Stage: hand.Flop, RunoutPhase: 2}))
	a.armRunoutTimer(true, hand.Flop)
	if a.runoutTimer == first {
		t.Fatal("phase two at the same stage must arm a fresh runout timer")
	}
}
