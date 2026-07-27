// Command gen builds handeval's perfect-hash lookup tables (tables.bin).
//
// It derives every entry from the reference evaluator (handeval/ref), so the
// generated tables are exactly the reference ordering — just precomputed. Run
// it via `go generate ./internal/engine/handeval/...` whenever ref changes.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"math/bits"
	"os"
	"sort"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/hashq"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
)

const (
	numRanks    = 13
	flushSize   = 1 << numRanks // every 13-bit rank mask, indexed directly
	numDistinct = 7462          // distinct 5-card hand strengths in Texas Hold'em
	magic       = "PHE1"
)

func main() {
	out := flag.String("o", "tables.bin", "output path for the generated table blob")
	flag.Parse()

	order := rankAllHands()
	flush := buildFlushTable(order)
	noflush := buildNoFlushTable(order)
	cat := buildCategoryTable(order)

	var buf bytes.Buffer
	buf.WriteString(magic)
	writeU32(&buf, uint32(len(flush)))
	writeU32(&buf, uint32(len(noflush)))
	writeU32(&buf, uint32(len(cat)))
	for _, v := range flush {
		writeU16(&buf, v)
	}
	for _, v := range noflush {
		writeU16(&buf, v)
	}
	buf.Write(cat)

	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "gen: write %s: %v\n", *out, err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
	fmt.Printf("gen: wrote %s (%d bytes): %d flush, %d non-flush, %d categories\n",
		*out, buf.Len(), len(flush), len(noflush), len(cat))
}

// handOrder maps a reference Score to a dense strength value in [1, 7462]
// where higher always wins — the same direction as the reference Score, so no
// caller's comparison logic changes. 0 stays reserved for "no hand".
type handOrder map[ref.Score]uint16

func (o handOrder) value(s ref.Score) uint16 {
	v, ok := o[s]
	if !ok {
		panic(fmt.Sprintf("gen: score %#x produced by a 5-card hand was not in the enumerated ordering", s))
	}
	return v
}

// rankAllHands enumerates all C(52,5) = 2,598,960 five-card hands, collects
// the distinct reference Scores, and assigns each a dense rank. Finding
// exactly 7462 distinct values is the classic sanity check that the reference
// evaluator's ordering is the real Texas Hold'em ordering.
func rankAllHands() handOrder {
	var full [52]deck.Card
	i := 0
	for suit := deck.Clubs; suit <= deck.Spades; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			full[i] = deck.Card{Rank: rank, Suit: suit}
			i++
		}
	}

	seen := map[ref.Score]bool{}
	for a := 0; a < 52; a++ {
		for b := a + 1; b < 52; b++ {
			for c := b + 1; c < 52; c++ {
				for d := c + 1; d < 52; d++ {
					for e := d + 1; e < 52; e++ {
						seen[ref.Evaluate5([5]deck.Card{full[a], full[b], full[c], full[d], full[e]})] = true
					}
				}
			}
		}
	}
	if len(seen) != numDistinct {
		panic(fmt.Sprintf("gen: expected %d distinct hand strengths, got %d", numDistinct, len(seen)))
	}

	scores := make([]ref.Score, 0, len(seen))
	for s := range seen {
		scores = append(scores, s)
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i] < scores[j] })

	order := make(handOrder, len(scores))
	for i, s := range scores {
		order[s] = uint16(i + 1) // 1 = worst (7-high), 7462 = royal flush
	}
	return order
}

// buildFlushTable maps a 13-bit rank mask of a single suit to the strength of
// the best 5-card flush it contains. With 7 cards at most one suit can hold 5
// or more, and such a flush always beats every non-flush hand available from
// the same 7 cards, so the evaluator can short-circuit on it.
func buildFlushTable(order handOrder) []uint16 {
	table := make([]uint16, flushSize)
	for mask := 0; mask < flushSize; mask++ {
		if bits.OnesCount32(uint32(mask)) < 5 {
			continue
		}
		cards := make([]deck.Card, 0, numRanks)
		for r := 0; r < numRanks; r++ {
			if mask&(1<<r) != 0 {
				cards = append(cards, deck.Card{Rank: deck.Two + deck.Rank(r), Suit: deck.Spades})
			}
		}
		table[mask] = order.value(ref.BestN(cards))
	}
	return table
}

// buildNoFlushTable fills the perfect-hash table for every rank multiset of 7
// cards, evaluated with suits arranged so the hand is never a flush.
func buildNoFlushTable(order handOrder) []uint16 {
	table := make([]uint16, hashq.Size)
	filled := make([]bool, hashq.Size)
	var q [numRanks]uint8

	var walk func(pos, remaining int)
	walk = func(pos, remaining int) {
		if pos == numRanks {
			if remaining != 0 {
				return
			}
			idx := hashq.Hash(&q)
			if idx < 0 || idx >= hashq.Size {
				panic(fmt.Sprintf("gen: hash %d out of range for quinary %v", idx, q))
			}
			if filled[idx] {
				panic(fmt.Sprintf("gen: hash collision at %d for quinary %v", idx, q))
			}
			filled[idx] = true
			table[idx] = order.value(ref.Best7(cardsForQuinary(q)))
			return
		}
		limit := 4
		if remaining < limit {
			limit = remaining
		}
		for v := 0; v <= limit; v++ {
			q[pos] = uint8(v)
			walk(pos+1, remaining-v)
		}
		q[pos] = 0
	}
	walk(0, hashq.Hand)

	for i, ok := range filled {
		if !ok {
			panic(fmt.Sprintf("gen: perfect hash left index %d unfilled — not a bijection", i))
		}
	}
	return table
}

// cardsForQuinary turns a rank multiset into 7 concrete cards, handing each
// rank's copies the least-used suits so far. Seven cards spread over four
// suits this way never put five in one suit, so the resulting hand is
// guaranteed not to be a flush — which is exactly what the non-flush table
// must describe.
func cardsForQuinary(q [numRanks]uint8) [7]deck.Card {
	var out [7]deck.Card
	var used [4]int
	n := 0
	for r := 0; r < numRanks; r++ {
		for cp := uint8(0); cp < q[r]; cp++ {
			best := 0
			for s := 1; s < 4; s++ {
				if used[s] < used[best] {
					best = s
				}
			}
			used[best]++
			out[n] = deck.Card{Rank: deck.Two + deck.Rank(r), Suit: deck.Suit(best)}
			n++
		}
	}
	if n != 7 {
		panic(fmt.Sprintf("gen: quinary %v yielded %d cards, want 7", q, n))
	}
	for s, count := range used {
		if count >= 5 {
			panic(fmt.Sprintf("gen: quinary %v put %d cards in suit %d — that is a flush", q, count, s))
		}
	}
	return out
}

// buildCategoryTable lets Score.Category() be a single array index instead of
// hard-coded rank boundaries that could drift from the generated ordering.
func buildCategoryTable(order handOrder) []byte {
	table := make([]byte, numDistinct+1)
	for score, value := range order {
		table[value] = byte(score.Category())
	}
	return table
}

func writeU16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

func writeU32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}
