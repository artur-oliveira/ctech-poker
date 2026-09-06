package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.aoctech.app/poker/cli/internal/auth"
	"gopkg.aoctech.app/poker/cli/internal/config"
	"gopkg.aoctech.app/poker/cli/internal/rest"
	"gopkg.aoctech.app/poker/cli/internal/wsclient"
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

func TestTableExitCarriesAuthoritativeOutcomeIntoLobby(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateInTable
	s.table = NewTableModel(TableConfig{RoomID: "r-1", RoomName: "Aurora"})

	m, _ := s.Update(TableExitedMsg{
		RoomID: "r-1", RoomName: "Aurora", Reason: "exit_requested",
		SettledAmount: 1500, SettlementKnown: true,
	})
	s = m.(*Shell)
	if s.state != stateHome || s.table != nil {
		t.Fatalf("table exit did not return home: state=%v table=%v", s.state, s.table)
	}
	out := strings.Join(s.lines, "\n")
	for _, want := range []string{"mesa Aurora", "banca final 1500 fichas", "saída concluída"} {
		if !strings.Contains(out, want) {
			t.Errorf("lobby handoff missing %q: %q", want, out)
		}
	}
	m, _ = s.Update(tableExitRecapMsg{
		exited:  TableExitedMsg{RoomID: "r-1", RoomName: "Aurora"},
		session: rest.Session{TableID: "r-1", BuyinAmount: 1000, CashoutAmount: 1500, NetPnL: 500, JoinedAt: 1_000, EndedAt: 3_701_000},
	})
	s = m.(*Shell)
	out = strings.Join(s.lines, "\n")
	for _, want := range []string{"Resumo da sessão", "Tempo na mesa   1h 01min", "Entrada         1000 fichas", "Retirada        1500 fichas", "+500 fichas"} {
		if !strings.Contains(out, want) {
			t.Fatalf("session recap missing %q: %q", want, out)
		}
	}
}

func TestForcedTableExitDoesNotClaimSettlement(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateInTable
	s.table = NewTableModel(TableConfig{RoomID: "r-1", RoomName: "Aurora"})

	m, _ := s.Update(TableExitedMsg{RoomName: "Aurora", Reason: "forced"})
	s = m.(*Shell)
	out := strings.Join(s.lines, "\n")
	if !strings.Contains(out, "liquidação não confirmada") || strings.Contains(out, "banca final") {
		t.Fatalf("forced exit claimed an authoritative settlement: %q", out)
	}
}

