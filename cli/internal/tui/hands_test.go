package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

func sampleHands(now time.Time) []rest.Hand {
	return []rest.Hand{
		{PK: "you", HandID: "hand-00000000001", TableID: "table-000000001", Outcome: "won", NetChange: 250,
			EndedAt: now.UnixMilli(), SmallBlind: 10, BigBlind: 20, HoleCards: []string{"As", "Kd"}, Board: []string{"Qh", "Jc", "Ts", "2d", "3c"}},
		{PK: "you", HandID: "hand-00000000002", TableID: "table-000000001", Outcome: "lost", NetChange: -100,
			EndedAt: now.Add(-time.Hour).UnixMilli(), HoleCards: []string{"7s", "7d"}, Board: []string{"Ah", "Kc", "2s"}},
	}
}

func TestHandsListRendersSummarySelectionAndWidth(t *testing.T) {
	now := time.Now()
	m := &HandsModel{
		cardMode: game.CardASCII, width: 42, height: 14, cursors: []string{""},
		page: rest.Page[rest.Hand]{Data: sampleHands(now), HasNext: true, NextCursor: "next"},
	}
	view := m.View()
	for _, want := range []string{"Histórico de mãos", "2 mãos · +150 fichas · 1V 0E 1D", "Vitória", "As Kd", "N próx."} {
		if !strings.Contains(view, want) {
			t.Errorf("list missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if visibleWidth(line) > terminalLineWidth(42) {
			t.Fatalf("line overflow (%d): %q", visibleWidth(line), line)
		}
	}
	if got := len(strings.Split(view, "\n")); got != 14 {
		t.Fatalf("view height=%d want 14", got)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 1 {
		t.Fatalf("selected=%d want 1", m.selected)
	}
}

func TestHandsCursorNavigationAndBacktracking(t *testing.T) {
	m := &HandsModel{width: 80, height: 24, cursors: []string{""}, page: rest.Page[rest.Hand]{HasNext: true, NextCursor: "c2"}}
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil || !m.loading {
		t.Fatal("next page did not start loading")
	}
	seq := m.requestSeq
	_ = m.Update(handsPageMsg{seq: seq, cursor: "c2", pageIndex: 1, page: rest.Page[rest.Hand]{HasNext: false}})
	if m.pageIndex != 1 || len(m.cursors) != 2 || m.cursors[1] != "c2" {
		t.Fatalf("cursor state=%+v index=%d", m.cursors, m.pageIndex)
	}
	cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil || !m.loading {
		t.Fatal("previous page did not start loading")
	}
}

func TestHandDetailContainsCardsTimelineAndProof(t *testing.T) {
	hand := sampleHands(time.Now())[0]
	hand.Opponents = []rest.OpponentSummary{{PlayerID: "opp", Name: "Caio", HoleCards: []string{"9h", "9c"}}}
	hand.ServerSeed = strings.Repeat("a", 64)
	hand.CommitHash = strings.Repeat("b", 64)
	history := rest.HandHistory{Actions: []rest.HandHistoryAction{
		{Seq: 1, PlayerID: "you", Action: "raise", Amount: 60, Timestamp: hand.EndedAt - 1000,
			Frame: &rest.ReplayFrame{Stage: "preflop", Pot: 90}},
		{Seq: 2, PlayerID: "opp", Action: "call", Amount: 60, Timestamp: hand.EndedAt,
			Frame: &rest.ReplayFrame{Stage: "flop", Board: []string{"Qh", "Jc", "Ts"}, Pot: 120}},
	}}
	out := renderHandDetail(hand, history, nil, 58, game.CardASCII)
	for _, want := range []string{"Vitória · +250 fichas", "Sua mão: As Kd", "Adversários", "Caio · 9h 9c", "Pré-flop", "Você", "aumentou 60", "Flop · Qh Jc Ts", "Prova completa disponível", strings.Repeat("a", 16)} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if visibleWidth(line) > 58 {
			t.Fatalf("detail overflow (%d): %q", visibleWidth(line), line)
		}
	}
}

func TestHandsPartialHistoryFailureStillShowsHand(t *testing.T) {
	hand := sampleHands(time.Now())[0]
	out := renderHandDetail(hand, rest.HandHistory{}, errors.New("timeline offline"), 60, game.CardASCII)
	if !strings.Contains(out, "Sua mão: As Kd") || !strings.Contains(out, "Ações indisponíveis: timeline offline") {
		t.Fatalf("partial detail not preserved:\n%s", out)
	}
}

func TestHandsVeryShortScreenNeverExceedsHeight(t *testing.T) {
	m := &HandsModel{width: 20, height: 2, loading: true}
	if got := len(strings.Split(m.View(), "\n")); got > 2 {
		t.Fatalf("short view used %d rows", got)
	}
}

func TestHandsEscapeEmitsExit(t *testing.T) {
	m := &HandsModel{}
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc did not emit an exit command")
	}
	if _, ok := cmd().(handsExitMsg); !ok {
		t.Fatalf("exit message=%T", cmd())
	}
}
