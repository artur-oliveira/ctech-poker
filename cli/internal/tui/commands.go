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
	ActPeekBoth
	ActPeekCard1
	ActPeekCard2
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

	// Bare single-key hotkeys.
	if !strings.HasPrefix(input, "/") {
		if input == "k" { // show/hide my hand — allowed any time
			return nil, ActPeekBoth, nil
		}
		if !v.IsYourTurn {
			return nil, ActNone, fmt.Errorf("não é sua vez")
		}
		switch input {
		case "f":
			return fold(v)
		case "c":
			return callOrCheck(v)
		case "r":
			return raiseTo(v.Legal, v.Legal.GetMinRaiseTo(), v)
		case "p":
			return pot(v)
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
		return fold(v)
	case "/raise":
		if len(args) != 1 {
			return nil, ActNone, fmt.Errorf("uso: /raise <valor>")
		}
		to, perr := strconv.ParseInt(args[0], 10, 64)
		if perr != nil {
			return nil, ActNone, fmt.Errorf("valor inválido: %s", args[0])
		}
		return raiseTo(v.Legal, to, v)
	case "/pot":
		return pot(v)
	case "/allin":
		if !hasAction(v, "raise") || v.Legal.MaxRaiseTo <= 0 {
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
		r, ok := game.LookupReaction(args[0])
		if !ok {
			return nil, ActNone, fmt.Errorf("reação desconhecida: %s (Tab lista as disponíveis)", args[0])
		}
		m := &proto.ClientMessage{Type: "reaction", ReactionId: r.ID}
		switch {
		case r.Targeted && len(args) < 2:
			return nil, ActNone, fmt.Errorf("a reação %q precisa de um jogador alvo", r.ID)
		case r.Targeted:
			id, terr := resolveTarget(v, strings.Join(args[1:], " "))
			if terr != nil {
				return nil, ActNone, terr
			}
			m.TargetPlayerId = id
		case len(args) > 1:
			return nil, ActNone, fmt.Errorf("a reação %q não tem alvo", r.ID)
		}
		return m, ActNone, nil
	case "/peek":
		which := "all"
		if len(args) == 1 {
			which = args[0]
		}
		switch which {
		case "all":
			return nil, ActPeekBoth, nil
		case "1":
			return nil, ActPeekCard1, nil
		case "2":
			return nil, ActPeekCard2, nil
		default:
			return nil, ActNone, fmt.Errorf("uso: /peek [all|1|2]")
		}
	case "/showcards":
		which := "all"
		if len(args) == 1 {
			which = args[0]
		}
		m := &proto.ClientMessage{Type: "show_cards", ActionId: uuid.NewString()}
		switch which {
		case "all":
		case "1":
			i := int32(0)
			m.CardIndex = &i
		case "2":
			i := int32(1)
			m.CardIndex = &i
		default:
			return nil, ActNone, fmt.Errorf("uso: /showcards [all|1|2]")
		}
		return m, ActNone, nil
	case "/rit":
		if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
			return nil, ActNone, fmt.Errorf("uso: /rit <on|off>")
		}
		enabled := args[0] == "on"
		return &proto.ClientMessage{Type: "set_run_it_twice", RunItTwice: &enabled}, ActNone, nil
	case "/rabbit":
		return &proto.ClientMessage{Type: "request_rabbit_hunt", ActionId: uuid.NewString()}, ActNone, nil
	case "/reqcards":
		return &proto.ClientMessage{Type: "request_winner_cards", ActionId: uuid.NewString()}, ActNone, nil
	case "/accept":
		return &proto.ClientMessage{Type: "accept_winner_cards", ActionId: uuid.NewString()}, ActNone, nil
	case "/decline":
		return &proto.ClientMessage{Type: "decline_winner_cards", ActionId: uuid.NewString()}, ActNone, nil
	case "/keep":
		return &proto.ClientMessage{Type: "keep_seat", ActionId: uuid.NewString()}, ActNone, nil
	case "/postbb":
		return &proto.ClientMessage{Type: "post_big_blind", ActionId: uuid.NewString()}, ActNone, nil
	case "/preselect":
		if len(args) != 1 {
			return nil, ActNone, fmt.Errorf("uso: /preselect <check_fold|fold|call|call_any|all_in|off>")
		}
		action := args[0]
		switch action {
		case "off", "none", "empty":
			action = ""
		case "check_fold", "fold", "call", "call_any", "all_in":
		default:
			return nil, ActNone, fmt.Errorf("modo inválido: %s", args[0])
		}
		m := &proto.ClientMessage{
			Type: "preselect_action", Action: action, ActionId: uuid.NewString(),
			ExpectedSnapshotVersion: v.SnapshotVersion,
			ExpectedHandId:          v.HandID,
			ExpectedStage:           v.Stage,
		}
		// ponytail: fixed-call amount left to the server; the CLI has no
		// viewer-scoped frozen call amount in the snapshot to echo.
		return m, ActNone, nil
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
		return &proto.ClientMessage{Type: "request_exit", ActionId: uuid.NewString()}, ActExit, nil
	case "/exit!":
		return nil, ActForceExit, nil
	default:
		return nil, ActNone, fmt.Errorf("comando desconhecido: %s (tente /help)", cmd)
	}
}

// resolveTarget maps a /react target token to a seat's player id. Accepts a
// raw player id, a case-insensitive name, or a position tag (UTG, CO, …), so
// the autocomplete can offer the readable name while the wire still carries
// the id. The viewer's own seat is never a valid target.
func resolveTarget(v game.TableView, q string) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", fmt.Errorf("faltou o jogador alvo")
	}
	for _, p := range v.Players {
		if p.ID == q && !p.IsYou {
			return p.ID, nil
		}
	}
	for _, p := range v.Players {
		if p.IsYou {
			continue
		}
		if strings.EqualFold(p.Name, q) || strings.EqualFold(p.Position, q) {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("jogador não encontrado na mesa: %s", q)
}

func fold(v game.TableView) (*proto.ClientMessage, LocalAction, error) {
	if !hasAction(v, "fold") {
		return nil, ActNone, fmt.Errorf("não dá para desistir agora")
	}
	return act("fold", 0, v), ActNone, nil
}

func raiseTo(legal *proto.LegalActions, to int64, v game.TableView) (*proto.ClientMessage, LocalAction, error) {
	if !hasAction(v, "raise") || legal == nil || legal.MinRaiseTo <= 0 || legal.MaxRaiseTo <= 0 {
		return nil, ActNone, fmt.Errorf("nada para aumentar agora")
	}
	if to < legal.MinRaiseTo || to > legal.MaxRaiseTo {
		return nil, ActNone, fmt.Errorf("aumento deve ficar entre %d e %d", legal.MinRaiseTo, legal.MaxRaiseTo)
	}
	return act("raise", to, v), ActNone, nil
}

func pot(v game.TableView) (*proto.ClientMessage, LocalAction, error) {
	if !hasAction(v, "raise") || v.Legal.PotRaiseTo <= 0 {
		return nil, ActNone, fmt.Errorf("aumento de pote indisponível agora")
	}
	return act("raise", v.Legal.PotRaiseTo, v), ActNone, nil
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
