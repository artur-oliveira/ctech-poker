package table

import (
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// TestDeadlinesForBroadcastWithholdsAnAlreadyElapsedTurnDeadline reproduces
// the 2026-09-04 incident (docs/specs/2026-09-04-cross-instance-stale-turn-timer.md):
// an actor recreated after eviction, a spot-instance replacement, or any
// other forced reload can inherit a *persisted* deadline that already
// elapsed while nobody was watching it (armTurnTimer deliberately reuses it
// "even if already past" so a resume never grants a fresh full window).
// Broadcasting that stale value made the client render a countdown already
// at 0s (or no ring at all) with no real decision window. deadlinesForBroadcast
// must withhold any deadline that is not strictly in the future.
func TestDeadlinesForBroadcastWithholdsAnAlreadyElapsedTurnDeadline(t *testing.T) {
	a := New("table", nil, true, nil)
	a.handID = "hand-1"
	a.turnDeadlineFor = "p1"

	a.turnDeadline = timeNowFunc().Add(5 * time.Second)
	a.turnBaseDeadline = timeNowFunc()
	if deadline, base, _ := a.deadlinesForBroadcast("p1", hand.PreFlop); deadline == 0 || base == 0 {
		t.Fatalf("future turn deadline was withheld: deadline=%d base=%d", deadline, base)
	}

	a.turnDeadline = timeNowFunc().Add(-5 * time.Second)
	if deadline, base, _ := a.deadlinesForBroadcast("p1", hand.PreFlop); deadline != 0 || base != 0 {
		t.Fatalf("already-elapsed turn deadline was NOT withheld: deadline=%d base=%d", deadline, base)
	}

	// A different current player (turnDeadlineFor stale for THEM) must never
	// see someone else's deadline, elapsed or not.
	if deadline, base, _ := a.deadlinesForBroadcast("p2", hand.PreFlop); deadline != 0 || base != 0 {
		t.Fatalf("deadline leaked to a non-matching current player: deadline=%d base=%d", deadline, base)
	}
}

func TestDeadlinesForBroadcastWithholdsAnAlreadyElapsedNextHandDeadline(t *testing.T) {
	a := New("table", nil, true, nil)
	a.handID = "hand-1"
	a.nextHandArmedFor = "hand-1"

	a.nextHandDeadline = timeNowFunc().Add(12 * time.Second)
	if _, _, nextHand := a.deadlinesForBroadcast("", hand.Complete); nextHand == 0 {
		t.Fatal("future next-hand deadline was withheld")
	}

	a.nextHandDeadline = timeNowFunc().Add(-1 * time.Second)
	if _, _, nextHand := a.deadlinesForBroadcast("", hand.Complete); nextHand != 0 {
		t.Fatalf("already-elapsed next-hand deadline was NOT withheld: next_hand=%d", nextHand)
	}

	// Not actually the hand that armed it (a stale a.nextHandArmedFor from an
	// earlier hand) must never surface either, regardless of timing.
	a.nextHandDeadline = timeNowFunc().Add(12 * time.Second)
	a.handID = "hand-2"
	if _, _, nextHand := a.deadlinesForBroadcast("", hand.Complete); nextHand != 0 {
		t.Fatalf("deadline leaked for a hand that never armed it: next_hand=%d", nextHand)
	}
}
