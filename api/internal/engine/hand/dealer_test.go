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

// TestRemovingPlayerBeforeButtonKeepsSameDealer reproduces a bug where
// dealerSeat, a raw index into t.players, went stale after a lower-indexed
// player left: RemovePlayerForActor spliced them out of the slice (shifting
// every later player's index down by one) without adjusting dealerSeat,
// silently handing the button to whoever now occupied that same numeric
// slot — a different player, without any real rotation.
func TestRemovingPlayerBeforeButtonKeepsSameDealer(t *testing.T) {
	table := NewTable([]*Player{
		{ID: "p0", Stack: 1000, Ready: true},
		{ID: "p1", Stack: 1000, Ready: true},
		{ID: "button", Stack: 1000, Ready: true},
		{ID: "p3", Stack: 1000, Ready: true},
	}, 10, 20)
	table.dealerSeat = 2
	table.dealerDrawn = true

	if _, _, err := table.RemovePlayerForActor("p0"); err != nil {
		t.Fatalf("RemovePlayerForActor: %v", err)
	}
	if got := table.players[table.dealerSeat].ID; got != "button" {
		t.Fatalf("dealer silently changed from %q to %q after removing an earlier seat", "button", got)
	}
}

// TestRemovingButtonPassesItToNextSeat covers the companion case: removing
// the dealer themselves needs no index adjustment, since the vacated slot's
// index is unchanged and the next player clockwise slides into it.
func TestRemovingButtonPassesItToNextSeat(t *testing.T) {
	table := NewTable([]*Player{
		{ID: "p0", Stack: 1000, Ready: true},
		{ID: "button", Stack: 1000, Ready: true},
		{ID: "next", Stack: 1000, Ready: true},
	}, 10, 20)
	table.dealerSeat = 1
	table.dealerDrawn = true

	if _, _, err := table.RemovePlayerForActor("button"); err != nil {
		t.Fatalf("RemovePlayerForActor: %v", err)
	}
	if got := table.players[table.dealerSeat].ID; got != "next" {
		t.Fatalf("expected the button to pass to the next seat, got %q", got)
	}
}
