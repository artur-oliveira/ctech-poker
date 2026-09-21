package highlights

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// Modelled on hand 01M328WD8SRSV25CCPV7BM7KS9 (table 01M327ZR25JS10AMJ7NWMK5YSE,
// 2026-09-21): the felt showed a 206.750 pot, but 125.750 of it was an uncalled
// all-in handed straight back. Only the 81.000 that two players actually
// contested counts — and it counts gross, because rake never appears on the
// pot display the players were reading.
func TestContestedPotExcludesRefundsAndIgnoresRake(t *testing.T) {
	outcome := hand.HandOutcome{PotResults: []hand.PotResult{
		{Amount: 81_000, PayoutAmount: 80_000, Winners: []string{"p1"}},
		{Amount: 125_750, PayoutAmount: 125_750, Refund: true},
	}}
	if got := ContestedPot(outcome); got != 81_000 {
		t.Fatalf("contested pot = %d, want 81000", got)
	}
}

func TestContestedPotSumsEveryContestedLayer(t *testing.T) {
	outcome := hand.HandOutcome{PotResults: []hand.PotResult{
		{Amount: 60_000, PayoutAmount: 59_400, Winners: []string{"p1"}},
		{Amount: 40_000, PayoutAmount: 39_600, Winners: []string{"p2"}},
	}}
	if got := ContestedPot(outcome); got != 100_000 {
		t.Fatalf("contested pot = %d, want 100000", got)
	}
}

func TestContestedPotIsZeroForAPureRefund(t *testing.T) {
	outcome := hand.HandOutcome{PotResults: []hand.PotResult{{Amount: 500, PayoutAmount: 500, Refund: true}}}
	if got := ContestedPot(outcome); got != 0 {
		t.Fatalf("contested pot = %d, want 0", got)
	}
}
