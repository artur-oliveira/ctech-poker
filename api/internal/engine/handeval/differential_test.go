package handeval

import (
	"math/rand/v2"
	"sort"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
)

// orderCheck accumulates (reference score, table score) pairs and asserts the
// two evaluators induce the same total order on hands. That — not equality of
// the numbers themselves — is the property every caller depends on: showdown
// compares scores, it never inspects them.
type orderCheck struct {
	byRef map[ref.Score]Score
	byNew map[Score]ref.Score
	cat   map[Score]ref.Category
}

func newOrderCheck() *orderCheck {
	return &orderCheck{
		byRef: map[ref.Score]Score{},
		byNew: map[Score]ref.Score{},
		cat:   map[Score]ref.Category{},
	}
}

func (o *orderCheck) add(t *testing.T, cards [7]deck.Card) {
	t.Helper()
	want, got := ref.Best7(cards), Best7(cards)
	if prev, ok := o.byRef[want]; ok && prev != got {
		t.Fatalf("hand %v: reference score %#x maps to both %d and %d", cards, want, prev, got)
	}
	if prev, ok := o.byNew[got]; ok && prev != want {
		t.Fatalf("hand %v: score %d maps back to both %#x and %#x", cards, got, prev, want)
	}
	if got == 0 {
		t.Fatalf("hand %v: well-formed hand scored 0", cards)
	}
	o.byRef[want] = got
	o.byNew[got] = want
	o.cat[got] = want.Category()
}

func (o *orderCheck) verify(t *testing.T) {
	t.Helper()
	refs := make([]ref.Score, 0, len(o.byRef))
	for r := range o.byRef {
		refs = append(refs, r)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i] < refs[j] })

	var prev Score
	for _, r := range refs {
		got := o.byRef[r]
		if got <= prev {
			t.Fatalf("ordering inverted: reference %#x scored %d, not above the previous %d", r, got, prev)
		}
		prev = got
	}
	for score, wantCat := range o.cat {
		if int(score.Category()) != int(wantCat) {
			t.Fatalf("score %d: Category() = %d, reference says %d", score, score.Category(), wantCat)
		}
	}
	t.Logf("checked %d distinct hand strengths", len(o.byRef))
}

// TestMatchesReferenceOrderingOnRandomHands is the default-suite guard: two
// million random 7-card hands must rank identically to the reference
// evaluator. The exhaustive C(52,7) version lives behind the `exhaustive`
// build tag.
func TestMatchesReferenceOrderingOnRandomHands(t *testing.T) {
	const hands = 2_000_000
	rng := rand.New(rand.NewPCG(0x5EED, 0xC0FFEE))
	full := fullDeck()
	check := newOrderCheck()

	for range hands {
		for i := 0; i < 7; i++ {
			j := i + rng.IntN(len(full)-i)
			full[i], full[j] = full[j], full[i]
		}
		check.add(t, [7]deck.Card(full[:7]))
	}
	check.verify(t)
}

// TestMalformedHandsScoreZero pins the defensive contract: a zeroed or
// duplicated card must lose every comparison rather than panic mid-showdown.
func TestMalformedHandsScoreZero(t *testing.T) {
	zeroed := [7]deck.Card{} // never dealt — Rank 0 is below deck.Two
	if got := Best7(zeroed); got != 0 {
		t.Fatalf("all-zero hand scored %d, want 0", got)
	}

	dup := [7]deck.Card{
		c(deck.Ace, deck.Clubs), c(deck.Ace, deck.Clubs), c(deck.Ace, deck.Clubs),
		c(deck.Ace, deck.Clubs), c(deck.Ace, deck.Clubs), c(deck.Two, deck.Hearts), c(deck.Three, deck.Hearts),
	}
	if got := Best7(dup); got != 0 {
		t.Fatalf("hand with five identical cards scored %d, want 0", got)
	}

	badSuit := [7]deck.Card{
		{Rank: deck.Ace, Suit: deck.Suit(9)}, c(deck.King, deck.Clubs), c(deck.Queen, deck.Clubs),
		c(deck.Jack, deck.Clubs), c(deck.Ten, deck.Clubs), c(deck.Two, deck.Hearts), c(deck.Three, deck.Hearts),
	}
	if got := Best7(badSuit); got != 0 {
		t.Fatalf("hand with an out-of-range suit scored %d, want 0", got)
	}
}

// TestScoreRangeCoversEveryHandStrength pins the two published counts this
// scheme has to reproduce: Score spans the 7462 distinct five-card strengths
// (0 reserved for "no hand"), while only 4824 of them are reachable once you
// get to pick the best five out of seven — seven cards can never be as weak
// as, say, 7-5-4-3-2 high.
func TestScoreRangeCoversEveryHandStrength(t *testing.T) {
	if len(categoryTable) != 7463 {
		t.Fatalf("categoryTable has %d entries, want 7463 (0 plus 7462 hand strengths)", len(categoryTable))
	}
	seen := map[Score]bool{}
	for _, v := range noFlushTable {
		seen[Score(v)] = true
	}
	for _, v := range flushTable {
		if v != 0 {
			seen[Score(v)] = true
		}
	}
	if len(seen) != 4824 {
		t.Fatalf("tables produce %d distinct seven-card scores, want 4824", len(seen))
	}
	for s := range seen {
		if s < 1 || s > 7462 {
			t.Fatalf("score %d is outside the valid range [1, 7462]", s)
		}
	}
}

func fullDeck() []deck.Card {
	out := make([]deck.Card, 0, 52)
	for suit := deck.Clubs; suit <= deck.Spades; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			out = append(out, deck.Card{Rank: rank, Suit: suit})
		}
	}
	return out
}

// benchHandCount is a power of two so the benchmark loop indexes with a mask
// instead of a division, keeping the measurement about Best7.
const benchHandCount = 4096

func benchHands() [][7]deck.Card {
	rng := rand.New(rand.NewPCG(1, 2))
	full := fullDeck()
	hands := make([][7]deck.Card, benchHandCount)
	for h := range hands {
		for i := 0; i < 7; i++ {
			j := i + rng.IntN(len(full)-i)
			full[i], full[j] = full[j], full[i]
		}
		hands[h] = [7]deck.Card(full[:7])
	}
	return hands
}

func BenchmarkBest7(b *testing.B) {
	hands := benchHands()
	var sink Score
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink ^= Best7(hands[i&(benchHandCount-1)])
	}
	_ = sink
}

func BenchmarkBest7Reference(b *testing.B) {
	hands := benchHands()
	var sink ref.Score
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink ^= ref.Best7(hands[i&(benchHandCount-1)])
	}
	_ = sink
}

// BenchmarkLoadTables measures the whole startup cost of the evaluator: the
// one-time decode of the embedded blob that init performs.
func BenchmarkLoadTables(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := loadTables(tablesBlob); err != nil {
			b.Fatal(err)
		}
	}
}