func TestStaleSocketCloseDoesNotOverwriteExitHandoff(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome
	s.appendLine("Você saiu da mesa Aurora · banca final 1500 fichas")
	stale := wsclient.New("ws://stale.invalid", nil, "")

	m, _ := s.Update(wsClosedMsg{client: stale, err: errors.New("closed")})
	s = m.(*Shell)
	out := strings.Join(s.lines, "\n")
	if strings.Contains(out, "de volta ao lobby") || strings.Contains(out, "conexão encerrada") {
		t.Fatalf("stale socket close overwrote the structured exit handoff: %q", out)
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
	m, cmd = s.Update(msg)
	s = m.(*Shell)
	if s.state != stateLoginConfirm {
		t.Fatalf("state = %v, want stateLoginConfirm after a successful login", s.state)
	}
	// The name lookup (no `name` JWT claim on this opaque token, so it falls
	// through to a /profile fetch against an unreachable APIBaseURL) fails
	// harmlessly — the confirm screen just shows no name.
	m, _ = s.Update(runCmd(t, s, cmd))
	s = m.(*Shell)

	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if s.state != stateHome {
		t.Fatalf("state = %v, want stateHome after confirming login", s.state)
	}
}

// TestLoginCommandSwitchesAccountWithoutQuitting covers /login: it must be
// reachable while already authenticated, its "sair" option must cancel back
// to stateHome (not quit the program, unlike the mandatory startup gate),
// and completing it must show the new account's name and clear stale
// scrollback/social-page state from the old account.
func TestLoginCommandSwitchesAccountWithoutQuitting(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"name": "Beto"})
	}))
	defer apiSrv.Close()
	var mux http.ServeMux
	mux.HandleFunc("/v1.0/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at-2", "token_type": "Bearer", "expires_in": 3600})
	})
	accountSrv := httptest.NewServer(&mux)
	defer accountSrv.Close()

	cfg := config.Settings{ConfigDir: t.TempDir(), ClientID: "poker-cli", AccountBaseURL: accountSrv.URL, APIBaseURL: apiSrv.URL}
	session := auth.NewSession(cfg, http.DefaultClient)
	if err := auth.SaveCredentials(config.CredentialsPath(cfg), auth.Credentials{
		AccessToken: "at-1", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	rc := rest.New(cfg.APIBaseURL, session.Token, http.DefaultClient)
	s := newShell(cfg, session, rc)
	s.state = stateHome
	s.lines = []string{"old account output"}
	s.socialPages = map[string]socialPageState{"friends": {index: 2}}

	model, cmd := s.dispatch("/login")
	s = model.(*Shell)
	if s.state != stateLoginChoice || !s.reauth || cmd != nil {
		t.Fatalf("/login did not open the login choice: state=%v reauth=%v", s.state, s.reauth)
	}
	if !strings.Contains(s.View(), "Cancelar") {
		t.Fatal("/login's login choice must offer Cancelar, not Sair, while already authenticated")
	}

	// "sair"/Cancelar (index 2) must return home, never tea.Quit.
	s.loginChoice = 2
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if s.state != stateHome || s.reauth || cmd != nil {
		t.Fatalf("Cancelar must return to stateHome without quitting: state=%v reauth=%v cmd=%v", s.state, s.reauth, cmd)
	}

	// Re-open and this time complete an API-key login.
	model, _ = s.dispatch("/login")
	s = model.(*Shell)
	s.state = stateAPIKeyInput
	for _, r := range "new-key" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	m, cmd = s.Update(runCmd(t, s, cmd)) // loginResultMsg -> enterLoginConfirm
	s = m.(*Shell)
	if s.state != stateLoginConfirm {
		t.Fatalf("state = %v, want stateLoginConfirm", s.state)
	}
	m, _ = s.Update(runCmd(t, s, cmd)) // authNameMsg from the /profile fetch
	s = m.(*Shell)
	if s.authenticatedName != "Beto" {
		t.Fatalf("authenticatedName = %q, want the new account's name", s.authenticatedName)
	}

	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if s.state != stateHome {
		t.Fatalf("state = %v, want stateHome after confirming", s.state)
	}
	if len(s.lines) != 1 || !strings.Contains(s.lines[0], "Beto") {
		t.Fatalf("lines = %v, want stale scrollback cleared and a confirmation naming Beto", s.lines)
	}
	if len(s.socialPages) != 0 {
		t.Fatalf("socialPages = %v, want cleared after switching accounts", s.socialPages)
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

func TestHomeFriendsCommandPrintsResult(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/social/friends" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"player_id": "caio", "name": "Caio", "presence": "in_table"}},
		})
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

	for _, r := range "/friends" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	msg := runCmd(t, s, cmd)
	m, _ = s.Update(msg)
	s = m.(*Shell)
	if !strings.Contains(s.View(), "Caio") || !strings.Contains(s.View(), "na mesa") {
		t.Fatalf("view missing friends result: %q", s.View())
	}
}

func TestHomeRequestsCommandDefaultsToIncomingAndAcceptsSent(t *testing.T) {
	var gotDirection string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDirection = r.URL.Query().Get("direction")
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
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
	m, cmd := s.dispatch("/requests")
	s = m.(*Shell)
	runCmd(t, s, cmd)
	if gotDirection != "incoming" {
		t.Fatalf("default direction = %q, want incoming", gotDirection)
	}

	m, cmd = s.dispatch("/requests sent")
	s = m.(*Shell)
	runCmd(t, s, cmd)
	if gotDirection != "outgoing" {
		t.Fatalf("/requests sent direction = %q, want outgoing", gotDirection)
	}

	m, cmd = s.dispatch("/requests bogus")
	s = m.(*Shell)
	if cmd != nil || !strings.Contains(s.View(), "uso: /requests") {
		t.Fatalf("invalid arg should error locally, not dispatch: view=%q", s.View())
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

func TestHomeSlashOpensMenuAndTabOnlyFillsNeverSubmits(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome

	for _, r := range "/pl" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	if !s.menu.visible {
		t.Fatal("menu should be visible while typing a command prefix")
	}
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyTab})
	s = m.(*Shell)
	// Tab is pure autocomplete: it fills the input, even for a zero-argument
	// command like /play, but never dispatches it — only Enter does that.
	if s.input.Value() != "/play" {
		t.Fatalf("input = %q, want tab-completed to /play", s.input.Value())
	}
	if s.state != stateHome || cmd != nil {
		t.Fatalf("Tab must not submit: state=%v cmd=%v", s.state, cmd)
	}

	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if s.state != statePlaySize {
		t.Fatalf("state = %v, want statePlaySize after pressing Enter", s.state)
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
	if !strings.Contains(out, "/profile") || !strings.Contains(out, "Veja seu perfil, código e saldos") {
		t.Fatalf("help output missing command+description: %q", out)
	}
}

func TestViewportShrinksToContentNotFullWindow(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome
	m, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	s = m.(*Shell)

	s.appendLine("one")
	s.appendLine("two")
	// Two lines of content in a 38-row-tall window: the viewport must size
	// to the content, not claim the whole window (the bug: a fixed-height
	// viewport left no room for the command menu below it).
	vpH, _ := s.layoutHeights()
	if vpH != 2 {
		t.Fatalf("viewport height = %d, want 2 (content-sized)", vpH)
	}
}

func TestViewportReservesRoomForOpenMenu(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome
	m, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	s = m.(*Shell)
	for i := 0; i < 20; i++ {
		s.appendLine("line")
	}
	fullHeight, _ := s.layoutHeights()

	for _, r := range "/pr" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	if !s.menu.visible {
		t.Fatal("menu should be visible for /pr")
	}
	vpH, menuRows := s.layoutHeights()
	if vpH >= fullHeight {
		t.Fatalf("viewport height = %d, want less than %d once the menu reserved its rows", vpH, fullHeight)
	}
	chrome := strings.Count(renderHomeHeader(s.windowWidth), "\n") + 2 // header + input
	if vpH+menuRows+chrome > s.windowHeight {
		t.Fatalf("viewport (%d) + menu (%d) + chrome (%d) exceeds the available window (%d) — menu would be pushed off-screen",
			vpH, menuRows, chrome, s.windowHeight)
	}
}

func TestHomeArrowKeysScrollLongOutput(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome
	m, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	s = m.(*Shell)
	for i := 0; i < 60; i++ {
		s.appendLine(fmt.Sprintf("conquista %d", i))
	}
	// Following the bottom: no scroll hint, view fits the window.
	if strings.Contains(s.View(), "rolam") {
		t.Fatal("scroll hint shown while already at the bottom")
	}
	// ArrowUp (menu closed) scrolls the scrollback back.
	for i := 0; i < 5; i++ {
		m, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp})
		s = m.(*Shell)
	}
	if s.followBottom {
		t.Fatal("ArrowUp did not move the viewport off the bottom")
	}
	view := s.View()
	if !strings.Contains(view, "rolam") {
		t.Fatalf("scroll hint missing after scrolling up:\n%s", view)
	}
	if got := len(strings.Split(view, "\n")); got > 12 {
		t.Fatalf("View() = %d lines, want <= 12", got)
	}
}

