package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	windowHeight int
	followBottom bool // true unless the user scrolled away from the latest output

	pkceFlow   *auth.PKCEFlow
	pkceCancel context.CancelFunc
	loginSeq   int
	copied     bool

	play  playState
	table *TableModel
	ws    *wsclient.Client
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
	ti.Focus()
	sp := spinner.New()
	sp.Style = accentStyle
	return &Shell{
		cfg: cfg, session: session, rc: rc, state: stateCheckingLogin, input: ti, spin: sp,
		menu: newCommandMenu(homeCommandSpecs), followBottom: true,
	}
}

// appendLine appends line to the scrollback. The viewport keeps following
// the bottom on the next render unless the user has scrolled away from it.
func (s *Shell) appendLine(line string) {
	s.lines = append(s.lines, line)
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
	chrome := 2 // logo + input
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
			s.input.Placeholder = "/help"
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
		s.input.Reset()
		s.input.Placeholder = buyInHint(s.play.bigBlind)
		s.state = statePlayBuyin
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
		s.startTable()
		return s, tea.Batch(pumpTable(s.ws), s.table.Init(), sendReadyCmd(s.ws))

	case wsStreamMsg:
		var cmd tea.Cmd
		if s.table != nil {
			var tm tea.Model
			tm, cmd = s.table.Update(SnapshotMsg{M: msg.m})
			s.table = tm.(*TableModel)
		}
		return s, tea.Batch(cmd, pumpTable(s.ws))

	case ReconnectingMsg, ReconnectedMsg:
		if s.table != nil {
			tm, _ := s.table.Update(msg)
			s.table = tm.(*TableModel)
		}
		return s, pumpTable(s.ws)

	case wsClosedMsg:
		s.leaveTable()
		if msg.err != nil {
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
		s.leaveTable()
		return s, nil

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
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
	}
}

func (s *Shell) leaveTable() {
	if s.ws != nil {
		_ = s.ws.Close()
		s.ws = nil
	}
	s.table = nil
	s.state = stateHome
	s.input.Reset()
	s.input.Placeholder = "/help"
	s.appendLine("· de volta ao lobby")
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
			s.input.Reset()
			s.input.Placeholder = buyInHint(s.play.bigBlind)
			s.state = statePlayBuyin
		case "esc":
			s.state = stateHome
		}
		return s, nil

	case statePlayBuyin:
		if msg.String() == "enter" {
			amount, err := parseBuyIn(s.input.Value(), s.play.bigBlind)
			if err != nil {
				s.appendLine("· " + err.Error())
				return s, nil
			}
			s.input.Reset()
			s.busy = true
			s.state = stateJoining
			if s.play.fromEnter {
				return s, joinRoom(s.rc, s.play, amount)
			}
			return s, joinOrCreate(s.rc, s.play, amount)
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd

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
		return fmt.Sprintf("%s\n\nComo você quer entrar?\n%sAbrir Navegador\n%sAPI Key\n%sSair%s\n",
			renderLogo(), cursor(0), cursor(1), cursor(2), errLine)
	case stateAPIKeyInput:
		view := renderLogo() + "\n\n" + s.input.View()
		if s.busy {
			view += "\n" + accentStyle.Render(s.spin.View()) + " verificando..."
		}
		return view
	case stateLoggingIn:
		var b strings.Builder
		b.WriteString(renderLogo())
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
		return fmt.Sprintf("%s\n%s", mutedStyle.Render(buyInHint(s.play.bigBlind)), s.input.View())
	case stateJoining:
		return accentStyle.Render(s.spin.View()) + " entrando na mesa..."
	case stateInTable:
		if s.table != nil {
			return s.table.View()
		}
		return ""
	default: // stateHome, stateCheckingLogin
		if s.state == stateCheckingLogin {
			return renderLogo() + "\n" + accentStyle.Render(s.spin.View()) + " verificando login..."
		}
		lines := []string{renderLogo()}
		if s.vpReady {
			vpH, menuRows := s.layoutHeights()
			s.viewport.Height = vpH
			if s.followBottom {
				s.viewport.GotoBottom()
			}
			if vpH > 0 {
				lines = append(lines, s.viewport.View())
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
		return strings.Join(lines, "\n")
	}
}
