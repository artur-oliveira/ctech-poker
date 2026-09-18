package main

import (
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/equity/preflop"
)

func TestGenerationDoesNotDependOnWorkerScheduling(t *testing.T) {
	one := generate(100, 1)
	many := generate(100, 4)
	if one != many {
		t.Fatal("worker scheduling changed generated values")
	}
}

func TestGeneratorPocketAcesIncludesSplitShares(t *testing.T) {
	got := estimateClass(preflop.Classes-1, 100_000)
	if got[0] < .84 || got[0] > .86 || got[7] < .33 || got[7] > .36 {
		t.Fatalf("unexpected aces equity: %v", got)
	}
}
