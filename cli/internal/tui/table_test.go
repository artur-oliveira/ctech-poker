package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/proto"
)

func tableFixtureSnapshot() *proto.ServerMessage {
	dealt := true
	eq := 0.62
	return &proto.ServerMessage{
		Type: "state",
		Snapshot: &proto.TableSnapshot{
			Stage: "flop", Board: []string{"Ah", "7c", "Kd"},
			CurrentPlayerId:      "you",
			DealerPlayerId:       "caio",
			SmallBlindPlayerId:   "duda",
			BigBlindPlayerId:     "edu",
			SnapshotVersion:      7,
			HandId:               "h-9",
			Pots:                 []*proto.Pot{{Amount: 24}},
			ActionDeadlineUnixMs: 9_000_000_000_000,
			LegalActions: &proto.LegalActions{
				Actions: []string{"fold", "call", "raise"}, CallAmount: 8, MinRaiseTo: 16, MaxRaiseTo: 246, PotRaiseTo: 32,
			},
			Seats: []*proto.Seat{
				{PlayerId: "caio", Name: "Caio", Stack: 297, State: "active", DealtIn: &dealt},
				{PlayerId: "duda", Name: "Duda", Stack: 100, State: "active", DealtIn: &dealt},
				{PlayerId: "edu", Name: "Edu", Stack: 402, State: "active", DealtIn: &dealt},
				{PlayerId: "you", Name: "VOCÊ", Stack: 246, State: "active", DealtIn: &dealt, HoleCards: []string{"As", "Qh"}, Equity: &eq},
			},
		},
	}
}

func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drainCmd(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestTableViewRendersLayoutBHeader(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)

	out := m.View()
	if !strings.Contains(out, "No-Limit Hold'em") {
		t.Errorf("missing game type: %q", out)
	}
	if !strings.Contains(out, "SUA VEZ") {
		t.Errorf("missing turn indicator: %q", out)
	}
	if !strings.Contains(out, "Pote 24") {
		t.Errorf("sandbox pot should have no currency symbol: %q", out)
	}
	if strings.Contains(out, "R$") {
		t.Errorf("sandbox table must not render R$: %q", out)
	}
}

func TestTableViewRealMoneyRendersCurrency(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", RealMoney: true, Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	if !strings.Contains(m.View(), "R$ 24") {
		t.Errorf("real-money pot should render R$: %q", m.View())
	}
}

func TestTableActionSendsActWithPreconditions(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)

	typeLine(t, m, "/raise 40")
	if len(sent) != 1 || sent[0].Type != "act" || sent[0].Action != "raise" || sent[0].Amount != 40 {
		t.Fatalf("sent = %+v", sent)
	}
	if sent[0].ExpectedSnapshotVersion != 7 || sent[0].ExpectedHandId != "h-9" {
		t.Fatalf("missing optimistic preconditions: %+v", sent[0])
	}
}

func TestTableUnavailableErrorResyncsAndResendsSameActionID(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	typeLine(t, m, "/fold")

	firstID := sent[0].ActionId
	sent = nil

	nm, cmd := m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "error", ActionId: firstID, Code: "unavailable", Message: "store busy"}})
	m = nm.(*TableModel)
	for _, msg := range drainCmd(cmd) {
		if msg != nil {
			m.Update(msg)
		}
	}
	if len(sent) != 2 {
		t.Fatalf("want a sync_state then a resend, got %+v", sent)
	}
	if sent[0].Type != "sync_state" {
		t.Errorf("first resend frame = %q, want sync_state", sent[0].Type)
	}
	if sent[1].ActionId != firstID {
		t.Errorf("resend must reuse the original action id: %q vs %q", sent[1].ActionId, firstID)
	}
}

func TestTableRemovedEmitsExitAndLogsSettledStack(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6})
	nm, cmd := m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "removed", Message: "idle", Amount: 1500}})
	m = nm.(*TableModel)
	if cmd == nil {
		t.Fatal("removed should emit a command")
	}
	if _, ok := cmd().(TableExitedMsg); !ok {
		t.Errorf("removed should emit TableExitedMsg, got %T", cmd())
	}
	if !strings.Contains(strings.Join(m.log, "\n"), "1500") {
		t.Errorf("settled stack not logged: %v", m.log)
	}
}

func TestTableClearCommandEmptiesLog(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	typeLine(t, m, "/clear")
	if len(m.log) != 0 {
		t.Fatalf("log = %v, want empty after /clear", m.log)
	}
}

func TestTableCtrlLClearsLog(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	m.appendLog("something")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(*TableModel)
	if len(m.log) != 0 {
		t.Fatalf("log = %v, want empty after Ctrl+L", m.log)
	}
}

func TestTableSlashOpensMenuAndTabCompletes(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	for _, r := range "/tal" {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(*TableModel)
	}
	if !m.menu.visible {
		t.Fatal("menu should be visible while typing a command prefix")
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(*TableModel)
	if m.input.Value() != "/talk " {
		t.Fatalf("input = %q, want tab-completed to /talk with a trailing space", m.input.Value())
	}
}

func TestTableHelpListsCommandsWithDescriptions(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	typeLine(t, m, "/help")
	out := strings.Join(m.log, "\n")
	if !strings.Contains(out, "/raise") || !strings.Contains(out, "Aumenta para o valor") {
		t.Fatalf("help output missing command+description: %q", out)
	}
}

func typeLine(t *testing.T, m *TableModel, line string) {
	t.Helper()
	for _, r := range line {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		*m = *nm.(*TableModel)
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	*m = *nm.(*TableModel)
	for _, msg := range drainCmd(cmd) {
		if msg != nil {
			nm2, _ := m.Update(msg)
			*m = *nm2.(*TableModel)
		}
	}
}

// TestTableViewNeverOverflowsWindow mirrors Shell's identical regression
// guard: on a small terminal, the table's larger command set could push a
// suggestion menu (or a long hand log) past the window's real height.
// Heights below the header's own line count (here ~4-5 lines) are out of
// scope — the header isn't compressible, so a window that small can't fit
// the table at all regardless of menu/log sizing; that's an accepted floor,
// not something layoutHeights can do anything further about.
func TestTableViewNeverOverflowsWindow(t *testing.T) {
	for _, height := range []int{10, 20, 24, 40} {
		m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
		nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
		m = nm.(*TableModel)
		nm, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: height})
		m = nm.(*TableModel)

		for _, seq := range []string{"/", "/c", "/ch", "/che", "/e"} {
			m.input.SetValue("")
			for _, r := range seq {
				nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = nm.(*TableModel)
			}
			got := strings.Split(m.View(), "\n")
			if len(got) > height {
				t.Fatalf("height=%d seq=%q: View() produced %d lines, want <= %d\n%s",
					height, seq, len(got), height, m.View())
			}
		}
	}
}
