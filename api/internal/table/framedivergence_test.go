package table

import (
	"context"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// The 2026-09-15 capture: the same (hand_id, snapshot_version) reached one
// client several times, and the only fields that ever differed between those
// frames were current_streak and equity — both broadcast-time overlays owned
// by whichever instance emitted the frame. Whatever two instances do publish
// for one version has to be identical now.
//
// This fixture exercises the streak half (the hand is Complete, so equity is
// not attached at all); equity's own cross-process determinism is pinned by
// TestEstimateIsIdenticalAcrossProcesses in internal/engine/equity. The
// assertion below covers both fields regardless, so a future fixture that
// does carry equity is checked without further work.
// See docs/specs/2026-09-17-table-snapshot-divergence-and-highlight-winner.md.
func TestTwoInstancesNeverPublishDivergentFramesForOneVersion(t *testing.T) {
	fakeClock(t)
	store := newFakeStreakStore()

	frames := map[string][]hand.Snapshot{}
	collect := func(instance string) func(string, hand.Snapshot) {
		return func(viewerID string, snapshot hand.Snapshot) {
			key := instance + "|" + viewerID
			frames[key] = append(frames[key], snapshot)
		}
	}

	instanceA := newStreakInstance(t, store, collect("a"))
	instanceB := newStreakInstance(t, store, collect("b"))

	// Both instances open their pacing window against the pre-hand badges,
	// which is the state the capture caught them in.
	instanceA.refreshStreaks(context.Background())
	instanceB.refreshStreaks(context.Background())

	// A runs the hand's post-hand hooks and publishes the new badges; B is
	// still inside its 30s window and would otherwise keep serving the
	// previous hand's numbers.
	instanceA.SetStreaksForActor(map[string]int{"p1": 1, "p2": -1})

	// B learns about it through the change signal A just sent.
	if err := instanceB.handleExternalChange(context.Background(), ExternalChangeCmd{}); err != nil {
		t.Fatalf("handleExternalChange: %v", err)
	}

	instanceA.publishSnapshots()
	instanceB.publishSnapshots()

	for _, viewerID := range []string{"p1", "p2"} {
		fromA := frames["a|"+viewerID]
		fromB := frames["b|"+viewerID]
		if len(fromA) == 0 || len(fromB) == 0 {
			t.Fatalf("viewer %s got %d frames from A and %d from B, want both non-empty",
				viewerID, len(fromA), len(fromB))
		}
		assertSameOverlays(t, viewerID, fromA[len(fromA)-1], fromB[len(fromB)-1])
	}
}

// assertSameOverlays compares exactly the two fields the capture showed
// diverging between frames sharing a snapshot version.
func assertSameOverlays(t *testing.T, viewerID string, a, b hand.Snapshot) {
	t.Helper()
	if a.SnapshotVersion != b.SnapshotVersion {
		t.Fatalf("viewer %s: versions %d and %d are not comparable",
			viewerID, a.SnapshotVersion, b.SnapshotVersion)
	}
	if len(a.Seats) != len(b.Seats) {
		t.Fatalf("viewer %s: %d seats vs %d", viewerID, len(a.Seats), len(b.Seats))
	}
	for i := range a.Seats {
		if a.Seats[i].CurrentStreak != b.Seats[i].CurrentStreak {
			t.Fatalf("viewer %s v%d seat %s: streak %d from one instance, %d from the other",
				viewerID, a.SnapshotVersion, a.Seats[i].PlayerID,
				a.Seats[i].CurrentStreak, b.Seats[i].CurrentStreak)
		}
		if !sameEquity(a.Seats[i].Equity, b.Seats[i].Equity) {
			t.Fatalf("viewer %s v%d seat %s: equity %v from one instance, %v from the other",
				viewerID, a.SnapshotVersion, a.Seats[i].PlayerID,
				a.Seats[i].Equity, b.Seats[i].Equity)
		}
	}
}

func sameEquity(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// newStreakInstance builds one process's Actor for the same table, sharing
// the streak store the way two API instances share one Valkey key.
func newStreakInstance(t *testing.T, store StreakStore, broadcast func(string, hand.Snapshot)) *Actor {
	t.Helper()
	actor := New("table-1", nil, true, broadcast)
	t.Cleanup(func() { actor.afkSweepTimer.Stop() })
	actor.cached = completedTable(t)
	actor.SetStreakStoreForActor(store)
	return actor
}
