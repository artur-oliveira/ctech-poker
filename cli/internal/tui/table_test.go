package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/proto"
	"gopkg.aoctech.app/poker/cli/internal/rest"
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

func TestTablePeekGateHidesHoleCardsUntilPeeked(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)

	if out := m.View(); !strings.Contains(out, "██ ██") || strings.Contains(out, "62%") {
		t.Fatalf("cards/equity should be hidden pre-peek: %q", out)
	}

	drainCmd(m.runLocal(ActPeekBoth))
	out := m.View()
	if !strings.Contains(out, "As") || !strings.Contains(out, "Qh") || !strings.Contains(out, "62%") {
		t.Fatalf("cards + equity should show after peeking both: %q", out)
	}
	if len(sent) != 1 || sent[0].Type != "peek_cards" {
		t.Fatalf("exactly one peek_cards breadcrumb expected: %+v", sent)
	}

	// Toggling again (hide, then peek one) must not send another breadcrumb this hand.
	drainCmd(m.runLocal(ActPeekBoth))
	drainCmd(m.runLocal(ActPeekCard1))
	if len(sent) != 1 {
		t.Fatalf("breadcrumb is once per hand: %+v", sent)
	}

	// New hand resets the gate.
	snap := tableFixtureSnapshot()
	snap.Snapshot.HandId = "h-10"
	nm, _ = m.Update(SnapshotMsg{M: snap})
	m = nm.(*TableModel)
	if out := m.View(); !strings.Contains(out, "██ ██") {
		t.Fatalf("new hand should hide cards again: %q", out)
	}
}

func TestTableCtrlCAtTableNeedsConfirmation(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		RoomID: "r-1", YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)

	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = nm.(*TableModel)
	if cmd != nil {
		t.Fatalf("first Ctrl+C must not exit")
	}
	if !strings.Contains(m.View(), "Ctrl+C de novo") {
		t.Fatalf("first Ctrl+C should warn: %q", m.View())
	}

	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = nm.(*TableModel)
	msgs := drainCmd(cmd)
	var exited bool
	for _, msg := range msgs {
		if _, ok := msg.(TableExitedMsg); ok {
			exited = true
		}
	}
	if !exited {
		t.Fatalf("second Ctrl+C should emit TableExitedMsg, got %#v", msgs)
	}
	// It's your turn in the fixture → fold, then request_exit.
	if len(sent) < 2 || sent[0].Action != "fold" || sent[len(sent)-1].Type != "request_exit" {
		t.Fatalf("expected fold + request_exit on confirmed exit: %+v", sent)
	}
}

func TestTableSeatTableRendersAlignedRows(t *testing.T) {
	msg := tableFixtureSnapshot()
	msg.Snapshot.Seats[1].State = "folded" // Duda
	msg.Snapshot.Seats[3].Contributed = 20 // you
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: msg})
	m = nm.(*TableModel)
	nm, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(*TableModel)

	out := ansi.Strip(m.View())
	if !strings.Contains(out, "\nJogadores\n") {
		t.Fatalf("expected a Jogadores heading line:\n%s", out)
	}
	var seatLines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "VOCÊ") || strings.Contains(l, "Caio") || strings.Contains(l, "Duda") || strings.Contains(l, "Edu") {
			seatLines = append(seatLines, l)
		}
	}
	if len(seatLines) != 4 {
		t.Fatalf("want 4 seat rows, got %d:\n%s", len(seatLines), strings.Join(seatLines, "\n"))
	}
	// Name column starts at the same visual column on every row (the actor
	// marker "▶" is one cell but three bytes, so measure display width).
	nameOffset := func(l string) int {
		for _, n := range []string{"Caio", "Duda", "Edu", "VOCÊ"} {
			if i := strings.Index(l, n); i >= 0 {
				return ansi.StringWidth(l[:i])
			}
		}
		return -1
	}
	want := nameOffset(seatLines[0])
	for _, l := range seatLines {
		if got := nameOffset(l); got != want {
			t.Errorf("name column misaligned: offset %d vs %d in %q", got, want, l)
		}
	}
	youLine := seatLines[3]
	if !strings.Contains(youLine, "▶") || !strings.Contains(youLine, "aposta 20") {
		t.Errorf("your row should be the actor and show the bet: %q", youLine)
	}
	for _, l := range seatLines {
		if strings.Contains(l, "Duda") && !strings.Contains(l, "desistiu") {
			t.Errorf("folded row missing note: %q", l)
		}
	}
}

