package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/proto"
	"gopkg.aoctech.app/poker/cli/internal/rest"
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
	// RealityCheckEvery is how often the responsible-gaming reminder (web
	// RealityCheck.tsx) fires while seated; <= 0 disables it.
	RealityCheckEvery time.Duration
	// Notes/SaveNote back /note and /player (nil disables both — same
	// pattern as CurrentSession).
	Notes    func(ctx context.Context, opponentIDs []string) ([]rest.PlayerNote, error)
	SaveNote func(ctx context.Context, opponentID, tag, text string) (rest.PlayerNote, error)
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
	notesLoadedMsg struct {
		notes []rest.PlayerNote
		err   error
	}
	noteSavedMsg struct {
		playerID string
		name     string
		note     rest.PlayerNote
		err      error
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

	// Per-hand local peek state (§1.1). Both cards render face-down until the
	// viewer peeks them; strength/equity stay hidden until both are peeked.
	// Reset when hand_id changes; forced revealed once the hand completes.
	peek               [2]bool
	peekHandID         string
	peekBreadcrumbSent bool

	// Ctrl+C at the table needs a confirmation press (§1.2).
	ctrlCAt time.Time

	// RealityCheck (web parity): joinedAt marks the first snapshot, handsSeen
	// counts distinct completed hand ids, realityShown is the last interval
	// boundary already announced (never re-announced, and skipped — not
	// dropped — while it's the viewer's turn, same as the web gate).
	joinedAt     time.Time
	handsSeen    map[string]bool
	realityShown int

	// Private per-opponent notes (§3 PlayerNoteDialog). notes is keyed by
	// opponent id, absent = no note; notesRequested tracks ids already asked
	// for (success or failure) so a missing-scope 403 isn't retried every
	// snapshot.
	notes          map[string]rest.PlayerNote
	notesRequested map[string]bool

	// Chat history pane (§3 Chat.tsx): chat is kept separately from the
	// narration log so /chat can review just the conversation — the log
	// mixes it in with every hand event, easy to miss. chatUnread counts
	// messages since the last /chat view (the web's unread badge).
	chat       []chatEntry
	chatUnread int
}

// chatEntry is one message in the table's chat history.
type chatEntry struct {
	name string
	text string
	at   time.Time
}

// NewTableModel builds the table view for cfg.
func NewTableModel(cfg TableConfig) *TableModel {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = promptStyle
	ti.Placeholder = "/ para ver comandos"
	ti.Focus()
	m := &TableModel{
		cfg: cfg, narr: game.NewNarrator(cfg.YouID).WithCardMode(cfg.CardMode),
		input: ti, menu: newCommandMenu(tableCommandSpecs),
		now: time.Now(), followBottom: true, handsSeen: map[string]bool{},
		notes: map[string]rest.PlayerNote{}, notesRequested: map[string]bool{},
	}
	m.menu.argFn = m.reactCompletions
	return m
}

// reactCompletions drives argument-level autocomplete for `/react`: first the
// reaction code, then — for a targeted reaction — the opponent to throw it at.
// prefix is the input text to keep verbatim ahead of the completed token.
func (m *TableModel) reactCompletions(value string) (choices []commandSpec, prefix string) {
	fields := strings.SplitN(value, " ", 3)
	switch fields[0] {
	case "/react":
	case "/note", "/player":
		return m.playerTargetCompletions(fields)
	default:
		return nil, ""
	}
	if len(fields) < 2 {
		return nil, ""
	}
	if len(fields) == 2 { // completing the reaction code
		partial := strings.ToLower(fields[1])
		for _, r := range game.TableReactions {
			if !strings.HasPrefix(r.ID, partial) {
				continue
			}
			args, desc := "", r.Label
			if r.Targeted {
				args, desc = "<jogador>", r.Label+" (mira alguém)"
			}
			choices = append(choices, commandSpec{Name: r.ID, Args: args, Desc: desc})
		}
		return choices, "/react "
	}
	r, ok := game.LookupReaction(fields[1]) // completing the target player
	if !ok || !r.Targeted {
		return nil, ""
	}
	partial := strings.ToLower(fields[2])
	for _, p := range m.view.Players {
		if p.IsYou || !strings.HasPrefix(strings.ToLower(p.Name), partial) {
			continue
		}
		choices = append(choices, commandSpec{Name: p.Name, Desc: p.Position})
	}
	return choices, "/react " + fields[1] + " "
}

