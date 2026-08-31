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

func TestSingleContributorProducesOneLayer(t *testing.T) {
	contributions := []Contribution{{PlayerID: "A", Amount: 100}}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{{Amount: 100, Eligible: []string{"A"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestEmptyContributionsProducesNoLayers(t *testing.T) {
	got := ComputeSidePots(nil)
	if len(got) != 0 {
		t.Fatalf("expected no layers for no contributions, got %+v", got)
	}
}

// Zero-amount contributions (e.g. a player who never put chips in this hand)
// must be dropped entirely, not treated as a real all-in level at 0.
func TestZeroAmountContributionsAreIgnored(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 0},
		{PlayerID: "B", Amount: 0},
	}
	got := ComputeSidePots(contributions)
	if len(got) != 0 {
		t.Fatalf("expected no layers when every contribution is zero, got %+v", got)
	}
}

// Everyone shoving for the exact same stack is the common case where a
// three/four-way all-in must NOT fracture into side pots at all.
func TestAllPlayersAllInAtSameAmountProducesSingleLayer(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 500},
		{PlayerID: "B", Amount: 500},
		{PlayerID: "C", Amount: 500},
		{PlayerID: "D", Amount: 500},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 2000, Eligible: []string{"A", "B", "C", "D"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// Two independent tie groups (A/B tied at the short-stack level, C/D tied at
// a mid level) ahead of one deep stack — the layering must resolve both ties
// as single boundaries rather than four separate levels.
func TestFourWayWithTwoSeparateTieGroups(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 50},
		{PlayerID: "B", Amount: 50},
		{PlayerID: "C", Amount: 150},
		{PlayerID: "D", Amount: 150},
		{PlayerID: "E", Amount: 400},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 250, Eligible: []string{"A", "B", "C", "D", "E"}}, // 50 * 5
		{Amount: 300, Eligible: []string{"C", "D", "E"}},           // 100 * 3
		{Amount: 250, Eligible: []string{"E"}},                     // 250 * 1
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

// A realistic full ring (six contributors, mixed cappers and ties) to stress
// the layering at table scale rather than only 3-4 players.
func TestSixWayRingWithMixedAllInsAndTies(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 25},
		{PlayerID: "B", Amount: 80},
		{PlayerID: "C", Amount: 80},
		{PlayerID: "D", Amount: 300},
		{PlayerID: "E", Amount: 300},
		{PlayerID: "F", Amount: 1000},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 150, Eligible: []string{"A", "B", "C", "D", "E", "F"}}, // 25 * 6
		{Amount: 275, Eligible: []string{"B", "C", "D", "E", "F"}},      // 55 * 5
		{Amount: 660, Eligible: []string{"D", "E", "F"}},                // 220 * 3
		{Amount: 700, Eligible: []string{"F"}},                          // 700 * 1
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	var total int64
	for _, l := range got {
		total += l.Amount
	}
	if total != 1785 {
		t.Fatalf("layers must sum to total contributed (1785), got %d", total)
	}
}

// A folded player's partial bet must never carve out its own side pot: its
// chips are absorbed into the layer the live players contest. Here A and C
// are all-in for 100; D1 and D2 folded after putting in 400 each. The result
// is a single 1000-chip pot for A and C — not a 400 main pot plus a 600
// "side pot" for the folded players.
func TestFoldedPartialBetsDoNotCreateASidePot(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 100},
		{PlayerID: "C", Amount: 100},
		{PlayerID: "D1", Amount: 400, Folded: true},
		{PlayerID: "D2", Amount: 400, Folded: true},
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 1000, Eligible: []string{"A", "C"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// A folded player whose contribution sits between two live levels adds no
// boundary of its own — it is spread across the layers that span it.
func TestFoldedMidStackContributionIsAbsorbed(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 100},               // live, all-in
		{PlayerID: "B", Amount: 300},               // live
		{PlayerID: "X", Amount: 200, Folded: true}, // folded between the two
	}
	got := sortedEligible(ComputeSidePots(contributions))
	want := []PotLayer{
		{Amount: 300, Eligible: []string{"A", "B"}}, // 100(A)+100(B)+100(X)
		{Amount: 300, Eligible: []string{"B"}},      // 200(B)+100(X), no boundary at 200
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// A folded player who over-bet everyone still gets the genuinely uncalled
// portion back: X bet 200, only A (all-in 100) was there to call it.
func TestFoldedPlayerUncalledExcessIsReturned(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "A", Amount: 100},
		{PlayerID: "X", Amount: 200, Folded: true},
	}
	got := ComputeSidePots(contributions)
	want := []PotLayer{
		{Amount: 200, Eligible: []string{"A"}},
		{Amount: 100, Eligible: []string{"X"}, Uncalled: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// The classic heads-up over-shove: the excess one player put in that the
// other could never cover is flagged Uncalled so the caller returns it.
func TestHeadsUpOverShoveExcessIsFlaggedUncalled(t *testing.T) {
	contributions := []Contribution{
		{PlayerID: "short", Amount: 10_000},
		{PlayerID: "big", Amount: 50_000},
	}
	got := ComputeSidePots(contributions)
	want := []PotLayer{
		{Amount: 20_000, Eligible: []string{"short", "big"}},
		{Amount: 40_000, Eligible: []string{"big"}, Uncalled: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// The function sorts distinct levels internally, so feeding it contributions
// in an already-shuffled, non-ascending order must still produce the same
// layers as the sorted-input case — guards against a future refactor that
// relies on input order instead of re-deriving it.
func TestResultIsIndependentOfInputOrder(t *testing.T) {
	shuffled := []Contribution{
		{PlayerID: "B", Amount: 300},
		{PlayerID: "C", Amount: 200},
		{PlayerID: "A", Amount: 100},
	}
	got := sortedEligible(ComputeSidePots(shuffled))
	want := []PotLayer{
		{Amount: 300, Eligible: []string{"A", "B", "C"}}, // 100 * 3
		{Amount: 200, Eligible: []string{"B", "C"}},      // 100 * 2
		{Amount: 100, Eligible: []string{"B"}},           // 100 * 1
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
