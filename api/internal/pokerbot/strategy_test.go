package pokerbot

import (
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type fixedRandom struct {
	floats []float64
	ints   []int
	fi, ii int
}

func (r *fixedRandom) Float64() float64 { v := r.floats[r.fi%len(r.floats)]; r.fi++; return v }
func (r *fixedRandom) IntN(n int) int   { v := r.ints[r.ii%len(r.ints)] % n; r.ii++; return v }

func TestEveryProfileReturnsOnlyLegalActions(t *testing.T) {
	legal := &hand.LegalActions{Actions: []string{"fold", "call", "raise"}, CallAmount: 20,
		MinRaiseTo: 40, MaxRaiseTo: 200, OneThirdPotRaiseTo: 60, HalfPotRaiseTo: 80, TwoThirdsPotRaiseTo: 100, PotRaiseTo: 140}
	for _, profile := range Profiles {
		r := &fixedRandom{floats: []float64{.01, .3, .9}, ints: []int{0, 1, 2}}
		for strength := 0.0; strength <= 1; strength += .05 {
			got, err := Decide(profile, Context{Legal: legal, Strength: strength, Pot: 100, Stack: 200}, r)
			if err != nil {
				t.Fatalf("%s: %v", profile.ID, err)
			}
			if got.Action != "fold" && got.Action != "call" && got.Action != "raise" {
				t.Fatalf("%s: illegal %q", profile.ID, got.Action)
			}
			if got.Action == "raise" && (got.Amount < legal.MinRaiseTo || got.Amount > legal.MaxRaiseTo) {
				t.Fatalf("%s: illegal raise %d", profile.ID, got.Amount)
			}
		}
	}
}

func TestThinkDelayHonorsDeadlineWithoutEncodingStrength(t *testing.T) {
	p := Profiles[1]
	r := &fixedRandom{floats: []float64{.9}, ints: []int{0}}
	got := ThinkDelay(p, ThinkHesitation, 3*time.Second, r)
	if got != 1500*time.Millisecond {
		t.Fatalf("got %v, want deadline margin", got)
	}
}

func TestHitAndRunAlwaysLeavesOnlyAfterWonAllIn(t *testing.T) {
	p := Profiles[0]
	r := &fixedRandom{floats: []float64{.99}, ints: []int{0}}
	if !ShouldCashOut(p, ExitContext{WonAllIn: true, HitAndRun: true}, r) {
		t.Fatal("hit-and-run winner stayed")
	}
	if ShouldCashOut(p, ExitContext{HitAndRun: true}, r) {
		t.Fatal("hit-and-run left without winning an all-in")
	}
}
