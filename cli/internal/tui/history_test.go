package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"gopkg.aoctech.app/poker/cli/internal/game"
)

func TestHistoryWalksBackAndRestoresTheDraft(t *testing.T) {
	var h history
	if _, ok := h.prev("nada"); ok {
		t.Fatal("empty history should not yield a command")
	}
	h.add("/profile")
	h.add("/friends")
	h.add("/friends") // consecutive repeat is dropped
	h.add("")         // empty is dropped

	got, _ := h.prev("/pro")
	if got != "/friends" || h.label() != "2/2" {
		t.Fatalf("first ↑ = %q %q, want /friends 2/2 (newest is last)", got, h.label())
	}
	got, _ = h.prev(got)
	if got != "/profile" || h.label() != "1/2" {
		t.Fatalf("second ↑ = %q %q, want /profile 1/2 (oldest is first)", got, h.label())
	}
	if got, _ = h.prev(got); got != "/profile" {
		t.Fatalf("↑ past the oldest entry = %q, want it to stay put", got)
	}
	got, _ = h.next()
	if got != "/friends" {
		t.Fatalf("↓ = %q, want /friends", got)
	}
	if got, _ = h.next(); got != "/pro" {
		t.Fatalf("↓ past the newest entry = %q, want the draft back", got)
	}
	if h.browsing() || h.label() != "" {
		t.Fatal("walking off the newest entry should end browsing")
	}
	if _, ok := h.next(); ok {
		t.Fatal("↓ while not browsing should do nothing")
	}
}

func TestHomeArrowUpRecallsTheLastCommand(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome
	m, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	s = m.(*Shell)

	s.input.SetValue("/help")
	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)

	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp})
	s = m.(*Shell)
	if s.input.Value() != "/help" {
		t.Fatalf("↑ recalled %q, want /help", s.input.Value())
	}
	if !strings.Contains(ansi.Strip(s.View()), "histórico 1/1") {
		t.Fatalf("history position badge missing:\n%s", s.View())
	}
	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	s = m.(*Shell)
	if s.input.Value() != "" || s.hist.browsing() {
		t.Fatalf("Esc should restore the draft and end browsing: %q", s.input.Value())
	}
}

func TestTableArrowUpRecallsTheLastCommand(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	nm, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(*TableModel)

	m.input.SetValue("/peek")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*TableModel)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = nm.(*TableModel)

	if m.input.Value() != "/peek" {
		t.Fatalf("↑ recalled %q, want /peek", m.input.Value())
	}
	if !strings.Contains(ansi.Strip(m.View()), "histórico 1/1") {
		t.Fatalf("history position badge missing:\n%s", m.View())
	}
}
