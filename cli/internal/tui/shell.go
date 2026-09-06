package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"gopkg.aoctech.app/poker/cli/internal/auth"
	"gopkg.aoctech.app/poker/cli/internal/config"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/proto"
	"gopkg.aoctech.app/poker/cli/internal/rest"
	"gopkg.aoctech.app/poker/cli/internal/wsclient"
)

type shellState int

const (
	stateCheckingLogin shellState = iota
	stateLoginChoice
	stateAPIKeyInput
	stateLoggingIn
	stateHome
	stateQuitting
)

// Shell is the CLI's top-level interactive program: a login gate followed by
// a `/command` home REPL, in the spirit of Claude Code's own shell.
type Shell struct {
	cfg     config.Settings
	session *auth.Session
	rc      *rest.Client

	state       shellState
	loginChoice int // 0 = browser, 1 = api key, 2 = sair
	loginErr    error
	input       textinput.Model
	spin        spinner.Model
	lines       []string
	busy        bool

	menu         *commandMenu
	viewport     viewport.Model
	vpReady      bool
	windowWidth  int
	windowHeight int
	followBottom bool // true unless the user scrolled away from the latest output

	pkceFlow   *auth.PKCEFlow
	pkceCancel context.CancelFunc
	loginSeq   int
	copied     bool

	play     playState
	table    *TableModel
	ws       *wsclient.Client
	wsCancel context.CancelFunc
}

type tableExitRecapMsg struct {
	exited  TableExitedMsg
	session rest.Session
	err     error
	attempt int
}

// NewShell wires a Session and REST client from cfg and returns the initial
// (not-yet-checked-login) shell.
func NewShell(cfg config.Settings) *Shell {
	session := auth.NewSession(cfg, http.DefaultClient)
	rc := rest.New(cfg.APIBaseURL, session.Token, http.DefaultClient)
	return newShell(cfg, session, rc)
}

// newShell is the seam tests use to inject a Session/Client pointed at stub
// servers.
func newShell(cfg config.Settings, session *auth.Session, rc *rest.Client) *Shell {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.PromptStyle = promptStyle
	ti.Placeholder = "/ para ver comandos"
	ti.Focus()
	sp := spinner.New()
	sp.Style = accentStyle
	return &Shell{
		cfg: cfg, session: session, rc: rc, state: stateCheckingLogin, input: ti, spin: sp,
		menu: newCommandMenu(homeCommandSpecs), followBottom: true,
	}
}

// appendLine appends line to the scrollback, splitting it first if it
// contains embedded newlines. This split is load-bearing, not cosmetic:
// layoutHeights sizes the viewport from len(s.lines), on the invariant that
// one slice element is exactly one visual line. A caller that appends a
// whole multi-line block (e.g. /help's formatCommandList output) as a
// single element breaks that invariant — the content itself still renders
// correctly (viewport.SetContent rejoins and re-splits everything on \n
// regardless), but the *height budget* is computed from the wrong count,
// undersizing the viewport by however many lines were hidden inside that
// one element. Reproduced live: /help's ~10-line reference collapsed to a
// budget of 1, showing only its last couple of lines. The viewport keeps
// following the bottom on the next render unless the user has scrolled
// away from it.
func (s *Shell) appendLine(line string) {
	if strings.Contains(line, "\n") {
		s.lines = append(s.lines, strings.Split(line, "\n")...)
	} else {
		s.lines = append(s.lines, line)
	}
	s.syncViewport()
}

// syncViewport refreshes the viewport's content only. It deliberately never
// touches scroll position (GotoBottom/AtBottom) — those depend on Height,
// which is only ever correct right after layoutHeights runs in View. Calling
// GotoBottom here used viewport.Height as it was left by whatever render
// happened to run *before* this content arrived (e.g. a smaller height from
// a moment the command menu was open), computing an offset for a viewport
// size that no longer matches — the reproduced bug: a page of new output
// (e.g. /profile's result) rendered as just its last line, the rest blank,
// because the stale offset pointed past the end of the resized viewport.
// followBottom (updated by the scroll keys in handleKey) is what View
// consults to decide whether to re-home to the bottom, always against the
// Height it just set.
func (s *Shell) syncViewport() {
	if !s.vpReady {
		return
	}
	s.viewport.SetContent(strings.Join(s.lines, "\n"))
}

