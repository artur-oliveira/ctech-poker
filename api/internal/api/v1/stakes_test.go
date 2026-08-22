package v1

import "testing"

func TestRealStakeLookupMatchesTierTable(t *testing.T) {
	cases := []struct {
		small, big, wantFee int64
		wantTier            string
	}{
		{5, 10, 100, TierMicro}, {50, 100, 100, TierMicro},
		{100, 200, 200, TierLow}, {200, 500, 200, TierLow},
		{500, 1000, 400, TierMid}, {1000, 2000, 400, TierMid},
		{2500, 5000, 800, TierHigh}, {5000, 10000, 800, TierHigh},
	}
	for _, c := range cases {
		fee, tier, ok := realStakeLookup(c.small, c.big)
		if !ok || fee != c.wantFee || tier != c.wantTier {
			t.Fatalf("realStakeLookup(%d,%d) = %d,%q,%v want %d,%q,true", c.small, c.big, fee, tier, ok, c.wantFee, c.wantTier)
		}
	}
}

func TestRealStakeLookupRejectsUnknownStake(t *testing.T) {
	if _, _, ok := realStakeLookup(7, 14); ok {
		t.Fatal("expected no match for an off-catalog stake pair")
	}
}

func TestSandboxStakesCarryNoFeeOrTier(t *testing.T) {
	for _, s := range sandboxPublicStakes {
		if s.FeeCents != 0 {
			t.Fatalf("sandbox stake %d/%d leaked a real-money fee: %d", s.SmallBlind, s.BigBlind, s.FeeCents)
		}
		if s.Tier != "" {
			t.Fatalf("sandbox stake %d/%d leaked a tier: %q", s.SmallBlind, s.BigBlind, s.Tier)
		}
	}
}
