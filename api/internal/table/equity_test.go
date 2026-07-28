package table

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestBroadcastAttachesEquitySynchronouslyOnlyToViewer(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	seen := map[string][]hand.Snapshot{}
	actor := New("table-1", nil, true, func(id string, snapshot hand.Snapshot) {
		seen[id] = append(seen[id], snapshot)
	})
	actor.cached = table
	actor.broadcastAll()

	for viewerID, snapshots := range seen {
		if len(snapshots) != 1 {
			t.Fatalf("viewer %s received %d broadcasts, want one complete snapshot", viewerID, len(snapshots))
		}
		for _, seat := range snapshots[0].Seats {
			if seat.PlayerID == viewerID && seat.Equity == nil {
				t.Fatalf("viewer %s did not receive its equity", viewerID)
			}
			if seat.PlayerID != viewerID && seat.Equity != nil {
				t.Fatalf("viewer %s received another player's equity for %s", viewerID, seat.PlayerID)
			}
		}
	}
}

func TestBroadcastReusesEquityWhenLogicalStateIsUnchanged(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	seen := map[string][]float64{}
	actor := New("table-1", nil, true, func(viewerID string, snapshot hand.Snapshot) {
		for _, seat := range snapshot.Seats {
			if seat.PlayerID == viewerID && seat.Equity != nil {
				seen[viewerID] = append(seen[viewerID], *seat.Equity)
			}
		}
	})
	actor.cached = table
	actor.broadcastAll()
	actor.broadcastAll()

	for viewerID, equities := range seen {
		if len(equities) != 2 {
			t.Fatalf("viewer %s received %d equities, want two", viewerID, len(equities))
		}
		if equities[0] != equities[1] {
			t.Fatalf("viewer %s equity changed without a logical state change: %f -> %f", viewerID, equities[0], equities[1])
		}
	}
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
