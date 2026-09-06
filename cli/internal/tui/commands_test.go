package tui

import (
	"testing"

	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/proto"
)

func turnView() game.TableView {
	return game.TableView{
		IsYourTurn:      true,
		SnapshotVersion: 42,
		HandID:          "h-1",
		Legal: &proto.LegalActions{
			Actions: []string{"fold", "call", "raise"}, CallAmount: 8,
			MinRaiseTo: 16, MaxRaiseTo: 246, PotRaiseTo: 32,
		},
	}
}

func TestParseFold(t *testing.T) {
	m, _, err := ParseTableCommand("/fold", turnView())
	if err != nil || m == nil || m.Type != "act" || m.Action != "fold" {
		t.Fatalf("m=%v err=%v", m, err)
	}
	if m.ActionId == "" || m.ExpectedSnapshotVersion != 42 || m.ExpectedHandId != "h-1" {
		t.Fatalf("missing optimistic preconditions: %+v", m)
	}
}

func TestParseCallUsesCallAmount(t *testing.T) {
	m, _, err := ParseTableCommand("/call", turnView())
	if err != nil || m.Action != "call" || m.Amount != 8 {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestParseCheckRejectedWhenNotLegal(t *testing.T) {
	if _, _, err := ParseTableCommand("/check", turnView()); err == nil {
		t.Fatal("check should be rejected when not in legal actions")
	}
}

func TestParseRaiseValidatesBounds(t *testing.T) {
	if _, _, err := ParseTableCommand("/raise 10", turnView()); err == nil {
		t.Fatal("raise below min should error")
	}
	if _, _, err := ParseTableCommand("/raise 999", turnView()); err == nil {
		t.Fatal("raise above max should error")
	}
	m, _, err := ParseTableCommand("/raise 40", turnView())
	if err != nil || m.Amount != 40 {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestParsePotResolvesToPotRaiseTo(t *testing.T) {
	m, _, err := ParseTableCommand("/pot", turnView())
	if err != nil || m.Action != "raise" || m.Amount != 32 {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestPotHotkeyMatchesDocumentedContract(t *testing.T) {
	m, _, err := ParseTableCommand("p", turnView())
	if err != nil || m == nil || m.Type != "act" || m.Action != "raise" || m.Amount != 32 {
		t.Fatalf("p hotkey: m=%v err=%v", m, err)
	}
}

func TestParseAllinResolvesToMaxRaiseTo(t *testing.T) {
	m, _, err := ParseTableCommand("/allin", turnView())
	if err != nil || m.Amount != 246 {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestBettingCommandsRejectUnavailableActions(t *testing.T) {
	v := turnView()
	v.Legal.Actions = []string{"check"}
	for _, command := range []string{"/fold", "/raise 40", "/pot", "/allin"} {
		if _, _, err := ParseTableCommand(command, v); err == nil {
			t.Errorf("%s should be rejected when unavailable", command)
		}
	}
}

func TestParseTalkRejectsOverLong(t *testing.T) {
	long := "/talk " + string(make([]byte, 60))
	if _, _, err := ParseTableCommand(long, turnView()); err == nil {
		t.Fatal("chat over 50 chars should error")
	}
	m, _, err := ParseTableCommand("/talk boa mão", turnView())
	if err != nil || m.Type != "chat" || m.Message != "boa mão" {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func reactView() game.TableView {
	v := turnView()
	v.Players = []game.PlayerView{
		{ID: "p-caio", Name: "Caio", Position: "CO"},
		{ID: "p-you", Name: "VOCÊ", Position: "BB", IsYou: true},
	}
	return v
}

func TestParseReactWithTarget(t *testing.T) {
	m, _, err := ParseTableCommand("/react chip Caio", reactView())
	if err != nil || m.Type != "reaction" || m.ReactionId != "chip" || m.TargetPlayerId != "p-caio" {
		t.Fatalf("m=%v err=%v", m, err)
	}
	// position tag and raw id also resolve
	if m, _, _ := ParseTableCommand("/react chip co", reactView()); m.TargetPlayerId != "p-caio" {
		t.Fatalf("position target: %+v", m)
	}
	if m, _, _ := ParseTableCommand("/react chip p-caio", reactView()); m.TargetPlayerId != "p-caio" {
		t.Fatalf("id target: %+v", m)
	}
}

func TestParseReactValidation(t *testing.T) {
	if _, _, err := ParseTableCommand("/react gg", reactView()); err == nil {
		t.Fatal("unknown reaction code should error")
	}
	if _, _, err := ParseTableCommand("/react chip", reactView()); err == nil {
		t.Fatal("targeted reaction without a player should error")
	}
	if _, _, err := ParseTableCommand("/react clap Caio", reactView()); err == nil {
		t.Fatal("broadcast reaction with a target should error")
	}
	if _, _, err := ParseTableCommand("/react chip ninguem", reactView()); err == nil {
		t.Fatal("unknown target should error")
	}
	m, _, err := ParseTableCommand("/react clap", reactView())
	if err != nil || m.ReactionId != "clap" || m.TargetPlayerId != "" {
		t.Fatalf("broadcast reaction: %+v err=%v", m, err)
	}
}

func TestParsePeekVariants(t *testing.T) {
	// /peek is a client-only reveal toggle; it sends no frame here.
	if m, local, _ := ParseTableCommand("/peek", turnView()); m != nil || local != ActPeekBoth {
		t.Fatalf("peek all: m=%+v local=%d", m, local)
	}
	if _, local, _ := ParseTableCommand("/peek 1", turnView()); local != ActPeekCard1 {
		t.Fatalf("peek 1: local=%d", local)
	}
	if _, local, _ := ParseTableCommand("/peek 2", turnView()); local != ActPeekCard2 {
		t.Fatalf("peek 2: local=%d", local)
	}
	if _, _, err := ParseTableCommand("/peek 3", turnView()); err == nil {
		t.Fatalf("peek 3 should error")
	}
	// `k` works even when it is not your turn.
	if _, local, err := ParseTableCommand("k", game.TableView{}); err != nil || local != ActPeekBoth {
		t.Fatalf("k off-turn: local=%d err=%v", local, err)
	}
}

func TestParseAuxiliaryTableCommands(t *testing.T) {
	cases := map[string]string{
		"/rabbit":   "request_rabbit_hunt",
		"/reqcards": "request_winner_cards",
		"/accept":   "accept_winner_cards",
		"/decline":  "decline_winner_cards",
		"/keep":     "keep_seat",
		"/postbb":   "post_big_blind",
	}
	for line, want := range cases {
		m, _, err := ParseTableCommand(line, turnView())
		if err != nil || m == nil || m.Type != want || m.ActionId == "" {
			t.Fatalf("%s: m=%+v err=%v", line, m, err)
		}
	}

	if m, _, err := ParseTableCommand("/rit on", turnView()); err != nil || m.Type != "set_run_it_twice" || m.RunItTwice == nil || !*m.RunItTwice {
		t.Fatalf("rit on: %+v err=%v", m, err)
	}
	if _, _, err := ParseTableCommand("/rit maybe", turnView()); err == nil {
		t.Fatalf("rit maybe should error")
	}

	m, _, err := ParseTableCommand("/showcards 2", turnView())
	if err != nil || m.Type != "show_cards" || m.CardIndex == nil || *m.CardIndex != 1 {
		t.Fatalf("showcards 2: %+v err=%v", m, err)
	}
	if m, _, _ := ParseTableCommand("/showcards", turnView()); m.CardIndex != nil {
		t.Fatalf("showcards all should carry no card_index: %+v", m)
	}

	m, _, err = ParseTableCommand("/preselect call_any", turnView())
	if err != nil || m.Type != "preselect_action" || m.Action != "call_any" ||
		m.ExpectedSnapshotVersion != 42 || m.ExpectedHandId != "h-1" {
		t.Fatalf("preselect: %+v err=%v", m, err)
	}
	if m, _, _ := ParseTableCommand("/preselect off", turnView()); m.Action != "" {
		t.Fatalf("preselect off should clear the action: %+v", m)
	}
	if _, _, err := ParseTableCommand("/preselect bogus", turnView()); err == nil {
		t.Fatalf("preselect bogus should error")
	}
}

func TestParseSitoutAndReady(t *testing.T) {
	m, _, _ := ParseTableCommand("/sitout", turnView())
	if m.Type != "ready" || m.Ready {
		t.Fatalf("sitout: %+v", m)
	}
	m, _, _ = ParseTableCommand("/ready", turnView())
	if m.Type != "ready" || !m.Ready {
		t.Fatalf("ready: %+v", m)
	}
}

func TestParseLocalActions(t *testing.T) {
	for in, want := range map[string]LocalAction{
		"/help": ActHelp, "/summary": ActSummary, "/last-winners": ActLastWinners,
		"/share": ActShare, "/exit!": ActForceExit,
	} {
		_, local, err := ParseTableCommand(in, turnView())
		if err != nil || local != want {
			t.Errorf("%s: local=%v err=%v", in, local, err)
		}
	}
	m, local, _ := ParseTableCommand("/exit", turnView())
	if local != ActExit || m == nil || m.Type != "request_exit" {
		t.Fatalf("/exit: m=%v local=%v", m, local)
	}
	if m.ActionId == "" {
		t.Fatal("/exit request must carry an action id for acknowledgement correlation")
	}
}

func TestHotkeysOnlyOnYourTurn(t *testing.T) {
	notYours := turnView()
	notYours.IsYourTurn = false
	if _, _, err := ParseTableCommand("f", notYours); err == nil {
		t.Fatal("hotkey off-turn should error")
	}
	m, _, err := ParseTableCommand("f", turnView())
	if err != nil || m.Action != "fold" {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestUnknownCommand(t *testing.T) {
	if _, _, err := ParseTableCommand("/nope", turnView()); err == nil {
		t.Fatal("unknown command should error")
	}
}
