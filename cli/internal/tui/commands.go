package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gopkg.aoctech.app/poker/cli/internal/game"
	"gopkg.aoctech.app/poker/cli/internal/proto"
)

// LocalAction is a client-only effect a table command triggers (no
// ClientMessage sent).
type LocalAction int

const (
	ActNone LocalAction = iota
	ActHelp
	ActSummary
	ActLastWinners
	ActShare
	ActClear
	ActExit
	ActForceExit
)

// ParseTableCommand turns one input line (a `/command` or a bare hotkey) into
// either a ClientMessage to send, a LocalAction, or an error explaining why
// the command is not valid right now. Exactly one of msg / local is
// meaningful; local == ActNone with msg == nil and err == nil means "nothing
// to do" (e.g. an empty line).
func ParseTableCommand(input string, v game.TableView) (msg *proto.ClientMessage, local LocalAction, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, ActNone, nil
	}

	// Bare single-key hotkeys, only while it's the viewer's turn.
	if !strings.HasPrefix(input, "/") {
		if !v.IsYourTurn {
			return nil, ActNone, fmt.Errorf("não é sua vez")
		}
		switch input {
		case "f":
			return act("fold", 0, v), ActNone, nil
		case "c":
			return callOrCheck(v)
		case "r":
			if v.Legal == nil {
				return nil, ActNone, fmt.Errorf("nada para apostar agora")
			}
			return act("raise", v.Legal.MinRaiseTo, v), ActNone, nil
		case "p":
			return peek("all")
		case "k":
			return &proto.ClientMessage{Type: "peek_cards"}, ActNone, nil
		default:
			return nil, ActNone, fmt.Errorf("tecla desconhecida: %q", input)
		}
	}

	fields := strings.Fields(input)
	cmd, args := fields[0], fields[1:]

	switch cmd {
	case "/check":
		if !hasAction(v, "check") {
			return nil, ActNone, fmt.Errorf("não dá para dar check agora")
		}
		return act("check", 0, v), ActNone, nil
	case "/call":
		return callOrCheck(v)
	case "/fold":
		return act("fold", 0, v), ActNone, nil
	case "/raise":
		if len(args) != 1 {
			return nil, ActNone, fmt.Errorf("uso: /raise <valor>")
		}
		to, perr := strconv.ParseInt(args[0], 10, 64)
		if perr != nil {
			return nil, ActNone, fmt.Errorf("valor inválido: %s", args[0])
		}
		if v.Legal != nil && (to < v.Legal.MinRaiseTo || to > v.Legal.MaxRaiseTo) {
			return nil, ActNone, fmt.Errorf("raise deve ficar entre %d e %d", v.Legal.MinRaiseTo, v.Legal.MaxRaiseTo)
		}
		return act("raise", to, v), ActNone, nil
	case "/pot":
		if v.Legal == nil || v.Legal.PotRaiseTo == 0 {
			return nil, ActNone, fmt.Errorf("sem raise de pote disponível")
		}
		return act("raise", v.Legal.PotRaiseTo, v), ActNone, nil
	case "/allin":
		if v.Legal == nil {
			return nil, ActNone, fmt.Errorf("nada para apostar agora")
		}
		return act("raise", v.Legal.MaxRaiseTo, v), ActNone, nil
	case "/talk":
		text := strings.TrimSpace(strings.TrimPrefix(input, cmd))
		if text == "" {
			return nil, ActNone, fmt.Errorf("uso: /talk <mensagem>")
		}
		if len(text) > 50 {
			return nil, ActNone, fmt.Errorf("mensagem muito longa (máx 50)")
		}
		return &proto.ClientMessage{Type: "chat", Message: text}, ActNone, nil
	case "/react":
		if len(args) < 1 {
			return nil, ActNone, fmt.Errorf("uso: /react <código> [jogador]")
		}
		m := &proto.ClientMessage{Type: "reaction", ReactionId: args[0]}
		if len(args) >= 2 {
			m.TargetPlayerId = args[1]
		}
		return m, ActNone, nil
	case "/peek":
		which := "all"
		if len(args) == 1 {
			which = args[0]
		}
		return peek(which)
	case "/sitout":
		return &proto.ClientMessage{Type: "ready", Ready: false}, ActNone, nil
	case "/ready":
		return &proto.ClientMessage{Type: "ready", Ready: true}, ActNone, nil
	case "/summary":
		return nil, ActSummary, nil
	case "/last-winners":
		return nil, ActLastWinners, nil
	case "/share":
		return nil, ActShare, nil
	case "/clear":
		return nil, ActClear, nil
	case "/help":
		return nil, ActHelp, nil
	case "/exit":
		return &proto.ClientMessage{Type: "request_exit"}, ActExit, nil
	case "/exit!":
		return nil, ActForceExit, nil
	default:
		return nil, ActNone, fmt.Errorf("comando desconhecido: %s (tente /help)", cmd)
	}
}

func hasAction(v game.TableView, name string) bool {
	if v.Legal == nil {
		return false
	}
	for _, a := range v.Legal.Actions {
		if a == name {
			return true
		}
	}
	return false
}

func callOrCheck(v game.TableView) (*proto.ClientMessage, LocalAction, error) {
	if hasAction(v, "check") {
		return act("check", 0, v), ActNone, nil
	}
	if !hasAction(v, "call") {
		return nil, ActNone, fmt.Errorf("nada para pagar agora")
	}
	return act("call", v.Legal.CallAmount, v), ActNone, nil
}

func peek(which string) (*proto.ClientMessage, LocalAction, error) {
	m := &proto.ClientMessage{Type: "peek_cards"}
	switch which {
	case "all", "":
		// both cards — no card_index
	case "1":
		i := int32(0)
		m.CardIndex = &i
	case "2":
		i := int32(1)
		m.CardIndex = &i
	default:
		return nil, ActNone, fmt.Errorf("uso: /peek [all|1|2]")
	}
	return m, ActNone, nil
}

// act builds an `act` ClientMessage with a fresh action id and the
// optimistic-concurrency preconditions from v.
func act(action string, amount int64, v game.TableView) *proto.ClientMessage {
	return &proto.ClientMessage{
		Type:                    "act",
		Action:                  action,
		Amount:                  amount,
		ActionId:                uuid.NewString(),
		ExpectedSnapshotVersion: v.SnapshotVersion,
		ExpectedHandId:          v.HandID,
	}
}
