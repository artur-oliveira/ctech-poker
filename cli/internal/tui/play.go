package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.aoctech.app/poker/cli/internal/proto"
	"gopkg.aoctech.app/poker/cli/internal/rest"
	"gopkg.aoctech.app/poker/cli/internal/wsclient"
)

// play-flow shell states (offset so they never collide with the base states).
const (
	statePlaySize shellState = iota + 100
	statePlayStake
	statePlayBuyin
	statePlayAutoRebuy
	stateJoining
	stateInTable
)

// tableSizes are the /play "table size" choices.
var tableSizes = []struct {
	label string
	seats int
}{
	{"Heads-up (2)", 2},
	{"6-Max (6)", 6},
	{"Full Ring (9)", 9},
}

// playState holds the in-progress /play or /enter selection.
type playState struct {
	fromEnter  bool
	seats      int
	stakes     []rest.Stake
	stakeIdx   int
	sizeIdx    int
	smallBlind int64
	bigBlind   int64
	buyIn      int64
	autoRebuy  bool
	roomID     string
	shareCode  string
}

// --- async messages ---

type stakesLoadedMsg struct {
	stakes []rest.Stake
	err    error
}
type roomLoadedMsg struct {
	room rest.Room
	err  error
}
type joinedMsg struct {
	roomID    string
	shareCode string
	blinds    [2]int64
	err       error
}
type wsConnectedMsg struct {
	client *wsclient.Client
	err    error
}
type wsStreamMsg struct{ m *proto.ServerMessage }
type wsClosedMsg struct{ err error }

func loadStakes(rc *rest.Client) tea.Cmd {
	return func() tea.Msg {
		s, err := rc.Stakes(context.Background(), "sandbox")
		return stakesLoadedMsg{stakes: s, err: err}
	}
}

func loadRoom(rc *rest.Client, id string) tea.Cmd {
	return func() tea.Msg {
		r, err := rc.Room(context.Background(), id)
		return roomLoadedMsg{room: r, err: err}
	}
}

func joinOrCreate(rc *rest.Client, ps playState) tea.Cmd {
	return func() tea.Msg {
		resp, err := rc.JoinOrCreate(context.Background(), rest.JoinOrCreateReq{
			CurrencyMode: "sandbox", SmallBlind: ps.smallBlind, BigBlind: ps.bigBlind,
			MaxSeats: ps.seats, Amount: ps.buyIn, AutoRebuy: ps.autoRebuy,
			IdempotencyKey: newIdemKey(),
		})
		if err != nil {
			return joinedMsg{err: err}
		}
		return joinedMsg{roomID: resp.RoomID, blinds: [2]int64{ps.smallBlind, ps.bigBlind}}
	}
}

func joinRoom(rc *rest.Client, ps playState) tea.Cmd {
	return func() tea.Msg {
		err := rc.JoinRoom(context.Background(), ps.roomID, rest.JoinReq{
			Amount: ps.buyIn, AutoRebuy: ps.autoRebuy, ShareCode: ps.shareCode,
			IdempotencyKey: newIdemKey(),
		})
		if err != nil {
			return joinedMsg{err: err}
		}
		return joinedMsg{roomID: ps.roomID, shareCode: ps.shareCode, blinds: [2]int64{ps.smallBlind, ps.bigBlind}}
	}
}

func connectTable(apiBaseURL string, token func(context.Context) (string, error), roomID, shareCode string) tea.Cmd {
	return func() tea.Msg {
		cl := wsclient.New(wsURLFor(apiBaseURL, roomID), token, rest.OriginHeader)
		if err := cl.Connect(context.Background(), shareCode); err != nil {
			return wsConnectedMsg{err: err}
		}
		return wsConnectedMsg{client: cl}
	}
}

// pumpTable blocks on the next ServerMessage and turns it into a shell
// message; it re-schedules itself from the shell's Update.
func pumpTable(cl *wsclient.Client) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-cl.Messages()
		if !ok {
			return wsClosedMsg{err: cl.Err()}
		}
		switch m.Type {
		case wsclient.TypeReconnecting:
			return ReconnectingMsg{}
		case wsclient.TypeReconnected:
			return ReconnectedMsg{}
		}
		return wsStreamMsg{m: m}
	}
}

func wsURLFor(apiBaseURL, roomID string) string {
	u := strings.Replace(apiBaseURL, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	return strings.TrimRight(u, "/") + "/v1.0/tables/" + roomID + "/ws"
}

func newIdemKey() string {
	return "cli-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// buyInRange is the client-side window (20–100 big blinds), mirroring the
// server's publicBuyInMin/MaxBigBlinds. The server clamps anyway.
func buyInRange(bb int64) (min, max int64) { return bb * 20, bb * 100 }

// defaultBuyIn is the value the prompt is pre-filled with: a full 100-BB
// stack, the standard buy-in, so a player who just wants to sit down can
// press Enter without typing anything.
func defaultBuyIn(bb int64) int64 { return bb * 100 }

func buyInHint(bb int64) string {
	min, max := buyInRange(bb)
	return fmt.Sprintf("Quantas fichas levar para a mesa? Entre %d e %d, múltiplo de %d.", min, max, bb)
}

func parseBuyIn(s string, bb int64) (int64, error) {
	// Tolerate a stray "buy-in" or "fichas" the user may have typed around
	// the number after reading the label — take the first run of digits.
	v, err := strconv.ParseInt(firstDigitRun(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("digite só um número, ex: %d", defaultBuyIn(bb))
	}
	min, max := buyInRange(bb)
	if v < min || v > max {
		return 0, fmt.Errorf("precisa ficar entre %d e %d", min, max)
	}
	if v%bb != 0 {
		return 0, fmt.Errorf("precisa ser múltiplo de %d", bb)
	}
	return v, nil
}

// firstDigitRun returns the first contiguous sequence of ASCII digits in s,
// or "" if there is none.
func firstDigitRun(s string) string {
	start := -1
	for i, r := range s {
		if r >= '0' && r <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			return s[start:i]
		}
	}
	if start >= 0 {
		return s[start:]
	}
	return ""
}
