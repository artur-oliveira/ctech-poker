package matchup

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

func TestPairKeyIsOrderIndependent(t *testing.T) {
	k1, low1, high1 := pairKey("sandbox", "b", "a")
	k2, low2, high2 := pairKey("sandbox", "a", "b")
	if k1 != k2 || low1 != low2 || high1 != high2 {
		t.Fatalf("pairKey not order independent: %q/%q/%q vs %q/%q/%q", k1, low1, high1, k2, low2, high2)
	}
	if low1 != "a" || high1 != "b" {
		t.Fatalf("expected lexicographic a/b, got %s/%s", low1, high1)
	}
}

func TestDeltasForMultiWayHandCountsHandsWinsTiesButNoNetChange(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:       []string{"a"},
		Participants:  []string{"a", "b", "c"},
		Payouts:       map[string]int64{"a": 300},
		Contributions: map[string]int64{"a": 100, "b": 100, "c": 100},
	}
	deltas := deltasFor("sandbox", outcome)
	if len(deltas) != 3 {
		t.Fatalf("expected 3 pairs, got %d", len(deltas))
	}
	for _, d := range deltas {
		if d.handsTogether != 1 {
			t.Fatalf("pair %s/%s handsTogether = %d, want 1", d.idLow, d.idHigh, d.handsTogether)
		}
		if d.headsUp || d.netLow != 0 || d.netHigh != 0 {
			t.Fatalf("pair %s/%s should carry no net change in a 3-way hand: %+v", d.idLow, d.idHigh, d)
		}
		wantWinLow, wantWinHigh := int64(0), int64(0)
		if d.idLow == "a" {
			wantWinLow = 1
		}
		if d.idHigh == "a" {
			wantWinHigh = 1
		}
		if d.winsLow != wantWinLow || d.winsHigh != wantWinHigh || d.ties != 0 {
			t.Fatalf("pair %s/%s win split = winLow=%d winHigh=%d ties=%d", d.idLow, d.idHigh, d.winsLow, d.winsHigh, d.ties)
		}
	}
}

func TestDeltasForHeadsUpHandTracksNetChangeBothSides(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:       []string{"z"},
		Participants:  []string{"a", "z"},
		Payouts:       map[string]int64{"z": 190},
		Contributions: map[string]int64{"a": 100, "z": 100},
	}
	deltas := deltasFor("sandbox", outcome)
	if len(deltas) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(deltas))
	}
	d := deltas[0]
	if d.idLow != "a" || d.idHigh != "z" {
		t.Fatalf("expected a/z ordering, got %s/%s", d.idLow, d.idHigh)
	}
	// Payouts is net of rake, Contributions is gross: 200 contributed, 190
	// paid out to z (10 rake). netLow=-100 (a lost its whole 100 buy-in),
	// netHigh=90 (z won 90 net of rake) — deliberately not netLow's
	// negation, see the Stats.NetChangeLow/NetChangeHigh doc comment.
	if !d.headsUp || d.netLow != -100 || d.netHigh != 90 {
		t.Fatalf("heads-up net change = %+v, want netLow=-100 netHigh=90", d)
	}
	if d.winsLow != 0 || d.winsHigh != 1 || d.ties != 0 {
		t.Fatalf("win split = %+v, want winHigh=1", d)
	}
}

func TestDeltasForTiedHandCountsBothSidesAsATie(t *testing.T) {
	outcome := hand.HandOutcome{
		Winners:       []string{"a", "z"},
		Participants:  []string{"a", "z"},
		Payouts:       map[string]int64{"a": 95, "z": 95},
		Contributions: map[string]int64{"a": 100, "z": 100},
	}
	deltas := deltasFor("sandbox", outcome)
	if len(deltas) != 1 || deltas[0].ties != 1 || deltas[0].winsLow != 0 || deltas[0].winsHigh != 0 {
		t.Fatalf("split-pot deltas = %+v, want a single tie", deltas)
	}
}

func TestDeltasForSkipsEmptyAndSelfPairs(t *testing.T) {
	outcome := hand.HandOutcome{Participants: []string{"a", "", "a"}}
	if deltas := deltasFor("sandbox", outcome); len(deltas) != 0 {
		t.Fatalf("expected no pairs from an empty/self participant list, got %+v", deltas)
	}
}
