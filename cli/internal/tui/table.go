package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/proto"
)

// TableConfig wires a TableModel to its transport and identity.
type TableConfig struct {
	RoomID    string
	RoomName  string
	ShareCode string
	RealMoney bool
	Blinds    [2]int64
	MaxSeats  int
	CardMode  game.CardMode
	YouID     string

	// Send delivers a ClientMessage to the table socket.
	Send func(*proto.ClientMessage) error
	// Session backs /summary (nil disables it).
	CurrentSession func(context.Context) (summary string, err error)
}

// TableExitedMsg is emitted when the player leaves the table (via /exit, a
// forced exit, or a "removed" frame). The parent model returns to its own
// prior screen; the table itself never calls tea.Quit.
type TableExitedMsg struct{}

func exitCmd() tea.Cmd { return func() tea.Msg { return TableExitedMsg{} } }

// Table messages.
type (
	// SnapshotMsg carries one decoded ServerMessage from the table socket.
	SnapshotMsg struct{ M *proto.ServerMessage }
	// TableTickMsg drives the 1s countdown redraw.
	TableTickMsg time.Time
	// ReconnectingMsg / ReconnectedMsg toggle the connection banner.
	ReconnectingMsg struct{}
	ReconnectedMsg  struct{}
	// TableFatalMsg ends the table view with an error.
	TableFatalMsg struct{ Err error }
	sessionSummaryMsg struct {
		text string
		err  error
	}
)

// TableModel renders one poker table (Layout B) and turns input into
// ClientMessages.
type TableModel struct {
	cfg      TableConfig
	view     game.TableView
	haveView bool
	narr     *game.Narrator
	log      []string
	input    textinput.Model
	now      time.Time
	reconnecting bool

	pendingActionID string
	pendingMsg      *proto.ClientMessage
	quit            bool
}

// NewTableModel builds the table view for cfg.
func NewTableModel(cfg TableConfig) *TableModel {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Focus()
	return &TableModel{
		cfg:   cfg,
		narr:  game.NewNarrator(cfg.YouID).WithCardMode(cfg.CardMode),
		input: ti,
		now:   time.Now(),
	}
}

func (m *TableModel) Init() tea.Cmd { return tableTick() }

func tableTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return TableTickMsg(t) })
}

func (m *TableModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TableTickMsg:
		m.now = time.Time(msg)
		return m, tableTick()

	case ReconnectingMsg:
		m.reconnecting = true
		m.appendLog("· reconectando…")
		return m, nil
	case ReconnectedMsg:
		m.reconnecting = false
		m.appendLog("· reconectado")
		return m, nil

	case TableFatalMsg:
		m.appendLog("erro: " + msg.Err.Error())
		return m, nil

	case sessionSummaryMsg:
		if msg.err != nil {
			m.appendLog("resumo indisponível: " + msg.err.Error())
		} else {
			m.appendLog(msg.text)
		}
		return m, nil

	case SnapshotMsg:
		return m, m.handleServerMessage(msg.M)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *TableModel) handleServerMessage(sm *proto.ServerMessage) tea.Cmd {
	switch sm.Type {
	case "state":
		if sm.Snapshot == nil {
			return nil
		}
		for _, line := range m.narr.OnSnapshot(sm.Snapshot) {
			m.appendLog(line)
		}
		m.view = game.NewTableView(sm.Snapshot, m.cfg.YouID, m.cfg.RoomName, m.cfg.RealMoney, m.cfg.Blinds, m.cfg.MaxSeats, m.cfg.CardMode)
		m.haveView = true
		return nil

	case "error":
		if sm.ActionId != "" && sm.ActionId == m.pendingActionID {
			pending := m.pendingMsg
			m.pendingActionID = ""
			m.pendingMsg = nil
			if sm.Code == "unavailable" && pending != nil {
				m.appendLog("· instável, re-sincronizando e reenviando")
				m.pendingActionID = pending.ActionId
				m.pendingMsg = pending
				return func() tea.Msg {
					if m.cfg.Send == nil {
						return nil
					}
					if err := m.cfg.Send(&proto.ClientMessage{Type: "sync_state"}); err != nil {
						return TableFatalMsg{Err: err}
					}
					// same ActionId — the server's idempotency guard collapses the retry
					if err := m.cfg.Send(pending); err != nil {
						return TableFatalMsg{Err: err}
					}
					return nil
				}
			}
			m.appendLog("ação inválida: " + sm.Message)
			return nil
		}
		m.appendLog("erro: " + sm.Message)
		return nil

	case "removed":
		m.appendLog(strings.Join(m.narr.OnMessage(sm), "\n"))
		m.quit = true
		return exitCmd()

	case "chat", "reaction", "achievement_unlocked":
		if lines := m.narr.OnMessage(sm); len(lines) > 0 {
			m.appendLog(strings.Join(lines, "\n"))
		}
		return nil
	}
	return nil
}

func (m *TableModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		m.quit = true
		return m, exitCmd()
	}
	if msg.String() != "enter" {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	line := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	if line == "" {
		return m, nil
	}
	m.appendLog(m.input.Prompt + line)

	cm, local, err := ParseTableCommand(line, m.view)
	if err != nil {
		m.appendLog("· " + err.Error())
		return m, nil
	}
	if local == ActExit && cm != nil {
		// tell the server we're leaving, then exit locally
		return m, tea.Batch(m.send(cm), exitCmd())
	}
	if cmd := m.runLocal(local); cmd != nil || local != ActNone {
		return m, cmd
	}
	if cm != nil {
		if cm.Type == "act" {
			m.pendingActionID = cm.ActionId
			m.pendingMsg = cm
		}
		return m, m.send(cm)
	}
	return m, nil
}