// playerTargetCompletions completes /note and /player's first (and only,
// besides /note's free-form note text) argument: the opponent's name.
func (m *TableModel) playerTargetCompletions(fields []string) (choices []commandSpec, prefix string) {
	if len(fields) != 2 {
		return nil, "" // /note's later args are free-form note text, not completed
	}
	partial := strings.ToLower(fields[1])
	for _, p := range m.view.Players {
		if p.IsYou || !strings.HasPrefix(strings.ToLower(p.Name), partial) {
			continue
		}
		choices = append(choices, commandSpec{Name: p.Name, Desc: p.Position})
	}
	return choices, fields[0] + " "
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
		return m, tea.Batch(tableTick(), m.maybeRealityCheck())

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

	case notesLoadedMsg:
		// Silent on failure (missing scope, network hiccup) — same as the web's
		// silentError: true, a private-note lookup must never interrupt the game.
		for _, n := range msg.notes {
			m.notes[n.OpponentID] = n
		}
		return m, nil

	case noteSavedMsg:
		if msg.err != nil {
			m.appendLog(errorStyle.Render("· não deu para salvar a anotação: " + msg.err.Error()))
			return m, nil
		}
		if msg.note.Tag == "" && msg.note.Text == "" {
			delete(m.notes, msg.playerID)
			m.appendLog(successStyle.Render("✓ anotação sobre " + msg.name + " removida"))
		} else {
			msg.note.OpponentID = msg.playerID
			m.notes[msg.playerID] = msg.note
			m.appendLog(successStyle.Render("✓ anotação sobre " + msg.name + " salva"))
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
		if m.joinedAt.IsZero() {
			m.joinedAt = m.now
		}
		if strings.EqualFold(sm.Snapshot.Stage, "complete") && sm.Snapshot.HandId != "" {
			m.handsSeen[sm.Snapshot.HandId] = true
		}
		m.view = game.NewTableView(sm.Snapshot, m.cfg.YouID, m.cfg.RoomName, m.cfg.RealMoney, m.cfg.Blinds, m.cfg.MaxSeats, m.cfg.CardMode)
		m.haveView = true
		m.reconcilePeek()
		m.refreshCommandMenu()
		m.refreshInputHint()
		return m.fetchMissingNotes()

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

	case "chat":
		m.recordChat(sm)
		if lines := m.narr.OnMessage(sm); len(lines) > 0 {
			m.appendLog(strings.Join(lines, "\n"))
		}
		return nil

	case "reaction", "achievement_unlocked":
		if lines := m.narr.OnMessage(sm); len(lines) > 0 {
			m.appendLog(strings.Join(lines, "\n"))
		}
		return nil
	}
	return nil
}

