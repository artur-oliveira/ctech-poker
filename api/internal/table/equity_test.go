package table

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestBroadcastAttachesEquityOnlyToViewer(t *testing.T) {
	table := hand.NewTable([]*hand.Player{{ID: "p1", Stack: 1000, Ready: true}, {ID: "p2", Stack: 1000, Ready: true}}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	seen := map[string]hand.Snapshot{}
	actor := New("table-1", nil, true, func(id string, snapshot hand.Snapshot) { seen[id] = snapshot })
	actor.cached = table
	actor.broadcastAll()

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
