package table

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
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

func TestEquityCacheKeysAndHandReset(t *testing.T) {
	actor := New("equity-key-test", nil, true, nil)
	hole := [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.King, Suit: deck.Clubs}}
	board := []deck.Card{{Rank: deck.Two, Suit: deck.Hearts}, {Rank: deck.Seven, Suit: deck.Clubs}, {Rank: deck.Jack, Suit: deck.Diamonds}}
	actor.handID = "first"
	if _, ok := actor.equityFor(hole, nil, 2); !ok {
		t.Fatal("estimate failed")
	}
	if _, ok := actor.equityFor(hole, nil, 2); !ok || len(actor.equityCache) != 1 {
		t.Fatal("repeated spot not cached")
	}
	actor.equityFor(hole, board, 2)
	actor.equityFor(hole, board, 1)
	if len(actor.equityCache) != 3 {
		t.Fatal("board and opponent count must distinguish entries")
	}
	actor.handID = "second"
	actor.equityFor(hole, nil, 2)
	if len(actor.equityCache) != 1 {
		t.Fatal("new hand retained old entries")
	}
	invalid := hole
	invalid[0] = deck.Card{Rank: deck.King, Suit: deck.Suit(4)}
	if _, ok := actor.equityFor(invalid, nil, 2); ok {
		t.Fatal("invalid card reused valid cache entry")
	}
}

func BenchmarkActorEquityCached(b *testing.B) {
	actor := New("equity-bench", nil, true, nil)
	hole := [2]deck.Card{{Rank: deck.Ace, Suit: deck.Clubs}, {Rank: deck.King, Suit: deck.Clubs}}
	actor.equityFor(hole, nil, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := actor.equityFor(hole, nil, 8); !ok {
			b.Fatal("estimate failed")
		}
	}
}