// layoutHeights splits the room available below the logo between the
// scrollback viewport and the command menu, so their combined total plus
// the fixed chrome (logo, input, the busy spinner when shown) can never
// exceed the terminal's actual height — on any window size, including ones
// too small to fit a full menu. The menu wins any contested space (it's
// what the user is actively looking at while typing); the viewport shrinks
// to make room, down to zero rather than forcing an overflow.
func (s *Shell) layoutHeights() (viewportH, menuRows int) {
	header := renderHomeHeader(s.windowWidth)
	chrome := strings.Count(header, "\n") + 1 // responsive header
	chrome++                                  // input
	if s.busy {
		chrome++
	}
	avail := s.windowHeight - chrome
	if avail < 0 {
		avail = 0
	}
	menuRows = s.menu.DesiredRows()
	if menuRows > avail {
		menuRows = avail
	}
	vpBudget := avail - menuRows
	viewportH = len(s.lines)
	if viewportH > vpBudget {
		viewportH = vpBudget
		if !s.followBottom && viewportH > 1 {
			viewportH-- // leave a row for the "scroll for more" hint
		}
	}
	if viewportH < 0 {
		viewportH = 0
	}
	return viewportH, menuRows
}

func (s *Shell) Init() tea.Cmd {
	return tea.Batch(checkLogin(s.session), s.spin.Tick)
}

type checkLoginMsg struct{ loggedIn bool }
type loginResultMsg struct{ err error }
type commandResultMsg struct{ lines []string }

// browserLoginResultMsg carries the outcome of the FinishPKCE wait. seq
// guards against a stale result arriving after the user already cancelled
// (Esc) or started a second attempt — only the result matching the current
// s.loginSeq is applied.
type browserLoginResultMsg struct {
	err error
	seq int
}

// copyResetMsg clears the "Copiado!" flash a couple seconds after it's shown.
type copyResetMsg struct{}

func checkLogin(session *auth.Session) tea.Cmd {
	return func() tea.Msg {
		_, err := session.Token(context.Background())
		return checkLoginMsg{loggedIn: !errors.Is(err, auth.ErrLoggedOut)}
	}
}

// openBrowserCmd fires the browser launch as a bubbletea command, in
// parallel with waitPKCECmd — neither blocks the other.
func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg { _ = auth.OpenBrowser(url); return nil }
}

// waitPKCECmd is the slow half of a browser login: it blocks until the
// loopback callback fires, ctx is cancelled (the user pressed Esc), or the
// flow's own timeout elapses, and reports back tagged with seq. ctx is what
// actually interrupts a blocked Wait — closing the flow's listener alone
// only stops it from accepting new connections, it does not unblock a Wait
// already in progress.
func waitPKCECmd(ctx context.Context, session *auth.Session, flow *auth.PKCEFlow, seq int) tea.Cmd {
	return func() tea.Msg {
		err := session.FinishPKCE(ctx, flow)
		return browserLoginResultMsg{err: err, seq: seq}
	}
}

func loginAPIKey(session *auth.Session, key string) tea.Cmd {
	return func() tea.Msg {
		err := session.LoginAPIKey(context.Background(), key)
		return loginResultMsg{err: err}
	}
}

