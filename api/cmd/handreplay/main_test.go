package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunScriptProducesPayoutsSummingToTotalPot(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("script.example.json"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	payouts, err := runScript(data)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	var total int64
	for _, v := range payouts {
		total += v
	}
	if total != 640 {
		t.Fatalf("expected total payouts of 640, got %d (%+v)", total, payouts)
	}
}

// TestRunScriptWithFoldBeforeRiverReconciles exercises Task 9's flagged (but
// unconfirmed) open item: runShowdown's handling when a player folds before
// the river could, in an unconstructed edge case, orphan side-pot chips whose
// only eligible layer members have all folded. This script folds Dealer on
// the flop while side pots are in play (SB is short and all-in from
// pre-flop), then runs BB unopposed to the river.
//
// Expected final contributions: Dealer=300 (folded, never adds flop money),
// SB=150 (all-in pre-flop), BB=500 (300 pre-flop + 200 flop bet).
// Side-pot layers: [0,150) x3 payers = 450, [150,300) x2 payers (Dealer, BB)
// = 300, [300,500) x1 payer (BB) = 200. Total = 950, matching total
// contributed (300+150+500=950) even though Dealer's 300 never wins
// anything — it's folded into the pot, not orphaned.
func TestRunScriptWithFoldBeforeRiverReconciles(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("script.fold.json"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	payouts, err := runScript(data)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	var total int64
	for _, v := range payouts {
		total += v
	}
	const wantTotal = 950 // total chips contributed by all three players this hand
	if total != wantTotal {
		t.Fatalf("expected total payouts of %d (all contributed chips, including the folded player's), got %d (%+v)", wantTotal, total, payouts)
	}
	if _, ok := payouts["Dealer"]; ok {
		t.Fatalf("Dealer folded and must not be paid anything, got %+v", payouts)
	}
}
