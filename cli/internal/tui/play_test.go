package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.aoctech.app/poker/cli/internal/auth"
	"gopkg.aoctech.app/poker/cli/internal/config"
	"gopkg.aoctech.app/poker/cli/internal/rest"
)

func TestParseBuyIn(t *testing.T) {
	if _, err := parseBuyIn("10", 10); err == nil {
		t.Error("below 20*BB should fail")
	}
	if _, err := parseBuyIn("2000", 10); err == nil {
		t.Error("above 100*BB should fail")
	}
	if _, err := parseBuyIn("105", 10); err == nil {
		t.Error("non-multiple of BB should fail")
	}
	got, err := parseBuyIn("500", 10)
	if err != nil || got != 500 {
		t.Fatalf("got %d err %v", got, err)
	}
	// Tolerate the label / units the user may have typed around the number
	// (the reported confusion: someone typed "buy-in 2000" into the field).
	for _, in := range []string{"buy-in 500", "  500  ", "500 fichas", "R$ 500"} {
		if got, err := parseBuyIn(in, 10); err != nil || got != 500 {
			t.Errorf("parseBuyIn(%q) = %d, %v — want 500", in, got, err)
		}
	}
	if _, err := parseBuyIn("abc", 10); err == nil {
		t.Error("no digits should fail")
	}
}

func TestWSURLFor(t *testing.T) {
	if got := wsURLFor("https://api.example.com", "r1"); got != "wss://api.example.com/v1.0/tables/r1/ws" {
		t.Errorf("got %q", got)
	}
	if got := wsURLFor("http://localhost:8080/", "r1"); got != "ws://localhost:8080/v1.0/tables/r1/ws" {
		t.Errorf("got %q", got)
	}
}

func loggedInShell(t *testing.T, apiSrv *httptest.Server) *Shell {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Settings{ConfigDir: dir, APIBaseURL: apiSrv.URL}
	if err := auth.SaveCredentials(config.CredentialsPath(cfg), auth.Credentials{
		AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	session := auth.NewSession(cfg, http.DefaultClient)
	rc := rest.New(cfg.APIBaseURL, session.Token, http.DefaultClient)
	s := newShell(cfg, session, rc)
	s.state = stateHome
	return s
}

func drain(t *testing.T, s *Shell, cmd tea.Cmd) {
	t.Helper()
	for _, msg := range drainCmd(cmd) {
		if msg == nil {
			continue
		}
		m, next := s.Update(msg)
		*s = *m.(*Shell)
		drain(t, s, next)
	}
}

func TestPlayFlowReachesJoinOrCreateWithChosenBucket(t *testing.T) {
	var joinBody map[string]any
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/rooms/stakes":
			json.NewEncoder(w).Encode(map[string]any{
				"currency_mode": "sandbox", "unit": "chip",
				"stakes": []map[string]int64{{"small_blind": 1, "big_blind": 2}, {"small_blind": 5, "big_blind": 10}},
			})
		case "/v1.0/rooms/join-or-create":
			json.NewDecoder(r.Body).Decode(&joinBody)
			json.NewEncoder(w).Encode(map[string]any{"room_id": "room-xyz", "created": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer apiSrv.Close()

	s := loggedInShell(t, apiSrv)

	// /play
	m, cmd := s.Update(keyRunes("/play"))
	*s = *m.(*Shell)
	m, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	*s = *m.(*Shell)
	if s.state != statePlaySize {
		t.Fatalf("state = %v, want statePlaySize", s.state)
	}

	// choose 6-Max (index 1), enter -> loads stakes
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, cmd = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	*s = *m.(*Shell)
	m, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	*s = *m.(*Shell)
	drain(t, s, cmd)
	if s.state != statePlayStake {
		t.Fatalf("state = %v, want statePlayStake", s.state)
	}

	// pick the first stake, enter -> buy-in prompt, pre-filled with the
	// default 100-BB stack.
	m, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	*s = *m.(*Shell)
	if s.state != statePlayBuyin {
		t.Fatalf("state = %v, want statePlayBuyin", s.state)
	}
	if s.input.Value() != "200" { // big_blind 2 * 100
		t.Fatalf("buy-in input not pre-filled: %q", s.input.Value())
	}

	// accept the default buy-in -> auto-rebuy step
	m, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	*s = *m.(*Shell)
	if s.state != statePlayAutoRebuy {
		t.Fatalf("state = %v, want statePlayAutoRebuy", s.state)
	}
	// toggle auto-rebuy on, then confirm -> joins
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	*s = *m.(*Shell)
	drain(t, s, cmd)

	if joinBody == nil {
		t.Fatal("join-or-create was never called")
	}
	if joinBody["max_seats"].(float64) != 9 && joinBody["max_seats"].(float64) != 6 {
		t.Errorf("max_seats = %v", joinBody["max_seats"])
	}
	if joinBody["big_blind"].(float64) != 2 {
		t.Errorf("big_blind = %v", joinBody["big_blind"])
	}
	if joinBody["amount"].(float64) != 200 {
		t.Errorf("amount = %v, want the pre-filled default 200", joinBody["amount"])
	}
	if joinBody["auto_rebuy"] != true {
		t.Errorf("auto_rebuy = %v, want true (toggled on)", joinBody["auto_rebuy"])
	}
	if joinBody["idem_key"] == "" {
		t.Error("idem_key not sent")
	}
}

func keyRunes(s string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