func (s *Shell) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.windowWidth = msg.Width
		s.windowHeight = msg.Height
		if !s.vpReady {
			s.viewport = viewport.New(msg.Width, 0)
			s.vpReady = true
		} else {
			s.viewport.Width = msg.Width
		}
		s.syncViewport()
		if s.table != nil {
			tm, cmd := s.table.Update(msg)
			s.table = tm.(*TableModel)
			return s, cmd
		}
		return s, nil

	case checkLoginMsg:
		if msg.loggedIn {
			s.state = stateHome
			s.input.Placeholder = "/ para ver comandos"
		} else {
			s.state = stateLoginChoice
		}
		return s, nil

	case loginResultMsg:
		s.busy = false
		if msg.err != nil {
			s.loginErr = msg.err
			s.state = stateLoginChoice
			return s, nil
		}
		s.state = stateHome
		s.input.Reset()
		s.input.Placeholder = "/ para ver comandos"
		s.appendLine(successStyle.Render("Logado."))
		return s, nil

	case browserLoginResultMsg:
		if msg.seq != s.loginSeq {
			return s, nil // stale — cancelled or superseded by a newer attempt
		}
		s.pkceFlow = nil
		if s.pkceCancel != nil {
			s.pkceCancel()
			s.pkceCancel = nil
		}
		if msg.err != nil {
			s.loginErr = msg.err
			s.state = stateLoginChoice
			return s, nil
		}
		s.state = stateHome
		s.input.Reset()
		s.input.Placeholder = "/ para ver comandos"
		s.appendLine(successStyle.Render("Logado."))
		return s, nil

	case copyResetMsg:
		s.copied = false
		return s, nil

	case commandResultMsg:
		s.busy = false
		s.lines = append(s.lines, msg.lines...)
		s.syncViewport()
		return s, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spin, cmd = s.spin.Update(msg)
		return s, cmd

	case stakesLoadedMsg:
		s.busy = false
		if msg.err != nil {
			s.appendLine(explainErr(msg.err))
			s.state = stateHome
			return s, nil
		}
		s.play.stakes = msg.stakes
		s.play.stakeIdx = 0
		s.state = statePlayStake
		return s, nil

	case roomLoadedMsg:
		s.busy = false
		if msg.err != nil {
			s.appendLine(explainErr(msg.err))
			s.state = stateHome
			return s, nil
		}
		s.play.smallBlind = msg.room.SmallBlind
		s.play.bigBlind = msg.room.BigBlind
		s.play.shareCode = msg.room.ShareCode
		s.enterBuyInStep()
		return s, nil

	case joinedMsg:
		if msg.err != nil {
			s.busy = false
			s.appendLine(explainErr(msg.err))
			s.state = stateHome
			return s, nil
		}
		s.play.roomID = msg.roomID
		s.play.shareCode = msg.shareCode
		s.play.smallBlind, s.play.bigBlind = msg.blinds[0], msg.blinds[1]
		s.appendLine("conectando à mesa…")
		return s, connectTable(s.cfg.APIBaseURL, s.session.Token, msg.roomID, msg.shareCode)

	case wsConnectedMsg:
		s.busy = false
		if msg.err != nil {
			s.appendLine("erro ao conectar: " + msg.err.Error())
			s.state = stateHome
			return s, nil
		}
		s.ws = msg.client
		wsCtx, cancel := context.WithCancel(context.Background())
		s.wsCancel = cancel
		s.startTable()
		// Run owns reconnect-with-resync, exactly like the web client — a
		// dropped socket recovers the table instead of dumping to the lobby.
		return s, tea.Batch(pumpTable(s.ws), s.table.Init(), sendReadyCmd(s.ws), runWS(s.ws, wsCtx))

	case wsStreamMsg:
		var cmd tea.Cmd
		if s.table != nil {
			var tm tea.Model
			tm, cmd = s.table.Update(SnapshotMsg{M: msg.m})
			s.table = tm.(*TableModel)
		}
		return s, tea.Batch(cmd, pumpTable(s.ws))

	case ReconnectingMsg, ReconnectedMsg:
		var cmd tea.Cmd
		if s.table != nil {
			tm, tableCmd := s.table.Update(msg)
			s.table = tm.(*TableModel)
			cmd = tableCmd
		}
		return s, tea.Batch(cmd, pumpTable(s.ws))

	case wsClosedMsg:
		// A pump from the table we just left can close after a new table has
		// already connected. Never let that stale event tear down the new socket
		// or add a second generic lobby message after a structured exit recap.
		if msg.client != nil && msg.client != s.ws {
			return s, nil
		}
		unexpected := s.table != nil // a deliberate /exit clears s.table first
		s.leaveTable(nil)
		if msg.err != nil && unexpected {
			s.appendLine("conexão encerrada: " + msg.err.Error())
		}
		return s, nil

	case TableTickMsg:
		if s.table == nil {
			return s, nil
		}
		tm, cmd := s.table.Update(msg)
		s.table = tm.(*TableModel)
		return s, cmd

	case TableExitedMsg:
		s.leaveTable(&msg)
		if msg.SettlementKnown && msg.RoomID != "" && s.rc != nil {
			return s, fetchTableExitRecap(s.rc, msg, 0)
		}
		return s, nil

	case tableExitRecapMsg:
		if msg.err == nil && msg.session.TableID == msg.exited.RoomID && msg.session.EndedAt > 0 {
			name := msg.exited.RoomName
			if name == "" {
				name = msg.exited.RoomID
			}
			for _, line := range formatSessionRecap(name, msg.session, msg.exited.RealMoney) {
				s.appendLine(line)
			}
			return s, nil
		}
		if msg.attempt < 2 {
			return s, fetchTableExitRecap(s.rc, msg.exited, msg.attempt+1)
		}
		s.appendLine(mutedStyle.Render("· resultado líquido ainda indisponível; a banca final acima foi confirmada pela mesa"))
		return s, nil

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
}

// runWS drives the client's reconnect-with-resync loop for the life of the
// table session; it returns (nil msg) only when ctx is cancelled in leaveTable.
func runWS(cl *wsclient.Client, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		cl.Run(ctx)
		return nil
	}
}

func sendReadyCmd(cl *wsclient.Client) tea.Cmd {
	return func() tea.Msg {
		_ = cl.Send(context.Background(), &proto.ClientMessage{Type: "ready", Ready: true})
		return nil
	}
}

func (s *Shell) startTable() {
	s.state = stateInTable
	s.table = NewTableModel(tableConfig(s))
}