func TestTableRealityCheckFiresPeriodicallyOffTurn(t *testing.T) {
	msg := tableFixtureSnapshot()
	msg.Snapshot.CurrentPlayerId = "caio" // not your turn

	var sessionCalls int
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		RealityCheckEvery: 10 * time.Minute,
		CurrentSession: func(context.Context) (string, error) {
			sessionCalls++
			return "sessão: buy-in 100", nil
		},
	})
	nm, _ := m.Update(SnapshotMsg{M: msg})
	m = nm.(*TableModel)
	t0 := m.joinedAt
	if t0.IsZero() {
		t.Fatal("joinedAt should be set on the first snapshot")
	}

	// Before the first boundary: nothing. maybeRealityCheck is called
	// directly (bypassing TableTickMsg/Update) so the test doesn't also pay
	// for tableTick's real 1s timer.
	m.now = t0.Add(5 * time.Minute)
	drainCmd(m.maybeRealityCheck())
	if strings.Contains(strings.Join(m.log, "\n"), "Pausa consciente") {
		t.Fatal("fired before the interval elapsed")
	}

	// Past the 10min boundary: fires once, fetches the session summary.
	m.now = t0.Add(11 * time.Minute)
	for _, res := range drainCmd(m.maybeRealityCheck()) {
		nm, _ = m.Update(res)
		m = nm.(*TableModel)
	}
	out := strings.Join(m.log, "\n")
	if !strings.Contains(out, "Pausa consciente") || !strings.Contains(out, "Mãos concluídas 0") {
		t.Fatalf("reality check did not fire: %q", out)
	}
	if !strings.Contains(out, "sessão: buy-in 100") || sessionCalls != 1 {
		t.Fatalf("session summary not fetched exactly once: calls=%d out=%q", sessionCalls, out)
	}

	// Same boundary again: no repeat.
	m.now = t0.Add(11*time.Minute + 30*time.Second)
	drainCmd(m.maybeRealityCheck())
	if sessionCalls != 1 {
		t.Fatalf("must not re-fire within the same boundary: calls=%d", sessionCalls)
	}

	// It's your turn now: the next boundary is held, not dropped.
	yourTurn := tableFixtureSnapshot() // CurrentPlayerId == "you"
	nm, _ = m.Update(SnapshotMsg{M: yourTurn})
	m = nm.(*TableModel)
	m.now = t0.Add(21 * time.Minute)
	drainCmd(m.maybeRealityCheck())
	if sessionCalls != 1 {
		t.Fatalf("must not fire during the viewer's turn: calls=%d", sessionCalls)
	}
	nm, _ = m.Update(SnapshotMsg{M: msg}) // back off-turn
	m = nm.(*TableModel)
	m.now = t0.Add(22 * time.Minute)
	drainCmd(m.maybeRealityCheck())
	if sessionCalls != 2 {
		t.Fatalf("held boundary should fire once the turn passes: calls=%d", sessionCalls)
	}
}

func submitTableLine(m *TableModel, line string) (*TableModel, tea.Cmd) {
	m.input.SetValue(line)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return nm.(*TableModel), cmd
}

