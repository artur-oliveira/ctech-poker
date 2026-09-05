package tui

import (
	"strings"
	"testing"
)

func testSpecs() []commandSpec {
	return []commandSpec{
		{Name: "/profile", Desc: "a"},
		{Name: "/play", Desc: "b"},
		{Name: "/peek", Args: "[all|1|2]", Desc: "c"},
	}
}

func TestMenuHiddenWithoutSlashPrefix(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("profile")
	if m.visible {
		t.Fatal("menu should stay hidden without a leading slash")
	}
}

func TestMenuHiddenOnceArgumentsStart(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/peek ")
	if m.visible {
		t.Fatal("menu should hide once the user moves on to arguments")
	}
}

func TestMenuNarrowsOnPrefix(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/p")
	if !m.visible || len(m.items) != 3 {
		t.Fatalf("visible=%v items=%d", m.visible, len(m.items))
	}
	m.UpdateInput("/pl")
	if !m.visible || len(m.items) != 1 || m.items[0].Name != "/play" {
		t.Fatalf("got %+v", m.items)
	}
}

func TestMenuHidesOnExactSingleMatch(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/play")
	if m.visible {
		t.Fatal("an exact single match should hide the menu (nothing left to suggest)")
	}
}

func TestMenuNavigation(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/p")
	m.moveNext()
	if m.selected != 1 {
		t.Fatalf("selected = %d", m.selected)
	}
	m.moveNext()
	m.moveNext()
	if m.selected != 0 {
		t.Fatalf("wrap-around: selected = %d", m.selected)
	}
	m.movePrev()
	if m.selected != 2 {
		t.Fatalf("wrap-around backwards: selected = %d", m.selected)
	}
}

func TestAcceptZeroArgCommandSubmitsImmediately(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/p")
	m.selected = 0 // /profile
	val, submit := m.accept()
	if val != "/profile" || !submit {
		t.Fatalf("val=%q submit=%v", val, submit)
	}
	if m.visible {
		t.Fatal("accept should hide the menu")
	}
}

func TestAcceptArgCommandFillsAndWaits(t *testing.T) {
	m := newCommandMenu(testSpecs())
	m.UpdateInput("/pe")
	val, submit := m.accept()
	if val != "/peek " || submit {
		t.Fatalf("val=%q submit=%v", val, submit)
	}
}

// TestMenuLineNeverExceedsWidth is the regression guard for a real bug
// reproduced live: a single overlong description (/logout's, at the time
// ~110 visible columns) wrapped onto a second physical terminal row that
// bubbletea's renderer — which counts rows by '\n' alone — had no idea
// about, desyncing its cursor-repositioning math for every frame after.
// The visible symptom looked exactly like a matching/selection bug (entries
// disappearing, the selection marker missing, recoverable only by scrolling
// the terminal's own history) despite the underlying menu state always
// being correct — proven separately by the selection-logic tests above.
// No line View() ever produces may exceed the width it was given, for any
// spec list, any window width, any prefix.
func TestMenuLineNeverExceedsWidth(t *testing.T) {
	for _, width := range []int{20, 40, 60, 76, 80, 100, 120} {
		for _, specs := range [][]commandSpec{homeCommandSpecs, tableCommandSpecs} {
			m := newCommandMenu(specs)
			m.UpdateInput("/")
			view := m.View(len(specs)+1, width)
			for _, line := range strings.Split(view, "\n") {
				if n := len([]rune(stripANSI(line))); n > width {
					t.Fatalf("width=%d: line has %d visible chars, want <= %d: %q", width, n, width, line)
				}
			}
		}
	}
}

// stripANSI removes lipgloss/termenv color escape sequences so a rendered
// line's *visible* length can be measured — their byte length is longer
// than what actually occupies terminal columns.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			b.WriteRune(r)
		}
	}
	return b.String()
}
