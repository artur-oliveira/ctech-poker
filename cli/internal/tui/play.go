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

func joinOrCreate(rc *rest.Client, ps playState, amount int64) tea.Cmd {
	return func() tea.Msg {
		resp, err := rc.JoinOrCreate(context.Background(), rest.JoinOrCreateReq{
			CurrencyMode: "sandbox", SmallBlind: ps.smallBlind, BigBlind: ps.bigBlind,
			MaxSeats: ps.seats, Amount: amount, IdempotencyKey: newIdemKey(),
		})
		if err != nil {
			return joinedMsg{err: err}
		}
		return joinedMsg{roomID: resp.RoomID, blinds: [2]int64{ps.smallBlind, ps.bigBlind}}
	}
}

func joinRoom(rc *rest.Client, ps playState, amount int64) tea.Cmd {
	return func() tea.Msg {
		err := rc.JoinRoom(context.Background(), ps.roomID, rest.JoinReq{
			Amount: amount, ShareCode: ps.shareCode, IdempotencyKey: newIdemKey(),
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

func buyInHint(bb int64) string {
	return fmt.Sprintf("buy-in (%d–%d, múltiplo de %d)", bb*20, bb*100, bb)
}

func parseBuyIn(s string, bb int64) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("valor inválido")
	}
	if v < bb*20 || v > bb*100 {
		return 0, fmt.Errorf("fora do intervalo %d–%d", bb*20, bb*100)
	}
	if v%bb != 0 {
		return 0, fmt.Errorf("deve ser múltiplo de %d", bb)
	}
	return v, nil
}
