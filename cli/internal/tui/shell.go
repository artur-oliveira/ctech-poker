package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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
	loginChoice int // 0 = browser, 1 = api key
	loginErr    error
	input       textinput.Model
	spin        spinner.Model
	lines       []string
	busy        bool

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
	ti.Focus()
	sp := spinner.New()
	return &Shell{cfg: cfg, session: session, rc: rc, state: stateCheckingLogin, input: ti, spin: sp}
}

func (s *Shell) Init() tea.Cmd {
	return tea.Batch(checkLogin(s.session), s.spin.Tick)
}

type checkLoginMsg struct{ loggedIn bool }
type loginResultMsg struct{ err error }
type commandResultMsg struct{ lines []string }

func checkLogin(session *auth.Session) tea.Cmd {
	return func() tea.Msg {
		_, err := session.Token(context.Background())
		return checkLoginMsg{loggedIn: !errors.Is(err, auth.ErrLoggedOut)}
	}
}

func loginPKCE(session *auth.Session) tea.Cmd {
	return func() tea.Msg {
		err := session.LoginPKCE(context.Background(), auth.OpenBrowser)
		return loginResultMsg{err: err}
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
		s.appendLine("Logado.")
		return s, nil

	case commandResultMsg:
		s.busy = false
		s.lines = append(s.lines, msg.lines...)
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
		case "up", "down":
			s.loginChoice = 1 - s.loginChoice
		case "enter":
			if s.loginChoice == 0 {
				s.busy = true
				s.appendLine("Abrindo o navegador para login...")
				return s, loginPKCE(s.session)
			}
			s.state = stateAPIKeyInput
			s.input.Reset()
			s.input.Placeholder = "cole sua API key"
		}
		return s, nil

	case stateAPIKeyInput:
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
		if msg.String() == "enter" {
			line := strings.TrimSpace(s.input.Value())
			s.input.Reset()
			if line == "" {
				return s, nil
			}
			s.appendLine(s.input.Prompt + line)
			return s.dispatch(line)
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
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

func (s *Shell) appendLine(line string) { s.lines = append(s.lines, line) }

// dispatch parses one `/command [args]` home-screen line and returns the
// resulting tea.Cmd. Unknown input is echoed back with a hint.
func (s *Shell) dispatch(line string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(line)
	cmd := fields[0]
	args := fields[1:]

	switch cmd {
	case "/help":
		s.appendLine(helpText)
		return s, nil
	case "/logout":
		if err := s.session.Logout(); err != nil {
			s.appendLine("erro: " + err.Error())
			return s, nil
		}
		s.state = stateLoginChoice
		s.lines = nil
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
		s.appendLine(fmt.Sprintf("comando desconhecido: %s (tente /help)", cmd))
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
		return "não logado — use /logout e faça login de novo"
	}
	if rest.IsStatus(err, http.StatusForbidden) {
		return "erro: " + err.Error() + " (este ambiente pode não ter habilitado o client poker-cli — ver docs/specs/2026-09-05-poker-cli.md §2)"
	}
	return "erro: " + err.Error()
}

const helpText = `comandos: /profile /achievements /play /enter <id> /logout /exit /help`

func (s *Shell) View() string {
	switch s.state {
	case stateQuitting:
		return ""
	case stateLoginChoice:
		cursor := func(i int) string {
			if i == s.loginChoice {
				return "› "
			}
			return "  "
		}
		errLine := ""
		if s.loginErr != nil {
			errLine = "\nerro no login: " + s.loginErr.Error()
		}
		return fmt.Sprintf("CTech Poker CLI\n\nComo você quer entrar?\n%sNavegador (PKCE)\n%sAPI Key%s\n",
			cursor(0), cursor(1), errLine)
	case stateAPIKeyInput:
		return "CTech Poker CLI\n\n" + s.input.View()
	case stateLoggingIn:
		return s.spin.View() + " entrando..."
	case statePlaySize:
		var b strings.Builder
		b.WriteString("Tamanho da mesa:\n")
		for i, sz := range tableSizes {
			cur := "  "
			if i == s.play.sizeIdx {
				cur = "› "
			}
			b.WriteString(cur + sz.label + "\n")
		}
		return b.String()
	case statePlayStake:
		var b strings.Builder
		b.WriteString("Stake (small/big blind):\n")
		for i, st := range s.play.stakes {
			cur := "  "
			if i == s.play.stakeIdx {
				cur = "› "
			}
			b.WriteString(fmt.Sprintf("%s%d / %d\n", cur, st.SmallBlind, st.BigBlind))
		}
		return b.String()
	case statePlayBuyin:
		return fmt.Sprintf("%s\n%s", buyInHint(s.play.bigBlind), s.input.View())
	case stateJoining:
		return s.spin.View() + " entrando na mesa..."
	case stateInTable:
		if s.table != nil {
			return s.table.View()
		}
		return ""
	default: // stateHome, stateCheckingLogin
		body := strings.Join(s.lines, "\n")
		if s.state == stateCheckingLogin {
			return "CTech Poker CLI\n" + s.spin.View() + " verificando login..."
		}
		if s.busy {
			return body + "\n" + s.spin.View() + "\n" + s.input.View()
		}
		return body + "\n" + s.input.View()
	}
}
