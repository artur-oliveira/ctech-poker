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
		seen = map[string]hand.Snapshot{}
	)
	actor := New("table-1", nil, true, func(id string, snapshot hand.Snapshot) {
		mu.Lock()
		seen[id] = snapshot
		mu.Unlock()
	})
	actor.cached = table
	actor.broadcastAll()

	// Equity is computed off the Run goroutine and pushed as a follow-up
	// broadcast, so wait for it rather than asserting synchronously.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := hasOwnEquity(seen)
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	for viewerID, snapshot := range seen {
		for _, seat := range snapshot.Seats {
			if seat.PlayerID == viewerID && seat.Equity == nil {
				t.Fatalf("viewer %s has no equity", viewerID)
			}
			if seat.PlayerID != viewerID && seat.Equity != nil {
				t.Fatalf("viewer %s received %s's equity", viewerID, seat.PlayerID)
			}
		}
	}
}

func hasOwnEquity(seen map[string]hand.Snapshot) bool {
	for viewerID, snapshot := range seen {
		for _, seat := range snapshot.Seats {
			if seat.PlayerID == viewerID && seat.Equity == nil {
				return false
			}
		}
	}
	return len(seen) > 0
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