// TestViewNeverOverflowsWindow is the regression guard for the real bug
// reported live: on a small terminal, a wide command menu (or a long
// scrollback) could add up to more lines than the window actually has,
// silently pushing the top of the menu (often /profile, alphabetically
// first) out of the visible area — recoverable only by scrolling the
// terminal's own history, not by anything this program could show. No
// combination of window height, prior scrollback size, or typed prefix may
// ever make View() taller than the window.
func TestViewNeverOverflowsWindow(t *testing.T) {
	for _, height := range []int{4, 6, 8, 10, 20, 24, 40} {
		s := newTestShell(t, nil, t.TempDir())
		s.state = stateHome
		s.appendLine("Logado.")
		m, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: height})
		s = m.(*Shell)

		for _, seq := range []string{"/", "/p", "/pr", "/pro", "/profile", "/e"} {
			s.input.SetValue("")
			for _, r := range seq {
				m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				s = m.(*Shell)
			}
			got := strings.Split(s.View(), "\n")
			if len(got) > height {
				t.Fatalf("height=%d seq=%q: View() produced %d lines, want <= %d\n%s",
					height, seq, len(got), height, s.View())
			}
		}
	}
}

func TestBrowserLoginLocksScreenAndShowsURL(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateLoginChoice
	s.loginChoice = 0

	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if s.state != stateLoggingIn || s.pkceFlow == nil {
		t.Fatalf("state=%v pkceFlow=%v, want locked into stateLoggingIn with a flow", s.state, s.pkceFlow)
	}
	if cmd == nil {
		t.Fatal("selecting browser login should kick off a command")
	}
	url := s.pkceFlow.AuthorizeURL
	if !strings.Contains(s.View(), url) {
		t.Fatalf("view should show the authorize URL: %q", s.View())
	}

	// While locked, arbitrary keys (e.g. trying to switch options) must do
	// nothing.
	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	s = m.(*Shell)
	if s.state != stateLoggingIn {
		t.Fatalf("state = %v, want to stay locked on stateLoggingIn", s.state)
	}
	s.pkceFlow.Cancel() // release the loopback port before the test ends
}

