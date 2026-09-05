package pokerstats

import "testing"

func TestLeaksBelowMinHandsReturnsNil(t *testing.T) {
	s := Stats{Hands: 5, VPIPHands: 4}
	s.calculateRates()
	if got := Leaks(s, MinHandsSelf); got != nil {
		t.Fatalf("expected no leaks below the sample floor, got %+v", got)
	}
}

func TestLeaksFlagsLooseVPIP(t *testing.T) {
	s := Stats{Hands: 100, VPIPHands: 45, PFRHands: 40}
	s.calculateRates()
	leaks := Leaks(s, MinHandsSelf)
	found := false
	for _, l := range leaks {
		if l.Metric == "vpip_rate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a vpip_rate leak, got %+v", leaks)
	}
}

func TestLeaksEmptyForBalancedStats(t *testing.T) {
	s := Stats{Hands: 100, VPIPHands: 22, PFRHands: 18, ThreeBetChances: 10, ThreeBetHands: 2}
	s.calculateRates()
	if got := Leaks(s, MinHandsSelf); len(got) != 0 {
		t.Fatalf("expected no leaks for balanced stats, got %+v", got)
	}
}
