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
		{Amount: 200, Eligible: []string{"B", "C"}},      // 100 * 2
		{Amount: 100, Eligible: []string{"B"}},           // 100 * 1
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
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 200, Eligible: []string{"A", "B"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// Regression: two players tied at the same all-in level (75) — added from
// task review. The tie must collapse into one layer boundary, not two
// identical ones, and every eligible set must include both tied players.
func TestTwoPlayersTiedAtSameAllInLevel(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 75},
		{PlayerID: "B", Amount: 75},
		{PlayerID: "C", Amount: 200},
		{PlayerID: "D", Amount: 400},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 300, Eligible: []string{"A", "B", "C", "D"}}, // 75 * 4
		{Amount: 250, Eligible: []string{"C", "D"}},           // 125 * 2
		{Amount: 200, Eligible: []string{"D"}},                // 200 * 1
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	var total int64
	for _, l := range got {
		total += l.Amount
	}
	if total != 750 {
		t.Fatalf("layers must sum to total contributed (750), got %d", total)
	}
}
