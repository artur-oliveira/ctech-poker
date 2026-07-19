package sidepots

import (
	"reflect"
	"sort"
	"testing"
)

func sortedEligible(layers []PotLayer) []PotLayer {
	out := make([]PotLayer, len(layers))
	for i, l := range layers {
		e := append([]string(nil), l.Eligible...)
		sort.Strings(e)
		out[i] = PotLayer{Amount: l.Amount, Eligible: e}
	}
	return out
}

func TestTwoWayAllInAtDifferentAmounts(t *testing.T) {
	// A all-in 100, B contributes 300 (not all-in), C all-in 200.
	contributions := []Contribution{
		{PlayerID: "A", Amount: 100},
		{PlayerID: "B", Amount: 300},
		{PlayerID: "C", Amount: 200},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 300, Eligible: []string{"A", "B", "C"}}, // 100 * 3
		{Amount: 200, Eligible: []string{"B", "C"}},       // 100 * 2
		{Amount: 100, Eligible: []string{"B"}},            // 100 * 1
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	var total int64
	for _, l := range got {
		total += l.Amount
	}
	if total != 600 {
		t.Fatalf("layers must sum to total contributed (600), got %d", total)
	}
}

func TestThreeWaySimultaneousAllInsAtDifferentAmounts(t *testing.T) {
	// A all-in 50, B all-in 150, C all-in 300, D contributes 300 (not all-in).
	contributions := []Contribution{
		{PlayerID: "A", Amount: 50},
		{PlayerID: "B", Amount: 150},
		{PlayerID: "C", Amount: 300},
		{PlayerID: "D", Amount: 300},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 200, Eligible: []string{"A", "B", "C", "D"}}, // 50 * 4
		{Amount: 300, Eligible: []string{"B", "C", "D"}},      // 100 * 3
		{Amount: 300, Eligible: []string{"C", "D"}},           // 150 * 2
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	var total int64
	for _, l := range got {
		total += l.Amount
	}
	if total != 800 {
		t.Fatalf("layers must sum to total contributed (800), got %d", total)
	}
}

func TestNoAllInsProducesASingleLayer(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 100},
		{PlayerID: "B", Amount: 100},
	}
	got := ComputeSidePots(contributions)
	if len(got) != 1 || got[0].Amount != 200 {
		t.Fatalf("expected a single 200-chip layer, got %+v", got)
	}
}
