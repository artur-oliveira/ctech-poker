package v1

import "testing"

func TestRealStakeFeeCentsMatchesTierTable(t *testing.T) {
	cases := []struct {
		small, big, wantFee int64
	}{
		{5, 10, 100}, {50, 100, 100},
		{100, 200, 200}, {200, 500, 200},
		{500, 1000, 400}, {1000, 2000, 400},
		{2500, 5000, 800}, {5000, 10000, 800},
	}
	for _, c := range cases {
		fee, ok := realStakeFeeCents(c.small, c.big)
		if !ok || fee != c.wantFee {
			t.Fatalf("realStakeFeeCents(%d,%d) = %d,%v want %d,true", c.small, c.big, fee, ok, c.wantFee)
		}
	}
}

func TestRealStakeFeeCentsRejectsUnknownStake(t *testing.T) {
	if _, ok := realStakeFeeCents(7, 14); ok {
		t.Fatal("expected no fee match for an off-catalog stake pair")
	}
}

func TestSandboxStakesCarryNoFee(t *testing.T) {
	for _, s := range sandboxPublicStakes {
		if s.FeeCents != 0 {
			t.Fatalf("sandbox stake %d/%d leaked a real-money fee: %d", s.SmallBlind, s.BigBlind, s.FeeCents)
		}
	}
}