func tableConfig(s *Shell) TableConfig {
	return TableConfig{
		RoomID: s.play.roomID, RoomName: s.play.roomID, ShareCode: s.play.shareCode,
		Blinds:   [2]int64{s.play.smallBlind, s.play.bigBlind},
		MaxSeats: maxOr(s.play.seats, 9), CardMode: cardMode(s.cfg),
		YouID: s.youID(),
		Send:  func(m *proto.ClientMessage) error { return s.ws.Send(context.Background(), m) },
		CurrentSession: func(ctx context.Context) (string, error) {
			sess, err := s.rc.CurrentSession(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("sessão: buy-in %d · saldo atual %d · resultado %d",
				sess.BuyinAmount, sess.CashoutAmount, sess.NetPnL), nil
		},
		// ponytail: fixed at the web's default (60min); the CLI has no
		// table-preferences store yet to make this configurable per player.
		RealityCheckEvery: 60 * time.Minute,
		Notes:             s.rc.Notes,
		SaveNote:          s.rc.SaveNote,
	}
}

func (s *Shell) leaveTable(exited *TableExitedMsg) {
	if s.wsCancel != nil {
		s.wsCancel()
		s.wsCancel = nil
	}
	if s.ws != nil {
		_ = s.ws.Close()
		s.ws = nil
	}
	s.table = nil
	s.state = stateHome
	s.input.Reset()
	s.input.Placeholder = "/ para ver comandos"
	if exited == nil {
		s.appendLine("· de volta ao lobby")
		return
	}
	name := exited.RoomName
	if name == "" {
		name = exited.RoomID
	}
	if name == "" {
		name = "atual"
	}
	line := "Você saiu da mesa " + name
	if exited.SettlementKnown {
		line += " · banca final " + formatExitAmount(exited.SettledAmount, exited.RealMoney)
	} else {
		line += " · saída local; liquidação não confirmada"
	}
	if reason := removalReason(exited.Reason); reason != "" {
		line += " · " + reason
	}
	s.appendLine(line)
}

func fetchTableExitRecap(rc *rest.Client, exited TableExitedMsg, attempt int) tea.Cmd {
	return func() tea.Msg {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		session, err := rc.CurrentSession(ctx)
		return tableExitRecapMsg{exited: exited, session: session, err: err, attempt: attempt}
	}
}

// formatSessionRecap renders the on-removal session recap (web SessionRecap):
// time at the table, buy-in, cash-out, net result. joined_at / ended_at are
// epoch-ms. Hands-played and biggest-pot (which the web pulls from hand
// history) are left out — the CLI has no hand-history client yet.
func formatSessionRecap(name string, sess rest.Session, realMoney bool) []string {
	money := func(n int64) string { return formatExitAmount(n, realMoney) }
	out := []string{successStyle.Render("✓ Resumo da sessão") + mutedStyle.Render(" — "+name)}
	if sess.EndedAt > sess.JoinedAt && sess.JoinedAt > 0 {
		out = append(out, "  Tempo na mesa   "+durationLabel(sess.EndedAt-sess.JoinedAt))
	}
	out = append(out,
		"  Entrada         "+money(sess.BuyinAmount),
		"  Retirada        "+money(sess.CashoutAmount),
	)
	result := formatSignedExitAmount(sess.NetPnL, realMoney)
	switch {
	case sess.NetPnL > 0:
		result = successStyle.Render(result)
	case sess.NetPnL < 0:
		result = errorStyle.Render(result)
	}
	return append(out, "  Resultado       "+result)
}

// durationLabel renders an epoch-ms span as "1h 04min" / "12min" / "8s".
func durationLabel(ms int64) string {
	secs := ms / 1000
	switch {
	case secs >= 3600:
		return fmt.Sprintf("%dh %02dmin", secs/3600, (secs%3600)/60)
	case secs >= 60:
		return fmt.Sprintf("%dmin", secs/60)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

func formatExitAmount(amount int64, realMoney bool) string {
	if realMoney {
		return fmt.Sprintf("R$ %d", amount)
	}
	return fmt.Sprintf("%d fichas", amount)
}

func formatSignedExitAmount(amount int64, realMoney bool) string {
	sign := ""
	if amount > 0 {
		sign = "+"
	}
	return sign + formatExitAmount(amount, realMoney)
}

func removalReason(code string) string {
	switch strings.TrimSpace(code) {
	case "exit_requested":
		return "saída concluída"
	case "idle", "idle_timeout":
		return "removido por inatividade"
	case "disconnected":
		return "removido após desconexão"
	case "forced":
		return "saída imediata"
	case "local":
		return ""
	case "":
		return "removido da mesa"
	default:
		return "removido da mesa"
	}
}

// enterBuyInStep moves into the buy-in prompt with the input pre-filled with
// a full 100-BB stack, so a player who just wants to sit down can press
// Enter without typing (and the text-entry field is never blank and
// ambiguous, which is what led someone to type the label "buy-in" into it).
func (s *Shell) enterBuyInStep() {
	s.input.Reset()
	s.input.Placeholder = ""
	s.input.SetValue(strconv.FormatInt(defaultBuyIn(s.play.bigBlind), 10))
	s.input.CursorEnd()
	s.state = statePlayBuyin
}

func maxOr(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func cardMode(cfg config.Settings) game.CardMode {
	if cfg.CardMode == "ascii" {
		return game.CardASCII
	}
	return game.CardColor
}

// youID returns the player id from the stored access token's `sub` claim.
func (s *Shell) youID() string {
	tok, err := s.session.Token(context.Background())
	if err != nil {
		return ""
	}
	return subFromJWT(tok)
}

func (s *Shell) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		// At the table the model owns Ctrl+C: it needs a confirmation press and
		// a clean fold / sit-out / request_exit before leaving (§1.2).
		if s.state == stateInTable && s.table != nil {
			tm, cmd := s.table.Update(msg)
			s.table = tm.(*TableModel)
			return s, cmd
		}
		s.state = stateQuitting
		return s, tea.Quit
	}

	switch s.state {
	case stateLoginChoice:
		switch msg.String() {
		case "up":
			s.loginChoice = (s.loginChoice - 1 + 3) % 3
		case "down":
			s.loginChoice = (s.loginChoice + 1) % 3
		case "enter":
			if s.loginChoice == 0 {
				flow, err := s.session.BeginPKCE()
				if err != nil {
					s.loginErr = err
					return s, nil
				}
				s.pkceFlow = flow
				s.loginErr = nil
				s.copied = false
				s.loginSeq++
				s.state = stateLoggingIn
				ctx, cancel := context.WithCancel(context.Background())
				s.pkceCancel = cancel
				return s, tea.Batch(openBrowserCmd(flow.AuthorizeURL), waitPKCECmd(ctx, s.session, flow, s.loginSeq))
			} else if s.loginChoice == 1 {
				s.state = stateAPIKeyInput
				s.input.Reset()
				s.input.Placeholder = "cole sua API key"
			} else {
				return s, tea.Quit
			}
		}
		return s, nil

	// The browser login is locked: while it's in flight, only 'c' (copy the
	// URL) and Esc (cancel) do anything — every other key is swallowed so
	// the user can't wander off into a different choice mid-flow. Ctrl+C
	// still quits the whole program (handled above, before this switch).
	case stateLoggingIn:
		switch msg.String() {
		case "c":
			if s.pkceFlow != nil {
				_ = clipboardWrite(s.pkceFlow.AuthorizeURL)
				s.copied = true
				return s, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return copyResetMsg{} })
			}
		case "esc":
			if s.pkceCancel != nil {
				s.pkceCancel() // unblocks the in-progress Wait; flow.Cancel() alone does not
				s.pkceCancel = nil
			}
			if s.pkceFlow != nil {
				s.pkceFlow.Cancel()
				s.pkceFlow = nil
			}
			s.loginSeq++ // invalidate any FinishPKCE result still in flight
			s.loginErr = nil
			s.state = stateLoginChoice
		}
		return s, nil

	case stateAPIKeyInput:
		if s.busy {
			return s, nil
		}
		if msg.String() == "enter" {
			key := strings.TrimSpace(s.input.Value())
			if key == "" {
				return s, nil
			}
			s.busy = true
			return s, loginAPIKey(s.session, key)
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd

	case stateHome:
		if s.busy {
			return s, nil
		}
		switch msg.Type {
		case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
			var cmd tea.Cmd
			s.viewport, cmd = s.viewport.Update(msg)
			s.followBottom = s.viewport.AtBottom()
			return s, cmd
		case tea.KeyUp, tea.KeyDown:
			// Arrow keys scroll the scrollback when the command menu isn't
			// steering them — the natural reach for reading long output
			// (e.g. /achievements) without hunting for PgUp.
			if !s.menu.visible {
				var cmd tea.Cmd
				s.viewport, cmd = s.viewport.Update(msg)
				s.followBottom = s.viewport.AtBottom()
				return s, cmd
			}
		case tea.KeyCtrlL:
			s.lines = nil
			s.followBottom = true
			s.syncViewport()
			return s, nil
		}
		if s.menu.visible {
			switch msg.Type {
			case tea.KeyUp:
				s.menu.movePrev()
				return s, nil
			case tea.KeyDown:
				s.menu.moveNext()
				return s, nil
			case tea.KeyTab:
				if val := s.menu.fill(); val != "" {
					s.input.SetValue(val)
					s.input.CursorEnd()
					s.menu.UpdateInput(val)
					s.syncViewport()
				}
				return s, nil
			case tea.KeyEnter:
				val, submit := s.menu.accept()
				if val == "" {
					return s, nil
				}
				s.input.SetValue(val)
				s.input.CursorEnd()
				s.syncViewport()
				if submit {
					return s.submitHomeLine()
				}
				return s, nil
			case tea.KeyEsc:
				s.menu.hide()
				s.syncViewport()
				return s, nil
			}
		}
		if msg.String() == "enter" {
			return s.submitHomeLine()
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		s.menu.UpdateInput(s.input.Value())
		s.syncViewport()
		return s, cmd

	case statePlaySize:
		switch msg.String() {
		case "up":
			s.play.sizeIdx = (s.play.sizeIdx - 1 + len(tableSizes)) % len(tableSizes)
		case "down":
			s.play.sizeIdx = (s.play.sizeIdx + 1) % len(tableSizes)
		case "enter":
			s.play.seats = tableSizes[s.play.sizeIdx].seats
			s.busy = true
			return s, loadStakes(s.rc)
		case "esc":
			s.state = stateHome
		}
		return s, nil

	case statePlayStake:
		switch msg.String() {
		case "up":
			s.play.stakeIdx = (s.play.stakeIdx - 1 + len(s.play.stakes)) % len(s.play.stakes)
		case "down":
			s.play.stakeIdx = (s.play.stakeIdx + 1) % len(s.play.stakes)
		case "enter":
			st := s.play.stakes[s.play.stakeIdx]
			s.play.smallBlind, s.play.bigBlind = st.SmallBlind, st.BigBlind
			s.enterBuyInStep()
		case "esc":
			s.state = stateHome
		}
		return s, nil

	case statePlayBuyin:
		switch msg.String() {
		case "enter":
			amount, err := parseBuyIn(s.input.Value(), s.play.bigBlind)
			if err != nil {
				s.appendLine(errorStyle.Render("· " + err.Error()))
				return s, nil
			}
			s.play.buyIn = amount
			s.play.autoRebuy = false
			s.input.Reset()
			s.state = statePlayAutoRebuy
			return s, nil
		case "esc":
			s.input.Reset()
			s.state = statePlayStake
			if s.play.fromEnter {
				s.state = stateHome
			}
			return s, nil
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd

	case statePlayAutoRebuy:
		switch msg.String() {
		case "up", "down", "left", "right", " ", "tab":
			s.play.autoRebuy = !s.play.autoRebuy
		case "y", "s":
			s.play.autoRebuy = true
		case "n":
			s.play.autoRebuy = false
		case "enter":
			s.busy = true
			s.state = stateJoining
			if s.play.fromEnter {
				return s, joinRoom(s.rc, s.play)
			}
			return s, joinOrCreate(s.rc, s.play)
		case "esc":
			s.state = statePlayBuyin
			s.input.Reset()
			s.input.SetValue(strconv.FormatInt(s.play.buyIn, 10))
			s.input.CursorEnd()
		}
		return s, nil

	case stateInTable:
		if s.table == nil {
			return s, nil
		}
		tm, cmd := s.table.Update(msg)
		s.table = tm.(*TableModel)
		return s, cmd

	default:
		return s, nil
	}
}

// submitHomeLine reads the current input, clears it, echoes it to the
// scrollback, and dispatches it. Shared by plain Enter and the command
// menu's zero-argument auto-submit.
func (s *Shell) submitHomeLine() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(s.input.Value())
	s.input.Reset()
	s.menu.hide()
	if line == "" {
		return s, nil
	}
	s.appendLine(s.input.Prompt + line)
	return s.dispatch(line)
}

// dispatch parses one `/command [args]` home-screen line and returns the
// resulting tea.Cmd. Unknown input is echoed back with a hint.
func (s *Shell) dispatch(line string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(line)
	cmd := fields[0]
	args := fields[1:]

	switch cmd {
	case "/help":
		s.appendLine(formatCommandList(homeCommandSpecs, s.viewport.Width))
		return s, nil
	case "/clear":
		s.lines = nil
		s.syncViewport()
		return s, nil
	case "/logout":
		if err := s.session.Logout(); err != nil {
			s.appendLine(errorStyle.Render("erro: " + err.Error()))
			return s, nil
		}
		s.state = stateLoginChoice
		s.lines = nil
		s.syncViewport()
		return s, nil
	case "/exit", "/quit":
		s.state = stateQuitting
		return s, tea.Quit
	case "/profile":
		s.busy = true
		return s, s.runProfile()
	case "/achievements":
		s.busy = true
		return s, s.runAchievements()
	case "/friends":
		s.busy = true
		return s, s.runFriends()
	case "/requests":
		direction := "incoming"
		if len(args) == 1 && args[0] == "sent" {
			direction = "outgoing"
		} else if len(args) > 0 {
			s.appendLine("uso: /requests [sent]")
			return s, nil
		}
		s.busy = true
		return s, s.runFriendRequests(direction)
	case "/blocked":
		s.busy = true
		return s, s.runBlocked()
	case "/recent":
		s.busy = true
		return s, s.runRecentPlayers()
	case "/inbox":
		s.busy = true
		return s, s.runInbox()
	case "/play":
		s.play = playState{}
		s.play.sizeIdx = 0
		s.state = statePlaySize
		return s, nil
	case "/enter":
		if len(args) != 1 {
			s.appendLine("uso: /enter <room-id> [--code <código>]")
			return s, nil
		}
		s.play = playState{fromEnter: true, roomID: args[0]}
		for i := 1; i+1 < len(args); i++ {
			if args[i] == "--code" {
				s.play.shareCode = args[i+1]
			}
		}
		s.busy = true
		return s, loadRoom(s.rc, args[0])
	default:
		s.appendLine(errorStyle.Render(fmt.Sprintf("comando desconhecido: %s (tente /help)", cmd)))
		return s, nil
	}
}

func (s *Shell) runProfile() tea.Cmd {
	return func() tea.Msg {
		p, err := s.rc.Me(context.Background())
		if err != nil {
			return commandResultMsg{lines: []string{explainErr(err)}}
		}
		return commandResultMsg{lines: strings.Split(FormatProfile(p), "\n")}
	}
}

func (s *Shell) runAchievements() tea.Cmd {
	return func() tea.Msg {
		a, err := s.rc.Achievements(context.Background())
		if err != nil {
			return commandResultMsg{lines: []string{explainErr(err)}}
		}
		return commandResultMsg{lines: strings.Split(FormatAchievements(a), "\n")}
	}
}

func (s *Shell) runFriends() tea.Cmd {
	return func() tea.Msg {
		players, err := s.rc.Friends(context.Background())
		if err != nil {
			return commandResultMsg{lines: []string{explainErr(err)}}
		}
		return commandResultMsg{lines: strings.Split(FormatSocialPlayers("Amigos", players), "\n")}
	}
}

func (s *Shell) runFriendRequests(direction string) tea.Cmd {
	title := "Pedidos de amizade recebidos"
	if direction == "outgoing" {
		title = "Pedidos de amizade enviados"
	}
	return func() tea.Msg {
		players, err := s.rc.FriendRequests(context.Background(), direction)
		if err != nil {
			return commandResultMsg{lines: []string{explainErr(err)}}
		}
		return commandResultMsg{lines: strings.Split(FormatSocialPlayers(title, players), "\n")}
	}
}

func (s *Shell) runBlocked() tea.Cmd {
	return func() tea.Msg {
		players, err := s.rc.Blocked(context.Background())
		if err != nil {
			return commandResultMsg{lines: []string{explainErr(err)}}
		}
		return commandResultMsg{lines: strings.Split(FormatSocialPlayers("Bloqueados", players), "\n")}
	}
}

func (s *Shell) runRecentPlayers() tea.Cmd {
	return func() tea.Msg {
		players, err := s.rc.RecentPlayers(context.Background())
		if err != nil {
			return commandResultMsg{lines: []string{explainErr(err)}}
		}
		return commandResultMsg{lines: strings.Split(FormatSocialPlayers("Jogadores recentes", players), "\n")}
	}
}

func (s *Shell) runInbox() tea.Cmd {
	return func() tea.Msg {
		events, err := s.rc.Inbox(context.Background())
		if err != nil {
			return commandResultMsg{lines: []string{explainErr(err)}}
		}
		return commandResultMsg{lines: strings.Split(FormatInbox(events), "\n")}
	}
}

func explainErr(err error) string {
	if errors.Is(err, auth.ErrLoggedOut) {
		return errorStyle.Render("não logado — use /logout e faça login de novo")
	}
	if rest.IsStatus(err, http.StatusForbidden) {
		return errorStyle.Render("erro: " + err.Error() + " (este ambiente pode não ter habilitado o client poker-cli — ver docs/specs/2026-09-05-poker-cli.md §2)")
	}
	return errorStyle.Render("erro: " + err.Error())
}

func (s *Shell) View() string {
	switch s.state {
	case stateQuitting:
		return ""
	case stateLoginChoice:
		cursor := func(i int) string {
			if i == s.loginChoice {
				return selectedStyle.Render("› ")
			}
			return "  "
		}
		errLine := ""
		if s.loginErr != nil {
			errLine = "\n" + errorStyle.Render("erro no login: "+s.loginErr.Error())
		}
		return fmt.Sprintf("%s\n\nComo você quer entrar?\n%sAbrir navegador\n%sUsar API key\n%sSair%s\n",
			renderHomeHeader(s.windowWidth), cursor(0), cursor(1), cursor(2), errLine)
	case stateAPIKeyInput:
		view := renderHomeHeader(s.windowWidth) + "\n\n" + mutedStyle.Render("Cole sua API key para continuar") + "\n" + s.input.View()
		if s.busy {
			view += "\n" + accentStyle.Render(s.spin.View()) + " verificando..."
		}
		return view
	case stateLoggingIn:
		var b strings.Builder
		b.WriteString(renderHomeHeader(s.windowWidth))
		b.WriteString("\n\n")
		b.WriteString(accentStyle.Render(s.spin.View()))
		b.WriteString(" aguardando confirmação no navegador...\n")
		if s.pkceFlow != nil {
			b.WriteString("\n")
			b.WriteString(mutedStyle.Render("Se não abrir automaticamente, copie e cole a seguinte URL no seu navegador:"))
			b.WriteString("\n")
			b.WriteString(accentStyle.Render(s.pkceFlow.AuthorizeURL))
			b.WriteString("\n\n")
			if s.copied {
				b.WriteString(successStyle.Render("Copiado!"))
			} else {
				b.WriteString(mutedStyle.Render("Pressione C para copiar · Esc para cancelar"))
			}
			b.WriteString("\n")
		}
		return b.String()
	case statePlaySize:
		var b strings.Builder
		b.WriteString(titleStyle.Render("Tamanho da mesa:") + "\n")
		for i, sz := range tableSizes {
			cur := "  "
			if i == s.play.sizeIdx {
				cur = selectedStyle.Render("› ")
			}
			b.WriteString(cur + sz.label + "\n")
		}
		return b.String()
	case statePlayStake:
		var b strings.Builder
		b.WriteString(titleStyle.Render("Stake (small/big blind):") + "\n")
		for i, st := range s.play.stakes {
			cur := "  "
			if i == s.play.stakeIdx {
				cur = selectedStyle.Render("› ")
			}
			b.WriteString(fmt.Sprintf("%s%d / %d\n", cur, st.SmallBlind, st.BigBlind))
		}
		return b.String()
	case statePlayBuyin:
		return fmt.Sprintf("%s\n%s\n%s",
			titleStyle.Render("Buy-in"),
			mutedStyle.Render(buyInHint(s.play.bigBlind)),
			s.input.View()) + "\n" + dimStyle.Render("Enter confirma · Esc volta")
	case statePlayAutoRebuy:
		opt := func(on bool, label string) string {
			if s.play.autoRebuy == on {
				return selectedStyle.Render("› " + label)
			}
			return "  " + label
		}
		return titleStyle.Render("Rebuy automático") + "\n" +
			mutedStyle.Render("Recompra fichas até o buy-in sozinho quando seu stack zerar.") + "\n" +
			opt(false, "Não") + "\n" + opt(true, "Sim") + "\n" +
			dimStyle.Render("↑↓ ou espaço alterna · Enter confirma · Esc volta")
	case stateJoining:
		return accentStyle.Render(s.spin.View()) + " entrando na mesa..."
	case stateInTable:
		if s.table != nil {
			return s.table.View()
		}
		return ""
	default: // stateHome, stateCheckingLogin
		if s.state == stateCheckingLogin {
			return renderHomeHeader(s.windowWidth) + "\n" + accentStyle.Render(s.spin.View()) + " verificando sessão..."
		}
		lines := []string{renderHomeHeader(s.windowWidth)}
		if s.vpReady {
			vpH, menuRows := s.layoutHeights()
			s.viewport.Height = vpH
			if s.followBottom {
				s.viewport.GotoBottom()
			}
			if vpH > 0 {
				lines = append(lines, s.viewport.View())
				if !s.viewport.AtBottom() {
					lines = append(lines, dimStyle.Render("↓ ↑↓/PgUp/PgDn rolam · End volta ao fim"))
				}
			}
			if s.busy {
				lines = append(lines, accentStyle.Render(s.spin.View()))
			}
			lines = append(lines, s.input.View())
			if menuRows > 0 {
				if menuView := s.menu.View(menuRows, s.viewport.Width); menuView != "" {
					lines = append(lines, menuView)
				}
			}
		} else {
			// No WindowSizeMsg yet — render unconstrained rather than guess a size.
			lines = append(lines, strings.Join(s.lines, "\n"))
			if s.busy {
				lines = append(lines, accentStyle.Render(s.spin.View()))
			}
			lines = append(lines, s.input.View())
			if menuView := s.menu.View(maxMenuRows+1, 0); menuView != "" {
				lines = append(lines, menuView)
			}
		}
		return padViewHeight(strings.Join(lines, "\n"), s.windowHeight)
	}
}
