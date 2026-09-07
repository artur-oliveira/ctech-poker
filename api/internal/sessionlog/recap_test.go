package sessionlog

import "testing"

// The table GSI is keyed by table alone, so a second sitting at the same table
// must not absorb the first one's hands — the session window is the only thing
// separating them.
func TestAggregateRecapKeepsOnlyThisSittingsHands(t *testing.T) {
	session := SessionItem{SK: "s-2", TableID: "tbl-1", JoinedAt: 2_000, EndedAt: 5_000, NetPnL: 300, BuyinAmount: 1_000}
	hands := []HandItem{
		{HandID: "before", NetChange: 9_000, EndedAt: 1_500},
		{HandID: "win-small", NetChange: 100, EndedAt: 2_500},
		{HandID: "loss", NetChange: -400, EndedAt: 3_000},
		{HandID: "win-big", NetChange: 600, EndedAt: 4_000},
		{HandID: "after", NetChange: -9_000, EndedAt: 6_000},
	}

	recap := aggregateRecap(session, hands, false)

	if recap.HandsPlayed != 3 || recap.HandsWon != 2 {
		t.Fatalf("played=%d won=%d", recap.HandsPlayed, recap.HandsWon)
	}
	if recap.DurationMs != 3_000 {
		t.Fatalf("duration=%d", recap.DurationMs)
	}
	if recap.BiggestWin == nil || recap.BiggestWin.HandID != "win-big" {
		t.Fatalf("biggest win=%+v", recap.BiggestWin)
	}
	if recap.BiggestLoss == nil || recap.BiggestLoss.HandID != "loss" {
		t.Fatalf("biggest loss=%+v", recap.BiggestLoss)
	}
}

// An open session has EndedAt 0: its duration runs to now and it has no upper
// bound on which hands count.
func TestAggregateRecapOpenSessionHasNoUpperBound(t *testing.T) {
	session := SessionItem{SK: "s-1", TableID: "tbl-1", JoinedAt: 1_000}
	recap := aggregateRecap(session, []HandItem{{HandID: "h", NetChange: 10, EndedAt: 1 << 40}}, false)
	if recap.HandsPlayed != 1 {
		t.Fatalf("played=%d", recap.HandsPlayed)
	}
	if recap.DurationMs <= 0 {
		t.Fatalf("duration=%d", recap.DurationMs)
	}
	if recap.BiggestLoss != nil {
		t.Fatalf("unexpected loss %+v", recap.BiggestLoss)
	}
}
