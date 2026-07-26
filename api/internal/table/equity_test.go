package table

import (
	"sync"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestBroadcastAttachesEquityOnlyToViewer(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	var (
		mu   sync.Mutex
		seen = map[string][]hand.Snapshot{}
	)
	actor := New("table-1", nil, true, func(id string, snapshot hand.Snapshot) {
		mu.Lock()
		seen[id] = append(seen[id], snapshot)
		mu.Unlock()
	})
	actor.cached = table
	actor.broadcastAll()

	// Equity is computed off the Run goroutine and pushed as a follow-up
	// broadcast, so wait for it rather than asserting synchronously.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := hasEquityDelta(seen)
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	for viewerID, snapshots := range seen {
		if len(snapshots) < 2 {
			t.Fatalf("viewer %s did not receive state plus equity delta", viewerID)
		}
		delta := snapshots[len(snapshots)-1]
		if !delta.EquityOnly || delta.EquityPlayerID != viewerID || delta.EquityValue == nil {
			t.Fatalf("viewer %s received invalid equity delta: %+v", viewerID, delta)
		}
		if len(delta.Seats) != 0 {
			t.Fatalf("equity delta must not replay a stale full snapshot, got %d seats", len(delta.Seats))
		}
		if delta.SnapshotVersion != snapshots[0].SnapshotVersion {
			t.Fatalf("equity delta version %d does not match base snapshot %d", delta.SnapshotVersion, snapshots[0].SnapshotVersion)
		}
	}
}

func hasEquityDelta(seen map[string][]hand.Snapshot) bool {
	if len(seen) == 0 {
		return false
	}
	for _, snapshots := range seen {
		if len(snapshots) < 2 || !snapshots[len(snapshots)-1].EquityOnly {
			return false
		}
	}
	return true
}

func TestBroadcastHonorsDisabledEquity(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	actor := New("table-1", nil, true, func(_ string, snapshot hand.Snapshot) {
		for _, seat := range snapshot.Seats {
			if seat.Equity != nil {
				t.Fatal("equity present while disabled")
			}
		}
	})
	actor.cached = table
	actor.SetEquityEnabledForActor(false)
	actor.broadcastAll()
}