func TestBrowserLoginCopyShowsFlashThenResets(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateLoginChoice
	s.loginChoice = 0
	m, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	defer s.pkceFlow.Cancel()

	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	s = m.(*Shell)
	if !s.copied {
		t.Fatal("'c' should mark the URL as copied")
	}
	if !strings.Contains(s.View(), "Copiado!") {
		t.Fatalf("view should show the copied flash: %q", s.View())
	}
	if cmd == nil {
		t.Fatal("copy should schedule the flash reset")
	}
	msg := cmd()
	m, _ = s.Update(msg)
	s = m.(*Shell)
	if s.copied {
		t.Fatal("copied flash should clear after the reset message")
	}
}

func TestBrowserLoginEscCancelsAndReturnsToChoice(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateLoginChoice
	s.loginChoice = 0
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	seqAtStart := s.loginSeq

	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	s = m.(*Shell)
	if s.state != stateLoginChoice || s.pkceFlow != nil {
		t.Fatalf("state=%v pkceFlow=%v, want back to stateLoginChoice with no flow", s.state, s.pkceFlow)
	}
	if s.loginSeq == seqAtStart {
		t.Fatal("cancelling must bump loginSeq so a late result is ignored")
	}

	// The in-flight FinishPKCE goroutine's result, once it arrives, must be
	// ignored — it carries the pre-cancel seq.
	msg := cmd()
	drained, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected a batched command, got %T", msg)
	}
	for _, c := range drained {
		if bm, ok := c().(browserLoginResultMsg); ok {
			m, _ := s.Update(bm)
			s = m.(*Shell)
		}
	}
	if s.state != stateLoginChoice {
		t.Fatalf("a stale result must not move the state, got %v", s.state)
	}
}

// TestResultAfterFirstCommandShowsAllLinesNotJustTheLast is the regression
// guard for the bug reported live: a command's whole result rendered as just
// its last line with the rest of the screen blank. Root cause: GotoBottom
// was called from syncViewport (i.e. from Update, when new lines arrive)
// against whatever viewport.Height an earlier render had left set — often
// smaller, e.g. right after submitting a one-line echoed command — computing
// a scroll offset for a size that hadn't been recomputed yet. The fix moved
// GotoBottom into View, always right after layoutHeights sets the correct
// Height for the content that's about to show.
func TestResultAfterFirstCommandShowsAllLinesNotJustTheLast(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"name": "Ana", "friend_code": "PKR-X", "wallet_mode": "sandbox",
			"sandbox_balance": 5000, "game_balance": 0,
		})
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
	m, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	s = m.(*Shell)

	for _, r := range "/profile" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)
	if cmd == nil {
		t.Fatal("expected a command to run /profile")
	}
	m, _ = s.Update(cmd())
	s = m.(*Shell)

	view := s.View()
	for _, want := range []string{"Ana", "PKR-X", "sandbox", "Fichas: 5.000"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q from view — result was clipped:\n%s", want, view)
		}
	}
}

