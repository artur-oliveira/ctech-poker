// Package game turns wire-level poker state (card codes, TableSnapshot) into
// what the terminal shows: formatted cards, hand-strength labels, position
// tags, and narration lines.
package game

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// CardMode selects how FormatCard/FormatCards render a card code.
type CardMode int

const (
	// CardColor renders "A♠" etc. with ANSI color per suit.
	CardColor CardMode = iota
	// CardASCII renders the raw wire code ("As"), no color — for --cards
	// ascii or when NO_COLOR is set.
	CardASCII
)

var suitGlyphs = map[byte]string{'h': "♥", 'd': "♦", 'c': "♣", 's': "♠"}

// suit colors: hearts red, diamonds blue, clubs green, spades bright
// white/bold — a 4-color deck's usual black substituted for a dark terminal
// (docs/specs/2026-09-05-poker-cli.md §9.4).
var suitStyles = map[byte]lipgloss.Style{
	'h': lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
	'd': lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
	'c': lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
	's': lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true),
}

// FormatCard renders one wire card code ("As", "Th", "7c" — rank + lowercase
// suit letter, matching api/internal/engine/hand/snapshot.go's cardCode).
func FormatCard(code string, mode CardMode) string {
	if mode == CardASCII || len(code) != 2 {
		return code
	}
	rank, suit := code[0], code[1]
	glyph, ok := suitGlyphs[suit]
	if !ok {
		return code
	}
	text := string(rank) + glyph
	if style, ok := suitStyles[suit]; ok {
		return style.Render(text)
	}
	return text
}

// FormatCards renders a space-joined hand or board.
func FormatCards(codes []string, mode CardMode) string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = FormatCard(c, mode)
	}
	return strings.Join(out, " ")
}

const rankOrder = "23456789TJQKA"

func rankValue(code string) int {
	return strings.IndexByte(rankOrder, code[0]) + 2
}

func rankName(v int) string {
	switch v {
	case 14:
		return "A"
	case 13:
		return "K"
	case 12:
		return "Q"
	case 11:
		return "J"
	default:
		return fmt.Sprintf("%d", v)
	}
}

var rankPluralPT = map[int]string{
	14: "ases", 13: "reis", 12: "damas", 11: "valetes",
	10: "dez", 9: "nove", 8: "oito", 7: "sete", 6: "seis", 5: "cinco", 4: "quatro", 3: "três", 2: "dois",
}

// HandStrength labels the best 5-card combination hole+board makes, in the
// same categories ui/src/lib/pokerRules.ts uses (ported, label-only — no
// kicker comparison, since nothing here ranks a showdown; the server does
// that). Empty when hole has fewer than 2 cards.
//
// ponytail: a straightforward rank/suit-count scan over <=7 cards, not a
// full 5-of-7 combinatorial evaluator — good enough for a label, upgrade to
// a real evaluator (internal/engine/handeval-style) if kicker-level
// precision is ever needed here.
func HandStrength(hole, board []string) string {
	if len(hole) < 2 {
		return ""
	}
	all := append(append([]string{}, hole...), board...)

	values := make([]int, len(all))
	for i, c := range all {
		values[i] = rankValue(c)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(values)))

	counts := map[int]int{}
	for _, v := range values {
		counts[v]++
	}
	type group struct{ value, count int }
	var groups []group
	for v, c := range counts {
		groups = append(groups, group{v, c})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].value > groups[j].value
	})

	suitCounts := map[byte]int{}
	for _, c := range all {
		suitCounts[c[1]]++
	}
	isFlush := false
	for _, n := range suitCounts {
		if n >= 5 {
			isFlush = true
			break
		}
	}

	unique := uniqueSortedDesc(values)
	straightHigh := 0
	for i := 0; i+4 < len(unique); i++ {
		if unique[i]-unique[i+4] == 4 {
			straightHigh = unique[i]
			break
		}
	}
	if straightHigh == 0 && contains(unique, 14) && contains(unique, 5) && contains(unique, 4) && contains(unique, 3) && contains(unique, 2) {
		straightHigh = 5 // the wheel: A-2-3-4-5, ace plays low
	}
	isStraight := straightHigh > 0

	switch {
	case isStraight && isFlush:
		return "straight flush"
	case groups[0].count == 4:
		return "quadra"
	case groups[0].count == 3 && len(groups) > 1 && groups[1].count >= 2:
		return "full house"
	case isFlush:
		return "flush"
	case isStraight:
		return "sequência"
	case groups[0].count == 3:
		return "trinca"
	case groups[0].count == 2 && len(groups) > 1 && groups[1].count == 2:
		return "dois pares"
	case groups[0].count == 2:
		if name, ok := rankPluralPT[groups[0].value]; ok {
			return "par de " + name
		}
		return "par"
	default:
		return "carta alta: " + rankName(values[0])
	}
}

func uniqueSortedDesc(values []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

func contains(vs []int, v int) bool {
	return slices.Contains(vs, v)
}
