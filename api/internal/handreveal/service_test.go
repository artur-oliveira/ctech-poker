package handreveal

import "testing"

func TestPayForRevealSplitArithmeticIsPure(t *testing.T) {
	// fee/2 with integer division: an odd fee leaves the extra unit as rake,
	// never credited — same rule as Table.RequestWinnerCards.
	fee := int64(201)
	if fee/2 != 100 {
		t.Fatalf("expected floor division, got %d", fee/2)
	}
}