// TestHelpShowsEveryLineNotJustTheLastFew is the regression guard for
// another real bug reproduced live: layoutHeights sizes the viewport from
// len(s.lines), on the invariant that one slice element is one visual line.
// /help's dispatch appended formatCommandList's whole multi-line reference
// as a *single* appendLine call — one slice element internally containing
// nine embedded newlines — so len(s.lines) undercounted the true visual line
// count (10 vs. 2), sizing the viewport far too short and showing only its
// last couple of lines. appendLine/appendLog now split any argument that
// contains embedded newlines before storing it, restoring the invariant no
// matter how a caller joins its own content beforehand.
func TestHelpShowsEveryLineNotJustTheLastFew(t *testing.T) {
	s := newTestShell(t, nil, t.TempDir())
	s.state = stateHome
	m, _ := s.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	s = m.(*Shell)

	for _, r := range "/help" {
		m, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = m.(*Shell)
	}
	m, _ = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s = m.(*Shell)

	view := s.View()
	for _, spec := range homeCommandSpecs {
		if !strings.Contains(view, spec.Name) {
			t.Fatalf("missing %q from /help output — result was clipped:\n%s", spec.Name, view)
		}
	}
}

func TestHandsCommandOpensArchiveAndReturnsHome(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/players/me/hands" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
			"pk": "you", "hand_id": "hand-1", "table_id": "table-1", "outcome": "won", "net_change": 120,
		}}})
	}))
	defer apiSrv.Close()

	dir := t.TempDir()
	cfg := config.Settings{ConfigDir: dir}
	if err := auth.SaveCredentials(config.CredentialsPath(cfg), auth.Credentials{AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	s := newTestShell(t, apiSrv, dir)
	s.state = stateHome
	_, _ = s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, cmd := s.dispatch("/hands")
	s = model.(*Shell)
	if s.state != stateHands || s.hands == nil || cmd == nil {
		t.Fatalf("archive did not open: state=%v hands=%v cmd=%v", s.state, s.hands, cmd)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected a batched command (load + window title)")
	}
	for _, c := range batch {
		if msg := c(); msg != nil {
			model, _ = s.Update(msg)
			s = model.(*Shell)
		}
	}
	if !strings.Contains(s.View(), "Histórico de mãos") || !strings.Contains(s.View(), "+120 fichas") {
		t.Fatalf("archive result missing:\n%s", s.View())
	}
	model, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	s = model.(*Shell)
	if cmd == nil {
		t.Fatal("archive Esc did not schedule exit")
	}
	model, _ = s.Update(cmd())
	s = model.(*Shell)
	if s.state != stateHome || s.hands != nil {
		t.Fatalf("archive did not return home: state=%v hands=%v", s.state, s.hands)
	}
}

func TestFriendsCursorPaginationRemembersPreviousPage(t *testing.T) {
	var cursors []string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)
		if cursor == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":     []map[string]any{{"player_id": "ana", "name": "Ana", "presence": "online"}},
				"has_next": true, "next_cursor": "cursor-2",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"player_id": "bia", "name": "Bia"}}})
	}))
	defer apiSrv.Close()

	dir := t.TempDir()
	cfg := config.Settings{ConfigDir: dir}
	if err := auth.SaveCredentials(config.CredentialsPath(cfg), auth.Credentials{AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	s := newTestShell(t, apiSrv, dir)
	s.state = stateHome
	s.windowWidth = 100

	model, cmd := s.dispatch("/friends")
	s = model.(*Shell)
	model, _ = s.Update(cmd())
	s = model.(*Shell)
	model, cmd = s.dispatch("/friends next")
	s = model.(*Shell)
	model, _ = s.Update(cmd())
	s = model.(*Shell)
	if !strings.Contains(strings.Join(s.lines, "\n"), "Bia") || len(cursors) != 2 || cursors[1] != "cursor-2" {
		t.Fatalf("next page missing: cursors=%v lines=%q", cursors, s.lines)
	}
	model, cmd = s.dispatch("/friends prev")
	s = model.(*Shell)
	model, _ = s.Update(cmd())
	s = model.(*Shell)
	if len(cursors) != 3 || cursors[2] != "" {
		t.Fatalf("previous cursor not restored: %v", cursors)
	}
}