func TestTablePlayerNotesFetchViewSaveAndClear(t *testing.T) {
	var savedTag, savedText string
	var saveCalls int
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Notes: func(_ context.Context, ids []string) ([]rest.PlayerNote, error) {
			if len(ids) != 3 { // caio, duda, edu — every opponent, not you
				t.Errorf("expected 3 opponent ids, got %v", ids)
			}
			return []rest.PlayerNote{{OpponentID: "caio", Tag: "red", Text: "joga muito"}}, nil
		},
		SaveNote: func(_ context.Context, id, tag, text string) (rest.PlayerNote, error) {
			saveCalls++
			savedTag, savedText = tag, text
			if tag == "" && text == "" {
				return rest.PlayerNote{OpponentID: id}, nil // deleted
			}
			return rest.PlayerNote{OpponentID: id, Tag: tag, Text: text}, nil
		},
	})
	nm, cmd := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	for _, res := range drainCmd(cmd) {
		nm, _ = m.Update(res)
		m = nm.(*TableModel)
	}
	if m.notes["caio"].Tag != "red" {
		t.Fatalf("note not cached after fetch: %+v", m.notes)
	}

	// A second snapshot with the same seats must not re-fetch.
	nm, cmd = m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	if cmd != nil {
		t.Fatal("notes for already-asked opponents must not be re-fetched")
	}

	// /note <player> with no more args shows the cached note.
	m, cmd = submitTableLine(m, "/note Caio")
	if cmd != nil {
		t.Fatal("viewing a note is local, no command expected")
	}
	if out := strings.Join(m.log, "\n"); !strings.Contains(out, "nota sobre Caio [red]: joga muito") {
		t.Fatalf("note view missing: %q", out)
	}

	// Free-form text saves, preserving the existing tag.
	m, cmd = submitTableLine(m, "/note Caio fica de olho no all-in")
	for _, res := range drainCmd(cmd) {
		nm, _ = m.Update(res)
		m = nm.(*TableModel)
	}
	if savedTag != "red" || savedText != "fica de olho no all-in" {
		t.Fatalf("save did not preserve the tag: tag=%q text=%q", savedTag, savedText)
	}
	if m.notes["caio"].Text != "fica de olho no all-in" {
		t.Fatalf("cache not updated after save: %+v", m.notes["caio"])
	}
	if out := strings.Join(m.log, "\n"); !strings.Contains(out, "anotação sobre Caio salva") {
		t.Fatalf("save confirmation missing: %q", out)
	}

	// tag <cor> changes only the tag.
	m, cmd = submitTableLine(m, "/note Caio tag blue")
	for _, res := range drainCmd(cmd) {
		nm, _ = m.Update(res)
		m = nm.(*TableModel)
	}
	if savedTag != "blue" || savedText != "fica de olho no all-in" {
		t.Fatalf("tag change should keep the text: tag=%q text=%q", savedTag, savedText)
	}

	// clear deletes both.
	m, cmd = submitTableLine(m, "/note Caio clear")
	for _, res := range drainCmd(cmd) {
		nm, _ = m.Update(res)
		m = nm.(*TableModel)
	}
	if _, ok := m.notes["caio"]; ok {
		t.Fatalf("cleared note should drop out of the cache: %+v", m.notes)
	}
	if saveCalls != 3 {
		t.Fatalf("expected 3 saves (text, tag, clear), got %d", saveCalls)
	}

	// Invalid tag is rejected before any request.
	saveCalls = 0
	m, cmd = submitTableLine(m, "/note Caio tag magenta")
	if cmd != nil || saveCalls != 0 {
		t.Fatal("invalid tag must not reach SaveNote")
	}
	if out := strings.Join(m.log, "\n"); !strings.Contains(out, "cor inválida") {
		t.Fatalf("invalid tag error missing: %q", out)
	}
}

func TestTablePlayerCommandShowsProfileAndNoteHint(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)

	m, _ = submitTableLine(m, "/player Caio")
	out := strings.Join(m.log, "\n")
	for _, want := range []string{"Caio — D · stack 297", "sem anotação", "/note Caio", "/react <código> Caio"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
}

func TestTableSeatRowShowsNoteTagDot(t *testing.T) {
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Notes: func(context.Context, []string) ([]rest.PlayerNote, error) {
			return []rest.PlayerNote{{OpponentID: "caio", Tag: "red"}}, nil
		},
	})
	nm, cmd := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	for _, res := range drainCmd(cmd) {
		nm, _ = m.Update(res)
		m = nm.(*TableModel)
	}
	nm, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(*TableModel)

	out := ansi.Strip(m.View())
	var caioLine, dudaLine string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "Caio") {
			caioLine = l
		}
		if strings.Contains(l, "Duda") {
			dudaLine = l
		}
	}
	if !strings.Contains(caioLine, "●") {
		t.Fatalf("noted player should show a tag dot: %q", caioLine)
	}
	if strings.Contains(dudaLine, "●") {
		t.Fatalf("un-noted player should not: %q", dudaLine)
	}
}

