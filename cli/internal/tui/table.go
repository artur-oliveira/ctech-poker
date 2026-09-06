package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
type TableExitedMsg struct {
	RoomID          string
	RoomName        string
	Reason          string
	SettledAmount   int64
	SettlementKnown bool
	RealMoney       bool
}

func exitCmd(msg TableExitedMsg) tea.Cmd { return func() tea.Msg { return msg } }

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
	TableFatalMsg     struct{ Err error }
	sessionSummaryMsg struct {
		text string
		err  error
	}
)

// TableModel renders one poker table (Layout B) and turns input into
// ClientMessages.
type TableModel struct {
	cfg          TableConfig
	view         game.TableView
	haveView     bool
	narr         *game.Narrator
	log          []string
	input        textinput.Model
	menu         *commandMenu
	viewport     viewport.Model
	vpReady      bool
	windowWidth  int
	windowHeight int
	followBottom bool // true unless the user scrolled away from the latest output
	now          time.Time
	reconnecting bool
	fatal        bool

	pendingActionID string
	pendingMsg      *proto.ClientMessage
	pendingVersion  uint64
	pendingSyncSeen bool
	exitRequested   bool // /exit sent request_exit; waiting for the server's "removed"
	exitActionID    string
	quit            bool
}

// NewTableModel builds the table view for cfg.
func NewTableModel(cfg TableConfig) *TableModel {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = promptStyle
	ti.Placeholder = "/ para ver comandos"
	ti.Focus()
	return &TableModel{
		cfg: cfg, narr: game.NewNarrator(cfg.YouID).WithCardMode(cfg.CardMode),
		input: ti, menu: newCommandMenu(tableCommandSpecs),
		now: time.Now(), followBottom: true,
	}
}

func (m *TableModel) Init() tea.Cmd { return tableTick() }

func tableTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return TableTickMsg(t) })
}

