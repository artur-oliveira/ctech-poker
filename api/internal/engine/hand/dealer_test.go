package hand

import "testing"

func TestFirstHandDrawsDealerAmongReadyPlayers(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 100 && len(seen) < 2; i++ {
		table := NewTable([]*Player{
			{ID: "p1", Stack: 1000, Ready: true},
			{ID: "p2", Stack: 1000, Ready: true},
			{ID: "p3", Stack: 1000, Ready: true},
		}, 10, 20)
		if err := table.StartHand(); err != nil {
			t.Fatal(err)
		}
		seen[table.dealerSeat] = true
	}
	if len(seen) < 2 {
		t.Fatal("initial CSPRNG dealer draw never varied in 100 hands")
	}
}

func TestInitialDealerIsAlwaysReady(t *testing.T) {
	table := NewTable([]*Player{
		{ID: "not-ready", Stack: 1000},
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "p2", Stack: 1000, Ready: true},
	}, 10, 20)
	if err := table.StartHand(); err != nil {
		t.Fatal(err)
	}
	if table.players[table.dealerSeat].ID == "not-ready" {
		t.Fatal("dealer was drawn from a player who was not ready")
	}
}