func TestTableChatHistoryPaneTracksUnreadAndReplays(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)

	nm, _ = m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "chat", PlayerId: "caio", Message: "gg"}})
	m = nm.(*TableModel)
	nm, _ = m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "chat", PlayerId: "you", Message: "vlw"}})
	m = nm.(*TableModel)

	if m.chatUnread != 1 {
		t.Fatalf("only messages from someone else should count as unread, got %d", m.chatUnread)
	}
	if out := m.View(); !strings.Contains(out, "1 mensagem(ns) nova(s)") {
		t.Fatalf("chat badge missing: %q", out)
	}

	m, cmd := submitTableLine(m, "/chat")
	if cmd != nil {
		t.Fatal("/chat is local, no command expected")
	}
	if m.chatUnread != 0 {
		t.Fatalf("viewing /chat should clear unread, got %d", m.chatUnread)
	}
	out := strings.Join(m.log, "\n")
	if !strings.Contains(out, "Caio: gg") || !strings.Contains(out, "você: vlw") {
		t.Fatalf("chat history missing entries: %q", out)
	}
	if strings.Contains(m.View(), "mensagem(ns) nova(s)") {
		t.Fatal("badge should be gone once viewed")
	}
}

func TestTableTimerRendersForOpponentWithBankAndIdleWarning(t *testing.T) {
	msg := tableFixtureSnapshot()
	msg.Snapshot.CurrentPlayerId = "caio"
	msg.Snapshot.LegalActions = nil
	// now = 100s; base clock ends at 115s, hard deadline at 130s (15s bank).
	now := int64(100_000)
	msg.Snapshot.ActionBaseDeadlineUnixMs = 115_000
	msg.Snapshot.ActionDeadlineUnixMs = 130_000
	msg.Snapshot.IdleRemovalUnixMs = 145_000

	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: msg})
	m = nm.(*TableModel)
	m.now = time.UnixMilli(now)
	out := ansi.Strip(m.View())

	if !strings.Contains(out, "Vez de Caio · stack 297 · 15s (+15s banco)") {
		t.Fatalf("opponent clock with bank split missing:\n%s", out)
	}
	if !strings.Contains(out, "Caio sai por inatividade em 45s") {
		t.Fatalf("idle warning for opponent missing:\n%s", out)
	}

	// Push now past the base clock: the actor is now spending the reserve.
	m.now = time.UnixMilli(120_000)
	if out := ansi.Strip(m.View()); !strings.Contains(out, "10s de banco") {
		t.Fatalf("time-bank phase not shown:\n%s", out)
	}
}

func TestTableIdleWarningForViewerTellsThemToAct(t *testing.T) {
	msg := tableFixtureSnapshot() // CurrentPlayerId == "you"
	msg.Snapshot.IdleRemovalUnixMs = 200_000
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: msg})
	m = nm.(*TableModel)
	m.now = time.UnixMilli(190_000)
	if out := ansi.Strip(m.View()); !strings.Contains(out, "você sai por inatividade em 10s — aja ou /keep") {
		t.Fatalf("viewer idle warning missing:\n%s", out)
	}
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
	if !strings.Contains(out, "Board  ") {
		t.Errorf("board should have its own labelled line: %q", out)
	}
	if strings.Contains(out, "R$") {
		t.Errorf("sandbox table must not render R$: %q", out)
	}
}

func TestTableHeaderShowsActorAndDecisionState(t *testing.T) {
	msg := tableFixtureSnapshot()
	msg.Snapshot.CurrentPlayerId = "caio"
	msg.Snapshot.LegalActions = nil
	msg.Snapshot.Seats[0].Contributed = 16
	m := NewTableModel(TableConfig{YouID: "you", RoomName: "quiet-42", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: msg})
	m = nm.(*TableModel)
	out := m.View()
	for _, want := range []string{"Vez de Caio", "stack 297", "aposta 16", "Duda"} {
		if !strings.Contains(out, want) {
			t.Errorf("header missing %q:\n%s", want, out)
		}
	}
}

