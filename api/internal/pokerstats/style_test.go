package pokerstats

import "testing"

func TestStyleForRequiresMinimumSample(t *testing.T) {
	if got := StyleFor(Stats{Hands: MinHandsPublic - 1, VPIPRate: .1}, MinHandsPublic); got != nil {
		t.Fatalf("StyleFor below public floor = %+v, want nil", got)
	}
}

func TestStyleForMatchesEveryStyle(t *testing.T) {
	tests := []struct {
		name  string
		stats Stats
		key   string
	}{
		{name: "selective", stats: Stats{Hands: 200, VPIPRate: .2, PFRRate: .05}, key: "selective"},
		{name: "explorer", stats: Stats{Hands: 200, VPIPRate: .4, PFRRate: .1}, key: "explorer"},
		{name: "initiative", stats: Stats{Hands: 200, VPIPRate: .3, PFRRate: .21}, key: "initiative"},
		{name: "counter", stats: Stats{Hands: 200, VPIPRate: .3, PFRRate: .1, ThreeBetChances: 10, ThreeBetRate: .1}, key: "counter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StyleFor(tt.stats, MinHandsPublic)
			for _, badge := range got {
				if badge.Key == tt.key {
					return
				}
			}
			t.Fatalf("StyleFor(%+v) = %+v, missing %q", tt.stats, got, tt.key)
		})
	}
}

func TestStyleForCapsAtThree(t *testing.T) {
	got := StyleFor(Stats{Hands: 200, VPIPRate: .4, PFRRate: .3, ThreeBetChances: 20, ThreeBetRate: .2}, MinHandsPublic)
	if len(got) != 3 {
		t.Fatalf("len(StyleFor) = %d, want 3: %+v", len(got), got)
	}
}

func TestStyleForFallsBackToBalanced(t *testing.T) {
	got := StyleFor(Stats{Hands: 200, VPIPRate: .3, PFRRate: .1}, MinHandsPublic)
	if len(got) != 1 || got[0].Key != "balanced" {
		t.Fatalf("StyleFor balanced = %+v", got)
	}
}