func (m *TableModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m.handleCtrlC()
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

	// /note and /player are REST-backed, not table-socket ClientMessages —
	// handled directly rather than threading a whole extra return value
	// through ParseTableCommand for two commands.
	if fields := strings.Fields(line); len(fields) > 0 {
		switch fields[0] {
		case "/note":
			return m, m.handleNoteCommand(fields[1:])
		case "/player":
			return m, m.handlePlayerCommand(fields[1:])
		}
	}

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

// handleCtrlC guards the table exit: outside a live seat it quits at once;
// seated, the first press warns and arms a 3s window, the second within it
// folds (if it's your turn) or sits you out, asks the server to remove you,
// then quits.
func (m *TableModel) handleCtrlC() (tea.Model, tea.Cmd) {
	if m.view.You.ID == "" || m.fatal || m.exitRequested {
		m.quit = true
		return m, exitCmd(m.exitDetails("local", "atalho Ctrl+C", 0, false))
	}
	if !m.ctrlCAt.IsZero() && time.Since(m.ctrlCAt) < 3*time.Second {
		send := func(cm *proto.ClientMessage) {
			if m.cfg.Send != nil {
				_ = m.cfg.Send(cm)
			}
		}
		if m.view.IsYourTurn && hasAction(m.view, "fold") {
			send(act("fold", 0, m.view))
			m.appendLog("· fold enviado ao sair")
		} else {
			send(&proto.ClientMessage{Type: "ready", Ready: false})
			m.appendLog("· sit-out enviado ao sair")
		}
		send(&proto.ClientMessage{Type: "request_exit", ActionId: uuid.NewString()})
		m.quit = true
		return m, exitCmd(m.exitDetails("local", "saída via Ctrl+C", 0, false))
	}
	m.ctrlCAt = time.Now()
	m.appendLog(mutedStyle.Render("· aperte Ctrl+C de novo em 3s para sair — você vai dar fold / sit-out"))
	return m, nil
}

// reconcilePeek resets per-hand peek state on a new hand and force-reveals
// both cards once the hand completes.
func (m *TableModel) reconcilePeek() {
	if m.view.HandID != m.peekHandID {
		m.peekHandID = m.view.HandID
		m.peek = [2]bool{}
		m.peekBreadcrumbSent = false
	}
	if strings.EqualFold(m.view.Stage, "complete") {
		m.peek = [2]bool{true, true}
	}
}

// maybeRealityCheck fires the periodic responsible-gaming reminder (web
// RealityCheck.tsx): every cfg.RealityCheckEvery of table time, at most once
// per interval boundary, never while it's the viewer's turn (checked again
// next tick rather than dropped). Prints a neutral summary instead of the
// web's modal — this is a terminal, nothing else demands attention over it.
func (m *TableModel) maybeRealityCheck() tea.Cmd {
	if m.cfg.RealityCheckEvery <= 0 || m.joinedAt.IsZero() || m.view.IsYourTurn {
		return nil
	}
	elapsed := m.now.Sub(m.joinedAt)
	due := int(elapsed / m.cfg.RealityCheckEvery)
	if due <= m.realityShown {
		return nil
	}
	m.realityShown = due
	m.appendLog(dimStyle.Render("── Pausa consciente ──"))
	m.appendLog(fmt.Sprintf("  Tempo na mesa   %s", durationLabel(elapsed.Milliseconds())))
	m.appendLog(fmt.Sprintf("  Mãos concluídas %d", len(m.handsSeen)))
	m.appendLog(mutedStyle.Render("  · não interrompe nada — continue quando quiser"))
	if m.cfg.CurrentSession == nil {
		return nil
	}
	return func() tea.Msg {
		text, err := m.cfg.CurrentSession(context.Background())
		return sessionSummaryMsg{text: text, err: err}
	}
}

// fetchMissingNotes requests notes for every seated opponent this model
// hasn't already asked about (whether or not that ask succeeded — a scope
// gap 403s every time, and retrying it every snapshot would just spam the
// same request forever).
func (m *TableModel) fetchMissingNotes() tea.Cmd {
	if m.cfg.Notes == nil {
		return nil
	}
	var missing []string
	for _, p := range m.view.Players {
		if p.IsYou || m.notesRequested[p.ID] {
			continue
		}
		m.notesRequested[p.ID] = true
		missing = append(missing, p.ID)
	}
	if len(missing) == 0 {
		return nil
	}
	return func() tea.Msg {
		notes, err := m.cfg.Notes(context.Background(), missing)
		return notesLoadedMsg{notes: notes, err: err}
	}
}

// noteTagColors maps a PlayerNote tag to the seat-row marker color
// (playernotes.PLAYER_NOTE_TAGS, mirrored — ui/src/lib/api/playerNotes.ts).
var noteTagColors = map[string]lipgloss.Color{
	"red": "#ef4444", "orange": "#f97316", "yellow": "#eab308",
	"green": "#22c55e", "blue": "#3b82f6", "purple": "#a855f7",
}

// noteTagDot renders the seat-row marker for a note's color tag, or "" for
// no tag / an unrecognized one.
func noteTagDot(tag string) string {
	color, ok := noteTagColors[tag]
	if !ok {
		return ""
	}
	return lipgloss.NewStyle().Foreground(color).Render("●")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// handleNoteCommand implements /note <jogador> [tag <cor>|clear|<texto>...]
// (web PlayerNoteDialog): no extra args shows the cached note, otherwise
// saves (tag/clear/free text) via cfg.SaveNote.
func (m *TableModel) handleNoteCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		m.appendLog(errorStyle.Render("uso: /note <jogador> [tag <cor>|clear|<texto>]"))
		return nil
	}
	id, err := resolveTarget(m.view, args[0])
	if err != nil {
		m.appendLog(errorStyle.Render("· " + err.Error()))
		return nil
	}
	name := args[0]
	for _, p := range m.view.Players {
		if p.ID == id {
			name = p.Name
		}
	}
	existing := m.notes[id]

	if len(args) == 1 {
		if existing.Tag == "" && existing.Text == "" {
			m.appendLog(mutedStyle.Render("· sem anotação sobre " + name))
		} else {
			m.appendLog(fmt.Sprintf("· nota sobre %s [%s]: %s", name, orDash(existing.Tag), orDash(existing.Text)))
		}
		return nil
	}

	tag, text := existing.Tag, existing.Text
	switch args[1] {
	case "tag":
		if len(args) != 3 {
			m.appendLog(errorStyle.Render("uso: /note <jogador> tag <" + strings.Join(rest.PlayerNoteTags, "|") + ">"))
			return nil
		}
		if _, ok := noteTagColors[args[2]]; !ok {
			m.appendLog(errorStyle.Render("cor inválida: " + args[2] + " (opções: " + strings.Join(rest.PlayerNoteTags, ", ") + ")"))
			return nil
		}
		tag = args[2]
	case "clear":
		tag, text = "", ""
	default:
		text = strings.Join(args[1:], " ")
		if len(text) > 500 {
			m.appendLog(errorStyle.Render("· nota muito longa (máx 500)"))
			return nil
		}
	}
	if m.cfg.SaveNote == nil {
		m.appendLog(errorStyle.Render("· anotações indisponíveis nesta sessão"))
		return nil
	}
	return func() tea.Msg {
		note, err := m.cfg.SaveNote(context.Background(), id, tag, text)
		return noteSavedMsg{playerID: id, name: name, note: note, err: err}
	}
}