func TestTableHeaderPrioritizesExecutableLegalActions(t *testing.T) {
	m := NewTableModel(TableConfig{YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	out := m.View()
	for _, want := range []string{"Ações:", "f desistir", "c pagar 8", "r aumentar 16–246"} {
		if !strings.Contains(out, want) {
			t.Errorf("decision surface missing %q:\n%s", want, out)
		}
	}
	wantOrder := []string{"/fold", "/call", "/raise", "/pot"}
	for i, want := range wantOrder {
		if m.menu.specs[i].Name != want {
			t.Fatalf("menu priority[%d] = %q, want %q", i, m.menu.specs[i].Name, want)
		}
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

func TestTablePendingActionShowsStatusBlocksConflictAndConfirmsOnlyMatchingAck(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	typeLine(t, m, "/raise 40")
	actionID := sent[0].ActionId
	if !strings.Contains(m.View(), "enviando aumento para 40") {
		t.Fatalf("pending status missing:\n%s", m.View())
	}
	typeLine(t, m, "/fold")
	if len(sent) != 1 {
		t.Fatalf("conflicting action was sent: %+v", sent)
	}
	if !strings.Contains(strings.Join(m.log, "\n"), "ainda está sendo enviada") {
		t.Fatalf("missing conflict explanation: %v", m.log)
	}
	newer := tableFixtureSnapshot()
	newer.Snapshot.SnapshotVersion = 8
	nm, _ = m.Update(SnapshotMsg{M: newer})
	m = nm.(*TableModel)
	if m.pendingMsg == nil || !strings.Contains(strings.Join(m.log, "\n"), "verificando") {
		t.Fatalf("unrelated snapshot must keep action pending: pending=%+v log=%v", m.pendingMsg, m.log)
	}
	nm, _ = m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "action_ack", ActionId: "another-action"}})
	m = nm.(*TableModel)
	if m.pendingMsg == nil {
		t.Fatal("unrelated acknowledgement cleared the pending action")
	}
	nm, _ = m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "action_ack", ActionId: actionID}})
	m = nm.(*TableModel)
	if m.pendingMsg != nil || !strings.Contains(strings.Join(m.log, "\n"), "confirmada") {
		t.Fatalf("matching acknowledgement did not confirm action: pending=%+v log=%v", m.pendingMsg, m.log)
	}
}

func TestTableReconnectPausesNetworkActionsAndResendsPending(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	typeLine(t, m, "/raise 40")
	firstID := sent[0].ActionId
	nm, _ = m.Update(ReconnectingMsg{})
	m = nm.(*TableModel)
	typeLine(t, m, "/talk teste")
	if len(sent) != 1 {
		t.Fatalf("network action sent while reconnecting: %+v", sent)
	}
	nm, cmd := m.Update(ReconnectedMsg{})
	m = nm.(*TableModel)
	for _, msg := range drainCmd(cmd) {
		if msg != nil {
			m.Update(msg)
		}
	}
	if len(sent) != 2 || sent[1].ActionId != firstID {
		t.Fatalf("pending action not resent idempotently: %+v", sent)
	}
}

func TestTableFatalStateOffersExitAndBlocksNetworkActions(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	nm, _ = m.Update(TableFatalMsg{Err: errors.New("socket fechou")})
	m = nm.(*TableModel)
	if !strings.Contains(m.View(), "/exit volta ao lobby") {
		t.Fatalf("fatal recovery missing:\n%s", m.View())
	}
	typeLine(t, m, "/call")
	if len(sent) != 0 {
		t.Fatalf("action sent after fatal failure: %+v", sent)
	}
}

func TestTableFatalStateIgnoresLateReconnect(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	nm, _ = m.Update(TableFatalMsg{Err: errors.New("socket fechou")})
	m = nm.(*TableModel)
	nm, cmd := m.Update(ReconnectedMsg{})
	m = nm.(*TableModel)
	if cmd != nil || !m.fatal {
		t.Fatalf("late reconnect reopened fatal table: fatal=%v cmd=%v", m.fatal, cmd)
	}
	typeLine(t, m, "/call")
	if len(sent) != 0 {
		t.Fatalf("network action sent after fatal -> reconnect ordering: %+v", sent)
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
	m := NewTableModel(TableConfig{RoomID: "r-1", RoomName: "Aurora", YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6})
	nm, cmd := m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "removed", Code: "exit_requested", Amount: 1500}})
	m = nm.(*TableModel)
	if cmd == nil {
		t.Fatal("removed should emit a command")
	}
	exited, ok := cmd().(TableExitedMsg)
	if !ok {
		t.Fatalf("removed should emit TableExitedMsg, got %T", cmd())
	}
	if exited.RoomName != "Aurora" || exited.Reason != "exit_requested" || !exited.SettlementKnown || exited.SettledAmount != 1500 {
		t.Errorf("incomplete exit handoff: %+v", exited)
	}
	if !strings.Contains(strings.Join(m.log, "\n"), "1500") {
		t.Errorf("settled stack not logged: %v", m.log)
	}
}