func (m *TableModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.input.Width = terminalLineWidth(msg.Width) - 2
		if m.input.Width < 1 {
			m.input.Width = 1
		}
		if !m.vpReady {
			m.viewport = viewport.New(terminalLineWidth(msg.Width), 0)
			m.vpReady = true
		} else {
			m.viewport.Width = terminalLineWidth(msg.Width)
		}
		m.syncViewport()
		return m, nil

	case TableTickMsg:
		m.now = time.Time(msg)
		return m, tableTick()

	case ReconnectingMsg:
		if !m.reconnecting {
			m.appendLog("· conexão instável; reconectando sem perder a mesa…")
		}
		m.reconnecting = true
		m.refreshCommandMenu()
		m.refreshInputHint()
		return m, nil
	case ReconnectedMsg:
		// A fatal table failure is terminal. A delayed transport event must not
		// silently reopen commands after the UI has told the player to leave.
		if m.fatal {
			return m, nil
		}
		m.reconnecting = false
		m.refreshCommandMenu()
		m.refreshInputHint()
		if m.pendingMsg != nil {
			m.appendLog("· reconectado; reenviando " + m.actionDescription(m.pendingMsg) + "…")
			return m, m.send(m.pendingMsg)
		}
		m.appendLog(successStyle.Render("✓ reconectado"))
		return m, nil

	case TableFatalMsg:
		m.reconnecting = false
		m.fatal = true
		m.pendingActionID = ""
		m.pendingMsg = nil
		m.refreshCommandMenu()
		m.refreshInputHint()
		detail := "erro desconhecido"
		if msg.Err != nil {
			detail = msg.Err.Error()
		}
		m.appendLog(errorStyle.Render("conexão encerrada: " + detail + " · use /exit para voltar ao lobby"))
		return m, nil

	case sessionSummaryMsg:
		if msg.err != nil {
			m.appendLog(errorStyle.Render("resumo indisponível: " + msg.err.Error()))
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
		if m.pendingMsg != nil && sm.Snapshot.SnapshotVersion > m.pendingVersion && !m.pendingSyncSeen {
			// Snapshot advancement is not proof that *our* action was applied: chat,
			// another player, or sync_state may also advance it. Only a correlated
			// action_ack below may claim success.
			m.appendLog(mutedStyle.Render("· mesa atualizada; verificando " + m.actionDescription(m.pendingMsg) + "…"))
			m.pendingSyncSeen = true
		}
		for _, line := range m.narr.OnSnapshot(sm.Snapshot) {
			m.appendLog(line)
		}
		m.view = game.NewTableView(sm.Snapshot, m.cfg.YouID, m.cfg.RoomName, m.cfg.RealMoney, m.cfg.Blinds, m.cfg.MaxSeats, m.cfg.CardMode)
		m.haveView = true
		m.refreshCommandMenu()
		m.refreshInputHint()
		return nil

	case "action_ack":
		if sm.ActionId != "" && sm.ActionId == m.pendingActionID && m.pendingMsg != nil {
			m.appendLog(successStyle.Render("✓ " + m.actionDescription(m.pendingMsg) + " confirmada"))
			m.clearPendingAction()
			m.refreshCommandMenu()
			m.refreshInputHint()
			return nil
		}
		if sm.ActionId != "" && sm.ActionId == m.exitActionID {
			m.exitActionID = ""
			m.appendLog(successStyle.Render("✓ saída confirmada; aguardando o encerramento da mesa"))
			m.refreshInputHint()
		}
		return nil

	case "error":
		if sm.ActionId != "" && sm.ActionId == m.pendingActionID {
			pending := m.pendingMsg
			m.clearPendingAction()
			if sm.Code == "unavailable" && pending != nil {
				m.appendLog("· instável, re-sincronizando e reenviando")
				m.pendingActionID = pending.ActionId
				m.pendingMsg = pending
				m.pendingSyncSeen = false
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
			m.appendLog(errorStyle.Render("ação inválida: " + sm.Message))
			m.refreshInputHint()
			return nil
		}
		if sm.ActionId != "" && sm.ActionId == m.exitActionID {
			m.exitActionID = ""
			m.exitRequested = false
			m.appendLog(errorStyle.Render("· não foi possível solicitar a saída: " + sm.Message))
			m.refreshInputHint()
			return nil
		}
		m.appendLog(errorStyle.Render("erro: " + sm.Message))
		return nil

	case "removed":
		m.appendLog(strings.Join(m.narr.OnMessage(sm), "\n"))
		m.quit = true
		exited := m.exitDetails(sm.Code, sm.Message, sm.Amount, true)
		return exitCmd(exited)

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
		return m, exitCmd(m.exitDetails("local", "atalho Ctrl+C", 0, false))
	}

	switch msg.Type {
	case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		m.followBottom = m.viewport.AtBottom()
		return m, cmd
	case tea.KeyCtrlL:
		m.log = nil
		m.followBottom = true
		m.syncViewport()
		return m, nil
	}

	if m.menu.visible {
		switch msg.Type {
		case tea.KeyUp:
			m.menu.movePrev()
			return m, nil
		case tea.KeyDown:
			m.menu.moveNext()
			return m, nil
		case tea.KeyTab:
			if val := m.menu.fill(); val != "" {
				m.input.SetValue(val)
				m.input.CursorEnd()
				m.menu.UpdateInput(val)
				m.syncViewport()
			}
			return m, nil
		case tea.KeyEnter:
			val, submit := m.menu.accept()
			if val == "" {
				return m, nil
			}
			m.input.SetValue(val)
			m.input.CursorEnd()
			m.syncViewport()
			if submit {
				return m.submitLine()
			}
			return m, nil
		case tea.KeyEsc:
			m.menu.hide()
			m.syncViewport()
			return m, nil
		}
	}

	if msg.String() == "enter" {
		return m.submitLine()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.menu.UpdateInput(m.input.Value())
	m.syncViewport()
	return m, cmd
}

func (m *TableModel) submitLine() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	m.menu.hide()
	if line == "" {
		return m, nil
	}
	m.appendLog(m.input.Prompt + line)

	cm, local, err := ParseTableCommand(line, m.view)
	if err != nil {
		m.appendLog(errorStyle.Render("· " + err.Error()))
		return m, nil
	}
	if local == ActExit {
		// Mirror the web UI: send request_exit and STAY connected. The server
		// removes the player once no hand is in progress and sends a "removed"
		// frame — that is what returns us to the lobby (see the "removed" case
		// above). Only leave locally when the socket can't carry the request
		// or we aren't seated in the first place.
		if m.reconnecting || m.fatal || cm == nil || m.view.You.ID == "" {
			m.appendLog("· saindo da mesa localmente…")
			m.quit = true
			return m, exitCmd(m.exitDetails("local", "conexão indisponível", 0, false))
		}
		if m.exitRequested {
			m.appendLog(mutedStyle.Render("· saída já solicitada — use /exit! para sair agora"))
			return m, nil
		}
		m.exitRequested = true
		m.exitActionID = cm.ActionId
		m.appendLog("· saída solicitada; você sai ao fim da mão · /exit! para sair já")
		m.refreshInputHint()
		return m, m.send(cm)
	}
	if local == ActForceExit {
		m.quit = true
		return m, exitCmd(m.exitDetails("forced", "saída imediata", 0, false))
	}
	if cmd := m.runLocal(local); cmd != nil || local != ActNone {
		return m, cmd
	}
	if cm != nil {
		if m.fatal {
			m.appendLog(errorStyle.Render("· conexão encerrada; use /exit para voltar ao lobby"))
			return m, nil
		}
		if m.reconnecting {
			m.appendLog(mutedStyle.Render("· reconectando; aguarde antes de enviar outra ação"))
			return m, nil
		}
		if cm.Type == "act" {
			if m.pendingMsg != nil {
				m.appendLog(mutedStyle.Render("· " + m.actionDescription(m.pendingMsg) + " ainda está sendo enviada"))
				return m, nil
			}
			m.pendingActionID = cm.ActionId
			m.pendingMsg = cm
			m.pendingVersion = m.view.SnapshotVersion
			m.pendingSyncSeen = false
			m.refreshCommandMenu()
			m.refreshInputHint()
		}
		return m, m.send(cm)
	}
	return m, nil
}

func (m *TableModel) clearPendingAction() {
	m.pendingActionID = ""
	m.pendingMsg = nil
	m.pendingSyncSeen = false
}

func (m *TableModel) exitDetails(code, message string, amount int64, settled bool) TableExitedMsg {
	reason := strings.TrimSpace(code)
	if reason == "" {
		reason = strings.TrimSpace(message)
	}
	exited := TableExitedMsg{
		RoomID: m.cfg.RoomID, RoomName: m.cfg.RoomName, Reason: reason,
		SettledAmount: amount, SettlementKnown: settled, RealMoney: m.cfg.RealMoney,
	}
	return exited
}

func (m *TableModel) runLocal(local LocalAction) tea.Cmd {
	switch local {
	case ActHelp:
		m.appendLog(formatCommandList(tableCommandSpecs, m.windowWidth) + "\n  " + formatHotkeyHelp(tableCommandSpecs))
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
			if err := copyClipboard(url); err != nil {
				m.appendLog("convite: " + url)
				m.appendLog(mutedStyle.Render("· não foi possível copiar; selecione o link acima"))
			} else {
				m.appendLog(successStyle.Render("✓ convite copiado: ") + url)
			}
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
	case ActClear:
		m.log = nil
		m.syncViewport()
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

// appendLog appends line to the scrollback and keeps the viewport following
// the bottom unless the user has scrolled up to read history.
// appendLog appends line to the log, splitting it first if it contains
// embedded newlines — see Shell.appendLine's doc comment for why this split
// is load-bearing (layoutHeights sizes the viewport from len(m.log) on the
// invariant that one slice element is one visual line; several callers here
// join multi-line narration with strings.Join before calling this once).
func (m *TableModel) appendLog(line string) {
	if line == "" {
		return
	}
	if strings.Contains(line, "\n") {
		m.log = append(m.log, strings.Split(line, "\n")...)
	} else {
		m.log = append(m.log, line)
	}
	m.syncViewport()
}

// syncViewport refreshes the log viewport's content only — never its scroll
// position. GotoBottom needs the viewport's Height to already reflect the
// content that's about to be shown, which is only true right after
// layoutHeights runs in View; calling it here against whatever Height an
// earlier render happened to leave behind computes an offset for a size
// that's about to change, e.g. showing only a result's last line with the
// rest blank the moment a burst of new lines arrives (see Shell.syncViewport
// for the full incident this mirrors). followBottom (kept in sync by the
// scroll keys in handleKey) is what View consults instead.
func (m *TableModel) syncViewport() {
	if !m.vpReady {
		return
	}
	m.viewport.SetContent(strings.Join(m.displayLogLines(m.windowWidth), "\n"))
}

func (m *TableModel) displayLogLines(maxWidth int) []string {
	width := terminalLineWidth(maxWidth)
	lines := make([]string, 0, len(m.log))
	for _, line := range m.log {
		wrapped := ansi.Hardwrap(line, width, false)
		lines = append(lines, strings.Split(wrapped, "\n")...)
	}
	return lines
}

// layoutHeights splits the room below the header between the scrolling log
// and the command menu so their combined total plus the fixed chrome
// (header, the two rule lines, the reconnect banner, input) can never
// exceed the terminal's actual height, on any window size — see
// Shell.layoutHeights's identical reasoning. The menu wins any contested
// space; the log shrinks to make room, down to zero rather than overflow.
func (m *TableModel) layoutHeights(header string) (viewportH, menuRows int) {
	chrome := strings.Count(header, "\n") + 1 // header
	chrome += 2                               // the two rule lines
	chrome++                                  // input
	if m.interactionStatus() != "" {
		chrome++
	}
	avail := m.windowHeight - chrome
	if avail < 0 {
		avail = 0
	}
	menuRows = m.menu.DesiredRows()
	if menuRows > 5 { // at most four choices plus the hidden-count row
		menuRows = 5
	}
	if menuRows > avail {
		menuRows = avail
	}
	vpBudget := avail - menuRows
	viewportH = len(m.displayLogLines(m.windowWidth))
	if viewportH > vpBudget {
		viewportH = vpBudget
	}
	if viewportH < 0 {
		viewportH = 0
	}
	return viewportH, menuRows
}

func copyClipboard(s string) error {
	// clipboard is best-effort; a headless environment simply won't have one.
	return clipboardWrite(s)
}

// View renders Layout B.
func (m *TableModel) View() string {
	if m.quit {
		return ""
	}
	header := m.header(m.windowWidth)
	ruleWidth := terminalLineWidth(m.windowWidth)
	if ruleWidth > 76 {
		ruleWidth = 76
	}
	rule := dimStyle.Render(strings.Repeat("─", ruleWidth))
	lines := []string{header, rule}

	if m.vpReady {
		vpH, menuRows := m.layoutHeights(header)
		m.viewport.Height = vpH
		if m.followBottom {
			m.viewport.GotoBottom()
		}
		if vpH > 0 {
			lines = append(lines, m.viewport.View())
		}
		lines = append(lines, rule)
		if status := m.interactionStatus(); status != "" {
			lines = append(lines, truncateVisible(status, terminalLineWidth(m.windowWidth)))
		}
		lines = append(lines, m.input.View())
		if menuRows > 0 {
			if menuView := m.menu.View(menuRows, m.windowWidth); menuView != "" {
				lines = append(lines, menuView)
			}
		}
	} else {
		// No WindowSizeMsg yet — render unconstrained rather than guess a size.
		displayLog := m.displayLogLines(0)
		start := 0
		if len(displayLog) > 14 {
			start = len(displayLog) - 14
		}
		lines = append(lines, strings.Join(displayLog[start:], "\n"), rule)
		if status := m.interactionStatus(); status != "" {
			lines = append(lines, truncateVisible(status, terminalLineWidth(m.windowWidth)))
		}
		lines = append(lines, m.input.View())
		if menuView := m.menu.View(maxMenuRows+1, 0); menuView != "" {
			lines = append(lines, menuView)
		}
	}
	return padViewHeight(strings.Join(lines, "\n"), m.windowHeight)
}

func (m *TableModel) header(maxWidth int) string {
	width := terminalLineWidth(maxWidth)
	if !m.haveView {
		return truncateVisible("conectando à mesa…", width)
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
	stage := stageLabel(v.Stage)
	board := game.FormatCards(v.Board, m.cfg.CardMode)
	if board == "" {
		board = "pré-flop"
	}
	hand := "Sua mão: —"
	if len(v.YourHole) > 0 {
		hand = "Sua mão: " + game.FormatCards(v.YourHole, m.cfg.CardMode)
		if v.YourStrength != "" {
			hand += " · " + v.YourStrength
		}
		if v.YourEquity >= 0 {
			hand += fmt.Sprintf(" · ~%.0f%%", v.YourEquity*100)
		}
	}

	var lines []string
	switch {
	case width >= 72:
		lines = append(lines,
			fmt.Sprintf("Mesa %s · No-Limit Hold'em · %s · blinds %s/%s · %d/%d",
				titleStyle.Render(name), stage, money(v.SmallBlind), money(v.BigBlind), v.Seated, v.MaxSeats),
			fmt.Sprintf("Pote: %s · Mesa: %s · %s", money(v.Pot), board, hand),
		)
		lines = append(lines, packHeaderSegments("Jogadores: ", m.playerSegments(money, false), width, 2)...)
	case width >= 48:
		lines = append(lines,
			fmt.Sprintf("Mesa %s · %s · blinds %s/%s", titleStyle.Render(name), stage, money(v.SmallBlind), money(v.BigBlind)),
			fmt.Sprintf("Pote: %s · Mesa: %s", money(v.Pot), board),
			hand,
		)
		lines = append(lines, packHeaderSegments("Em foco: ", m.playerSegments(money, true), width, 1)...)
	default:
		lines = append(lines,
			fmt.Sprintf("%s · %s", titleStyle.Render(name), stage),
			fmt.Sprintf("Pote %s · Mesa %s", money(v.Pot), board),
			hand,
		)
	}

	lines = append(lines, m.actorLine(money))
	if actions := m.actionSegments(money); len(actions) > 0 {
		lines = append(lines, packHeaderSegments("Ações: ", actions, width, 3)...)
	}
	for i := range lines {
		lines[i] = truncateVisible(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func stageLabel(stage string) string {
	switch strings.ToLower(stage) {
	case "preflop", "pre_flop":
		return "pré-flop"
	case "flop":
		return "flop"
	case "turn":
		return "turn"
	case "river":
		return "river"
	case "complete":
		return "showdown"
	default:
		if stage == "" {
			return "aguardando mão"
		}
		return stage
	}
}

func (m *TableModel) playerSegments(money func(int64) string, focused bool) []string {
	var out []string
	for _, p := range m.view.Players {
		if focused && !p.IsYou && p.ID != m.view.CurrentPlayer.ID {
			continue
		}
		position := p.Position
		if position == "" {
			position = "—"
		}
		name := p.Name
		if p.IsYou {
			name = "VOCÊ"
		}
		prefix := ""
		if p.ID == m.view.CurrentPlayer.ID {
			prefix = "▶ "
		}
		segment := fmt.Sprintf("%s[%s] %s %s", prefix, position, name, money(p.Stack))
		if p.Committed > 0 {
			segment += fmt.Sprintf(" (+%s)", money(p.Committed))
		}
		switch {
		case p.Folded:
			segment += " desistiu"
		case p.SittingOut:
			segment += " fora"
		}
		if p.IsYou {
			segment = accentStyle.Bold(true).Render(segment)
		}
		out = append(out, segment)
	}
	return out
}

func (m *TableModel) actorLine(money func(int64) string) string {
	p := m.view.CurrentPlayer
	if p.ID == "" {
		return mutedStyle.Render("Aguardando a próxima mão")
	}
	if p.IsYou {
		if m.view.ActionDeadlineMS > 0 {
			secs := (m.view.ActionDeadlineMS - m.now.UnixMilli()) / 1000
			if secs <= 0 {
				return mutedStyle.Render("Tempo esgotado · aguardando a mesa")
			}
			return successStyle.Bold(true).Render(fmt.Sprintf("● SUA VEZ · %ds", secs))
		}
		return successStyle.Bold(true).Render("● SUA VEZ")
	}
	name := p.Name
	if name == "" {
		name = "outro jogador"
	}
	return fmt.Sprintf("Vez de %s · stack %s", name, money(p.Stack))
}

func (m *TableModel) actionSegments(money func(int64) string) []string {
	if !m.view.IsYourTurn || m.view.Legal == nil {
		return nil
	}
	var out []string
	for _, action := range m.view.Legal.Actions {
		switch action {
		case "fold":
			out = append(out, "f desistir")
		case "check":
			out = append(out, "c passar")
		case "call":
			out = append(out, "c pagar "+money(m.view.Legal.CallAmount))
		case "raise":
			out = append(out, fmt.Sprintf("r aumentar %s–%s", money(m.view.Legal.MinRaiseTo), money(m.view.Legal.MaxRaiseTo)))
		}
	}
	return out
}

func packHeaderSegments(prefix string, segments []string, width, maxLines int) []string {
	if len(segments) == 0 || maxLines <= 0 {
		return nil
	}
	lines := []string{}
	line := prefix
	for _, segment := range segments {
		candidate := line + segment
		if line != prefix {
			candidate = line + " · " + segment
		}
		if visibleWidth(candidate) <= width {
			line = candidate
			continue
		}
		if line != prefix {
			lines = append(lines, line)
		}
		if len(lines) == maxLines {
			return lines
		}
		line = "  " + segment
	}
	if line != prefix && len(lines) < maxLines {
		lines = append(lines, line)
	}
	return lines
}

func visibleWidth(s string) int {
	return ansi.StringWidth(s)
}

func (m *TableModel) interactionStatus() string {
	switch {
	case m.fatal:
		return errorStyle.Render("conexão encerrada · /exit volta ao lobby")
	case m.reconnecting:
		return mutedStyle.Render("reconectando · última mesa preservada · ações de rede pausadas")
	case m.pendingMsg != nil:
		return accentStyle.Render("enviando " + m.actionDescription(m.pendingMsg) + "…")
	default:
		return ""
	}
}

func (m *TableModel) actionDescription(cm *proto.ClientMessage) string {
	if cm == nil {
		return "ação"
	}
	amount := func(value int64) string {
		if m.cfg.RealMoney {
			return fmt.Sprintf("R$ %d", value)
		}
		return fmt.Sprintf("%d", value)
	}
	switch cm.Action {
	case "fold":
		return "desistência"
	case "check":
		return "check"
	case "call":
		return "pagamento de " + amount(cm.Amount)
	case "raise":
		return "aumento para " + amount(cm.Amount)
	default:
		return "ação"
	}
}

func (m *TableModel) refreshInputHint() {
	switch {
	case m.fatal:
		m.input.Placeholder = "/exit para voltar ao lobby"
	case m.reconnecting:
		m.input.Placeholder = "reconectando; comandos locais continuam disponíveis"
	case m.pendingMsg != nil:
		m.input.Placeholder = m.actionDescription(m.pendingMsg) + " em envio"
	default:
		m.input.Placeholder = "/ para ver comandos"
	}
}

func (m *TableModel) refreshCommandMenu() {
	priority := []string{}
	switch {
	case m.fatal:
		priority = []string{"/exit", "/exit!", "/help", "/summary"}
	case m.reconnecting || m.pendingMsg != nil:
		priority = []string{"/help", "/summary", "/last-winners", "/exit"}
	case m.view.IsYourTurn && m.view.Legal != nil:
		for _, action := range m.view.Legal.Actions {
			switch action {
			case "fold", "check", "call", "raise":
				priority = append(priority, "/"+action)
			}
		}
		if hasAction(m.view, "raise") {
			priority = append(priority, "/pot", "/allin")
		}
	default:
		priority = []string{"/peek", "/talk", "/react", "/summary"}
	}
	m.menu.specs = prioritizeCommandSpecs(tableCommandSpecs, priority...)
	m.menu.UpdateInput(m.input.Value())
}