// handlePlayerCommand implements /player <jogador> — a text-mode stand-in for
// the web's per-seat action menu (profile/note/react at a glance). Friend/
// mute/block/report have no CLI-reachable endpoint yet (§4 social gap); this
// surfaces what the CLI actually can do for that seat today.
func (m *TableModel) handlePlayerCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		m.appendLog(errorStyle.Render("uso: /player <jogador>"))
		return nil
	}
	id, err := resolveTarget(m.view, strings.Join(args, " "))
	if err != nil {
		m.appendLog(errorStyle.Render("· " + err.Error()))
		return nil
	}
	var p game.PlayerView
	for _, pl := range m.view.Players {
		if pl.ID == id {
			p = pl
		}
	}
	stack := fmt.Sprintf("%d", p.Stack)
	if m.view.RealMoney {
		stack = fmt.Sprintf("R$ %d", p.Stack)
	}
	m.appendLog(fmt.Sprintf("· %s — %s · stack %s", p.Name, orDash(p.Position), stack))
	if note := m.notes[id]; note.Tag != "" || note.Text != "" {
		m.appendLog(fmt.Sprintf("  nota [%s]: %s", orDash(note.Tag), orDash(note.Text)))
	} else {
		m.appendLog(mutedStyle.Render("  sem anotação — /note " + p.Name + " <texto>"))
	}
	m.appendLog(mutedStyle.Render("  reagir: /react <código> " + p.Name))
	return nil
}