func TestTableExitRequestsExitAndStaysConnected(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)

	for _, r := range "/exit" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(*TableModel)
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(*TableModel)

	for _, msg := range drainCmd(cmd) {
		if _, ok := msg.(TableExitedMsg); ok {
			t.Fatal("/exit must not leave the table before the server removes the player")
		}
	}
	if len(sent) != 1 || sent[0].Type != "request_exit" {
		t.Fatalf("expected one request_exit frame, got %+v", sent)
	}
	if !m.exitRequested {
		t.Error("exitRequested not set")
	}
	if m.exitActionID == "" || m.exitActionID != sent[0].ActionId {
		t.Fatalf("exit acknowledgement correlation not tracked: model=%q sent=%q", m.exitActionID, sent[0].ActionId)
	}
	nm, _ = m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "action_ack", ActionId: m.exitActionID}})
	m = nm.(*TableModel)
	if m.exitActionID != "" || !strings.Contains(strings.Join(m.log, "\n"), "saída confirmada") {
		t.Fatalf("exit acknowledgement not surfaced: actionID=%q log=%v", m.exitActionID, m.log)
	}

	// The server eventually removes us -> that is what returns to the lobby.
	nm, cmd = m.Update(SnapshotMsg{M: &proto.ServerMessage{Type: "removed", Message: "left", Amount: 246}})
	if _, ok := cmd().(TableExitedMsg); !ok {
		t.Errorf("removed should emit TableExitedMsg, got %T", cmd())
	}
}

func TestTableExitRejectionUnlocksRetry(t *testing.T) {
	var sent []*proto.ClientMessage
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6,
		Send: func(cm *proto.ClientMessage) error { sent = append(sent, cm); return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)
	typeLine(t, m, "/exit")
	actionID := sent[0].ActionId
	nm, _ = m.Update(SnapshotMsg{M: &proto.ServerMessage{
		Type: "error", ActionId: actionID, Code: "unavailable", Message: "mesa ocupada",
	}})
	m = nm.(*TableModel)
	if m.exitRequested || m.exitActionID != "" {
		t.Fatalf("rejected exit stayed locked: requested=%v actionID=%q", m.exitRequested, m.exitActionID)
	}
	if !strings.Contains(strings.Join(m.log, "\n"), "não foi possível solicitar a saída") {
		t.Fatalf("exit rejection lacks recovery copy: %v", m.log)
	}
}

func TestTableForceExitLeavesImmediately(t *testing.T) {
	m := NewTableModel(TableConfig{
		YouID: "you", Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		Send: func(cm *proto.ClientMessage) error { return nil },
	})
	nm, _ := m.Update(SnapshotMsg{M: tableFixtureSnapshot()})
	m = nm.(*TableModel)

	for _, r := range "/exit!" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(*TableModel)
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	var exited bool
	for _, msg := range drainCmd(cmd) {
		if _, ok := msg.(TableExitedMsg); ok {
			exited = true
		}
	}
	if !exited {
		t.Fatal("/exit! should emit TableExitedMsg immediately")
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
	for _, want := range []string{"p /pot", "k /peek", "c /check ou /call"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing derived shortcut %q: %q", want, out)
		}
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

func TestTableViewNeverOverflowsTerminalWidth(t *testing.T) {
	for _, width := range []int{20, 40, 60, 80} {
		msg := tableFixtureSnapshot()
		msg.Snapshot.Seats[0].Name = "Jogador界🙂com-um-nome-muito-longo"
		m := NewTableModel(TableConfig{
			YouID: "you", RoomName: "mesa界🙂com-um-identificador-muito-longo",
			Blinds: [2]int64{1, 2}, MaxSeats: 6, CardMode: game.CardASCII,
		})
		nm, _ := m.Update(SnapshotMsg{M: msg})
		m = nm.(*TableModel)
		nm, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m = nm.(*TableModel)
		for _, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got > width-1 {
				t.Fatalf("width=%d line=%q occupies %d cells, want <= %d", width, line, got, width-1)
			}
		}
	}
}
