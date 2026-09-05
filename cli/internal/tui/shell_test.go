package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.aoctech.app/poker/cli/internal/auth"
	"gopkg.aoctech.app/poker/cli/internal/config"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

func newTestShell(t *testing.T, apiSrv *httptest.Server, credDir string) *Shell {
	t.Helper()
	cfg := config.Settings{ConfigDir: credDir, ClientID: "poker-cli"}
	if apiSrv != nil {
		cfg.APIBaseURL = apiSrv.URL
	}
	session := auth.NewSession(cfg, http.DefaultClient)
	rc := rest.New(cfg.APIBaseURL, session.Token, http.DefaultClient)
	return newShell(cfg, session, rc)
}

func runCmd(t *testing.T, s *Shell, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestCheckLoginMsgWithNoCredentialsShowsLoginChoice(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	m, _ := s.Update(checkLoginMsg{loggedIn: false})
	s = m.(*Shell)
	if s.state != stateLoginChoice {
		t.Fatalf("state = %v, want stateLoginChoice", s.state)
	}
}

func TestCheckLoginMsgWithCredentialsShowsHome(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	m, _ := s.Update(checkLoginMsg{loggedIn: true})
	s = m.(*Shell)
	if s.state != stateHome {
		t.Fatalf("state = %v, want stateHome", s.state)
	}
}

func TestLoginChoiceDownThenEnterEntersAPIKeyInput(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateLoginChoice

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s = m.(*Shell)
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if s.state != stateAPIKeyInput {
		t.Fatalf("state = %v, want stateAPIKeyInput", s.state)
	}
	if cmd != nil {
		t.Fatal("choosing the API key option must not fire a network command yet")
	}
}

func TestAPIKeyLoginFlowEndsInHome(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at-1", "token_type": "Bearer", "expires_in": 3600})
	})
	accountSrv := httptest.NewServer(&mux)
	defer accountSrv.Close()

	cfg := config.Settings{ConfigDir: t.TempDir(), ClientID: "poker-cli", AccountBaseURL: accountSrv.URL}
	session := auth.NewSession(cfg, http.DefaultClient)
	rc := rest.New(cfg.APIBaseURL, session.Token, http.DefaultClient)
	s := newShell(cfg, session, rc)
	s.state = stateAPIKeyInput

	for _, r := range "my-key" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if cmd == nil {
		t.Fatal("submitting the API key must fire a login command")
	}

	msg := runCmd(t, s, cmd)
	m, _ = s.Update(msg)
	s = m.(*Shell)
	if s.state != stateHome {
		t.Fatalf("state = %v, want stateHome after a successful login", s.state)
	}
}

func TestHomeProfileCommandPrintsResult(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"name": "Ana", "wallet_mode": "sandbox", "sandbox_balance": 5000})
	}))
	defer apiSrv.Close()

	dir := t.TempDir()
	cfg := config.Settings{ConfigDir: dir}
	if err := auth.SaveCredentials(config.CredentialsPath(cfg), auth.Credentials{
		AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	s := newTestShell(t, apiSrv, dir)
	s.state = stateHome

	for _, r := range "/profile" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if !s.busy || cmd == nil {
		t.Fatal("/profile must dispatch a busy network command")
	}

	msg := runCmd(t, s, cmd)
	m, _ = s.Update(msg)
	s = m.(*Shell)
	if s.busy {
		t.Fatal("shell should no longer be busy once the result arrives")
	}
	if !strings.Contains(s.View(), "Ana") {
		t.Fatalf("view missing profile result: %q", s.View())
	}
}

func TestHomeUnknownCommandIsEchoedWithHint(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome

	for _, r := range "/nope" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if !strings.Contains(s.View(), "comando desconhecido") {
		t.Fatalf("view = %q", s.View())
	}
}

func TestHomeLogoutReturnsToLoginChoice(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome

	for _, r := range "/logout" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if s.state != stateLoginChoice {
		t.Fatalf("state = %v, want stateLoginChoice after /logout", s.state)
	}
}

func TestHomeExitQuits(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome

	for _, r := range "/exit" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if s.state != stateQuitting || cmd == nil {
		t.Fatalf("state = %v cmd = %v, want stateQuitting + tea.Quit", s.state, cmd)
	}
}

func TestHomeClearCommandEmptiesScrollback(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome
	s.appendLine("something from before")

	for _, r := range "/clear" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if len(s.lines) != 0 {
		t.Fatalf("lines = %v, want empty after /clear", s.lines)
	}
}

func TestHomeCtrlLClearsScrollback(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome
	s.appendLine("something from before")

	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	s = m.(*Shell)
	if len(s.lines) != 0 {
		t.Fatalf("lines = %v, want empty after Ctrl+L", s.lines)
	}
}

func TestHomeSlashOpensMenuAndTabCompletes(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome

	for _, r := range "/pl" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	if !s.menu.visible {
		t.Fatal("menu should be visible while typing a command prefix")
	}
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = m.(*Shell)
	// /play takes no arguments, so completing it also submits it immediately
	// (same as typing it out and pressing Enter) rather than leaving it in
	// the input for a second Enter.
	if s.state != statePlaySize {
		t.Fatalf("state = %v, want statePlaySize after completing /play", s.state)
	}
	if s.menu.visible {
		t.Fatal("menu should hide after accepting")
	}
}

func TestHomeMenuArgCommandCompletesWithTrailingSpaceAndWaits(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome

	for _, r := range "/ent" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = m.(*Shell)
	if s.input.Value() != "/enter " {
		t.Fatalf("input = %q, want /enter with a trailing space", s.input.Value())
	}
	if cmd != nil {
		t.Fatal("an arg-taking command must not auto-submit on completion")
	}
}

func TestHomeHelpListsCommandsWithDescriptions(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome

	for _, r := range "/help" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	out := strings.Join(s.lines, "\n")
	if !strings.Contains(out, "/profile") || !strings.Contains(out, "Mostra seu perfil") {
		t.Fatalf("help output missing command+description: %q", out)
	}
}
