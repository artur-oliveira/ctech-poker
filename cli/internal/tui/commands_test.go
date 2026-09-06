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

func TestParseReactWithTarget(t *testing.T) {
	m, _, err := ParseTableCommand("/react gg caio", turnView())
	if err != nil || m.Type != "reaction" || m.ReactionId != "gg" || m.TargetPlayerId != "caio" {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestParsePeekVariants(t *testing.T) {
	m, _, _ := ParseTableCommand("/peek", turnView())
	if m.Type != "peek_cards" || m.CardIndex != nil {
		t.Fatalf("peek all: %+v", m)
	}
	m, _, _ = ParseTableCommand("/peek 2", turnView())
	if m.CardIndex == nil || *m.CardIndex != 1 {
		t.Fatalf("peek 2: %+v", m)
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
