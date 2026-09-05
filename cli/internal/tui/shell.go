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
	"gopkg.aoctech.app/poker/cli/internal/rest"
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

	state         shellState
	loginChoice   int // 0 = browser, 1 = api key
	loginErr      error
	input         textinput.Model
	spin          spinner.Model
	lines         []string
	busy          bool
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

	case tea.KeyMsg:
		return s.handleKey(msg)
	}
	return s, nil
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
	}
	return s, nil
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
		s.appendLine("/play ainda não está disponível nesta build (chega numa próxima task).")
		return s, nil
	case "/enter":
		if len(args) != 1 {
			s.appendLine("uso: /enter <room-id>")
			return s, nil
		}
		s.appendLine("/enter ainda não está disponível nesta build (chega numa próxima task).")
		return s, nil
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