func (m *TableModel) runLocal(local LocalAction) tea.Cmd {
	switch local {
	case ActHelp:
		m.appendLog(tableHelp)
	case ActLastWinners:
		w := m.narr.LastWinners(5)
		if len(w) == 0 {
			m.appendLog("· ainda sem mãos concluídas")
		} else {
			m.appendLog("últimos vencedores: " + strings.Join(w, " · "))
		}
	case ActShare:
		if m.cfg.ShareCode == "" {
			m.appendLog("· mesa pública, sem código de convite")
		} else {
			url := "https://poker.aoctech.app/table?code=" + m.cfg.ShareCode
			m.appendLog("convite: " + url)
			_ = copyClipboard(url)
		}
	case ActSummary:
		if m.cfg.CurrentSession == nil {
			m.appendLog("· resumo indisponível nesta sessão")
			return nil
		}
		return func() tea.Msg {
			text, err := m.cfg.CurrentSession(context.Background())
			return sessionSummaryMsg{text: text, err: err}
		}
	case ActExit:
		m.appendLog("· saindo da mesa…")
		m.quit = true
		return exitCmd()
	case ActForceExit:
		m.quit = true
		return exitCmd()
	}
	return nil
}

func (m *TableModel) send(cm *proto.ClientMessage) tea.Cmd {
	return func() tea.Msg {
		if m.cfg.Send == nil {
			return nil
		}
		if err := m.cfg.Send(cm); err != nil {
			return TableFatalMsg{Err: err}
		}
		return nil
	}
}

func (m *TableModel) appendLog(line string) {
	if line == "" {
		return
	}
	m.log = append(m.log, line)
	if len(m.log) > 200 {
		m.log = m.log[len(m.log)-200:]
	}
}

const tableHelp = "comandos: /check /call /raise <v> /pot /allin /fold /talk <msg> /react <cod> [j] /peek [all|1|2] /sitout /ready /summary /last-winners /share /exit  ·  teclas: f c r p k"

func copyClipboard(s string) error {
	// clipboard is best-effort; a headless environment simply won't have one.
	return clipboardWrite(s)
}

// View renders Layout B.
func (m *TableModel) View() string {
	if m.quit {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 76))
	b.WriteString("\n")

	start := 0
	if len(m.log) > 14 {
		start = len(m.log) - 14
	}
	b.WriteString(strings.Join(m.log[start:], "\n"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 76))
	b.WriteString("\n")
	if m.reconnecting {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("(reconectando — última mesa exibida)") + "\n")
	}
	b.WriteString(m.input.View())
	return b.String()
}

func (m *TableModel) header() string {
	if !m.haveView {
		return "conectando à mesa…"
	}
	v := m.view
	money := func(n int64) string {
		if v.RealMoney {
			return fmt.Sprintf("R$ %d", n)
		}
		return fmt.Sprintf("%d", n)
	}

	name := v.RoomName
	if name == "" {
		name = v.RoomID
	}
	var pos []string
	for _, p := range v.Players {
		if p.Position == "" {
			continue
		}
		tag := fmt.Sprintf("[%s] %s", p.Position, p.Name)
		if p.IsYou {
			tag = lipgloss.NewStyle().Bold(true).Render("[" + p.Position + "] VOCÊ")
		}
		pos = append(pos, tag)
	}

	turn := ""
	if v.IsYourTurn {
		secs := int64(0)
		if v.ActionDeadlineMS > 0 {
			secs = (v.ActionDeadlineMS - m.now.UnixMilli()) / 1000
		}
		turn = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(fmt.Sprintf("● SUA VEZ  %ds", secs))
	}

	hand := ""
	if len(v.YourHole) > 0 {
		hand = " Sua mão " + game.FormatCards(v.YourHole, m.cfg.CardMode)
		if v.YourStrength != "" {
			hand += " · " + v.YourStrength
		}
		if v.YourEquity >= 0 {
			hand += fmt.Sprintf(" · ~%.0f%%", v.YourEquity*100)
		}
	}

	legalLine := ""
	if v.IsYourTurn && v.Legal != nil {
		var parts []string
		for _, a := range v.Legal.Actions {
			switch a {
			case "call":
				parts = append(parts, fmt.Sprintf("call %d", v.Legal.CallAmount))
			case "raise":
				parts = append(parts, fmt.Sprintf("raise %d–%d", v.Legal.MinRaiseTo, v.Legal.MaxRaiseTo))
			default:
				parts = append(parts, a)
			}
		}
		legalLine = "\n  → " + strings.Join(parts, " · ")
	}

	return fmt.Sprintf(
		"─── %s ── No-Limit Hold'em ── Blinds %s / %s ── %d/%d jogadores ───\n  Pote %s   Board %s  %s\n  Posições  %s     %s%s",
		name, money(v.SmallBlind), money(v.BigBlind), v.Seated, v.MaxSeats,
		money(v.Pot), game.FormatCards(v.Board, m.cfg.CardMode), hand,
		strings.Join(pos, " · "), turn, legalLine,
	)
}