// applyPeek toggles local reveal state. idx < 0 toggles both cards
// independently. On the first reveal of a hand it fires the one-shot
// peek_cards achievement breadcrumb (best-effort).
func (m *TableModel) applyPeek(idx int) tea.Cmd {
	revealed := false
	if idx < 0 {
		for i := range m.peek {
			m.peek[i] = !m.peek[i]
			revealed = revealed || m.peek[i]
		}
	} else {
		m.peek[idx] = !m.peek[idx]
		revealed = m.peek[idx]
	}
	if !revealed || m.peekBreadcrumbSent || m.view.HandID == "" || len(m.view.YourHole) != 2 {
		return nil
	}
	if m.reconnecting || m.fatal || m.cfg.Send == nil {
		return nil
	}
	m.peekBreadcrumbSent = true
	cm := &proto.ClientMessage{Type: "peek_cards"}
	if idx >= 0 {
		i := int32(idx)
		cm.CardIndex = &i
	}
	return m.send(cm)
}

// handSummary renders the viewer's hole cards, face-down (██) until peeked;
// strength and equity appear only once both cards are peeked (§1.1).
func (m *TableModel) handSummary() string {
	v := m.view
	if len(v.YourHole) != 2 {
		if len(v.YourHole) > 0 {
			return "Sua mão: " + game.FormatCards(v.YourHole, m.cfg.CardMode)
		}
		return "Sua mão: —"
	}
	if !(m.peek[0] && m.peek[1]) {
		card := func(i int) string {
			if m.peek[i] {
				return game.FormatCard(v.YourHole[i], m.cfg.CardMode)
			}
			return "██"
		}
		return "Sua mão: " + card(0) + " " + card(1) + " " + mutedStyle.Render("(/peek para ver)")
	}
	s := "Sua mão: " + game.FormatCards(v.YourHole, m.cfg.CardMode)
	if v.YourStrength != "" {
		s += " · " + v.YourStrength
	}
	if v.YourEquity >= 0 {
		s += fmt.Sprintf(" · ~%.0f%%", v.YourEquity*100)
	}
	return s
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
	case ActChatHistory:
		const maxShown = 20
		m.chatUnread = 0
		if len(m.chat) == 0 {
			m.appendLog(mutedStyle.Render("· sem mensagens no chat ainda"))
			break
		}
		m.appendLog(mutedStyle.Render("── Chat ──"))
		start := max(0, len(m.chat)-maxShown)
		for _, c := range m.chat[start:] {
			m.appendLog(fmt.Sprintf("  %s %s: %s", c.at.Format("15:04"), c.name, c.text))
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
	case ActPeekBoth:
		return m.applyPeek(-1)
	case ActPeekCard1:
		return m.applyPeek(0)
	case ActPeekCard2:
		return m.applyPeek(1)
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
	// Board and hole cards each get their own line so they are easy to pick
	// out (§5); the pot rides along on the board line.
	boardLine := fmt.Sprintf("Board  %s · Pote %s", board, money(v.Pot))
	hand := m.handSummary()

	var lines []string
	switch {
	case width >= 72:
		lines = append(lines,
			fmt.Sprintf("Mesa %s · No-Limit Hold'em · %s · blinds %s/%s · %d/%d",
				titleStyle.Render(name), stage, money(v.SmallBlind), money(v.BigBlind), v.Seated, v.MaxSeats),
			boardLine,
			hand,
		)
		// The per-seat table costs one line per seat; on a short terminal fall
		// back to the packed two-line list so the log doesn't vanish.
		if m.windowHeight > 0 && m.windowHeight < 20 {
			lines = append(lines, packHeaderSegments("Jogadores: ", m.playerSegments(money, false), width, 2)...)
		} else {
			lines = append(lines, m.playerRows(money)...)
		}
	case width >= 48:
		lines = append(lines,
			fmt.Sprintf("Mesa %s · %s · blinds %s/%s", titleStyle.Render(name), stage, money(v.SmallBlind), money(v.BigBlind)),
			boardLine,
			hand,
		)
		lines = append(lines, packHeaderSegments("Em foco: ", m.playerSegments(money, true), width, 1)...)
	default:
		lines = append(lines,
			fmt.Sprintf("%s · %s", titleStyle.Render(name), stage),
			boardLine,
			hand,
		)
	}

	lines = append(lines, m.actorLine(money))
	if warn := m.idleWarningLine(); warn != "" {
		lines = append(lines, warn)
	}
	if badge := m.chatBadgeLine(); badge != "" {
		lines = append(lines, badge)
	}
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

// playerRows renders the seat list as an aligned table (§5): one row per
// seat, position / name / stack in fixed columns, an "aposta N" or status
// note trailing. Actor row is marked "▶" and coloured; the viewer's row is
// bold; folded / sitting-out rows are dimmed.
func (m *TableModel) playerRows(money func(int64) string) []string {
	if len(m.view.Players) == 0 {
		return []string{mutedStyle.Render("Jogadores: aguardando")}
	}
	type row struct {
		marker, pos, name, stack, note string
		you, actor, dim                bool
	}
	rows := make([]row, 0, len(m.view.Players))
	var posW, nameW, stackW int
	for _, p := range m.view.Players {
		r := row{
			pos: p.Position, name: p.Name, stack: money(p.Stack),
			you:   p.IsYou,
			actor: p.ID == m.view.CurrentPlayer.ID && m.view.CurrentPlayer.ID != "",
			dim:   p.Folded || p.SittingOut,
		}
		if r.pos == "" {
			r.pos = "—"
		}
		if p.IsYou {
			r.name = "VOCÊ"
		}
		switch {
		case p.Folded:
			r.note = "desistiu"
		case p.SittingOut:
			r.note = "fora"
		case p.Committed > 0:
			r.note = "aposta " + money(p.Committed)
		}
		// The note-tag dot is appended to the trailing note text, not the
		// fixed-width name column: it carries its own ANSI color escapes,
		// and only the columns built through the row's Sprintf below need
		// display-width-safe padding — the note suffix is concatenated
		// after that, unformatted, so raw ANSI here is safe.
		if tag := m.notes[p.ID].Tag; !p.IsYou && tag != "" {
			dot := noteTagDot(tag)
			if r.note != "" {
				dot += " "
			}
			r.note = dot + r.note
		}
		r.marker = " "
		if r.actor {
			r.marker = "▶"
		}
		rows = append(rows, r)
		posW = max(posW, len(r.pos))
		nameW = max(nameW, ansi.StringWidth(r.name))
		stackW = max(stackW, len(r.stack))
	}
	out := make([]string, 0, len(rows)+1)
	out = append(out, mutedStyle.Render("Jogadores"))
	for _, r := range rows {
		line := fmt.Sprintf("  %s %-*s  %-*s  %*s", r.marker, posW, r.pos, nameW, r.name, stackW, r.stack)
		if r.note != "" {
			line += "  " + r.note
		}
		switch {
		case r.you:
			line = accentStyle.Bold(true).Render(line)
		case r.actor:
			line = successStyle.Render(line)
		case r.dim:
			line = dimStyle.Render(line)
		}
		out = append(out, line)
	}
	return out
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

// turnClock renders the current actor's decision countdown, splitting the
// base room clock from the time bank the way the web PerimeterTimer does.
// Applies to whoever is on the clock — the viewer or an opponent. Returns ""
// when the snapshot carries no deadline, and "esgotado" past it.
func (m *TableModel) turnClock() string {
	dl := m.view.ActionDeadlineMS
	if dl <= 0 {
		return ""
	}
	now := m.now.UnixMilli()
	remain := dl - now
	if remain <= 0 {
		return "esgotado"
	}
	total := ceilSecs(remain)
	base := m.view.BaseDeadlineMS
	if base <= 0 || base >= dl {
		return fmt.Sprintf("%ds", total)
	}
	bankSecs := (dl - base) / 1000
	if now >= base {
		return fmt.Sprintf("%ds de banco", total) // base clock spent, into the reserve
	}
	return fmt.Sprintf("%ds (+%ds banco)", ceilSecs(base-now), bankSecs)
}

func ceilSecs(ms int64) int64 {
	if ms <= 0 {
		return 0
	}
	return (ms + 999) / 1000
}

func (m *TableModel) actorLine(money func(int64) string) string {
	p := m.view.CurrentPlayer
	if p.ID == "" {
		return mutedStyle.Render("Aguardando a próxima mão")
	}
	clock := m.turnClock()
	if p.IsYou {
		switch clock {
		case "":
			return successStyle.Bold(true).Render("● SUA VEZ")
		case "esgotado":
			return mutedStyle.Render("Tempo esgotado · aguardando a mesa")
		default:
			return successStyle.Bold(true).Render("● SUA VEZ · " + clock)
		}
	}
	name := p.Name
	if name == "" {
		name = "outro jogador"
	}
	line := fmt.Sprintf("Vez de %s · stack %s", name, money(p.Stack))
	if clock != "" && clock != "esgotado" {
		line += " · " + clock
	} else if clock == "esgotado" {
		line += " · tempo esgotado"
	}
	return line
}

// idleWarningLine surfaces the server's pending idle removal (the web
// IdleWarning). Shown for the viewer and for opponents alike.
func (m *TableModel) idleWarningLine() string {
	if m.view.IdleRemovalMS <= 0 {
		return ""
	}
	secs := ceilSecs(m.view.IdleRemovalMS - m.now.UnixMilli())
	p := m.view.CurrentPlayer
	if p.IsYou {
		return errorStyle.Render(fmt.Sprintf("⚠ você sai por inatividade em %ds — aja ou /keep", secs))
	}
	who := "o jogador da vez"
	if p.Name != "" {
		who = p.Name
	}
	return mutedStyle.Render(fmt.Sprintf("⚠ %s sai por inatividade em %ds", who, secs))
}

// recordChat appends one server chat message to the chat history (§3
// Chat.tsx) and, for a message from someone else, bumps the unread count.
func (m *TableModel) recordChat(sm *proto.ServerMessage) {
	if sm.Message == "" {
		return
	}
	m.chat = append(m.chat, chatEntry{name: m.playerName(sm.PlayerId), text: sm.Message, at: m.now})
	if sm.PlayerId != m.cfg.YouID {
		m.chatUnread++
	}
}

// playerName resolves a player id to its seat name for display, falling back
// to a generic label rather than the raw id once a seat hasn't loaded yet.
func (m *TableModel) playerName(id string) string {
	if id == m.cfg.YouID {
		return "você"
	}
	for _, p := range m.view.Players {
		if p.ID == id {
			return p.Name
		}
	}
	return "jogador"
}

// chatBadgeLine surfaces the chat history's unread count (the web's Chat.tsx
// badge) — chat is diluted into the narration log alongside every hand
// event, easy to miss, so this is shown independent of scroll position.
func (m *TableModel) chatBadgeLine() string {
	if m.chatUnread == 0 {
		return ""
	}
	return mutedStyle.Render(fmt.Sprintf("💬 %d mensagem(ns) nova(s) — /chat para ver", m.chatUnread))
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
