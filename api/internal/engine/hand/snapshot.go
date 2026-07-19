package hand

import "gopkg.aoctech.app/poker/api/internal/engine/deck"

// Snapshot is the wire-safe view of a Table for exactly one viewer. Building
// it here (not in a networking package) is what makes "never leak another
// player's hole cards" a single-source-of-truth guarantee instead of a
// convention every caller has to remember.
type Snapshot struct {
	Stage   string           `json:"stage"`
	Board   []string         `json:"board"`
	Seats   []SeatView       `json:"seats"`
	Payouts map[string]int64 `json:"payouts,omitempty"`
	Rake    int64            `json:"rake,omitempty"`
}

type SeatView struct {
	PlayerID    string   `json:"player_id"`
	Stack       int64    `json:"stack"`
	State       string   `json:"state"`
	Contributed int64    `json:"contributed"`
	HoleCards   []string `json:"hole_cards,omitempty"`
	Equity      *float64 `json:"equity,omitempty"`
}

var stageNames = map[Stage]string{
	WaitingForPlayers: "waiting_for_players",
	PreFlop:           "pre_flop",
	Flop:              "flop",
	Turn:              "turn",
	River:             "river",
	Showdown:          "showdown",
	Complete:          "complete",
}

var playerStateNames = map[PlayerState]string{
	Active:       "active",
	Folded:       "folded",
	AllIn:        "all_in",
	SittingOut:   "sitting_out",
	Disconnected: "disconnected",
	PendingEntry: "pending_entry",
}

var rankCodes = map[deck.Rank]byte{
	deck.Two: '2', deck.Three: '3', deck.Four: '4', deck.Five: '5', deck.Six: '6',
	deck.Seven: '7', deck.Eight: '8', deck.Nine: '9', deck.Ten: 'T',
	deck.Jack: 'J', deck.Queen: 'Q', deck.King: 'K', deck.Ace: 'A',
}

var suitCodes = map[deck.Suit]byte{
	deck.Clubs: 'c', deck.Diamonds: 'd', deck.Hearts: 'h', deck.Spades: 's',
}

func cardCode(c deck.Card) string {
	return string([]byte{rankCodes[c.Rank], suitCodes[c.Suit]})
}

func boardCodes(board []deck.Card) []string {
	out := make([]string, len(board))
	for i, c := range board {
		out[i] = cardCode(c)
	}
	return out
}

// ViewFor builds the snapshot viewerID is allowed to see: their own hole
// cards always visible; every other seat's hole cards hidden until the hand
// reaches Complete, at which point every non-folded hand was shown at
// showdown and is safe to reveal to everyone (folded hands are never
// revealed — a folded player's cards were never part of the showdown).
func (t *Table) ViewFor(viewerID string) Snapshot {
	seats := make([]SeatView, 0, len(t.players))
	revealAll := t.stage == Complete
	for _, p := range t.players {
		sv := SeatView{
			PlayerID:    p.ID,
			Stack:       p.Stack,
			State:       playerStateNames[p.State],
			Contributed: p.Contributed,
		}
		if p.ID == viewerID || (revealAll && p.State != Folded) {
			sv.HoleCards = []string{cardCode(p.HoleCards[0]), cardCode(p.HoleCards[1])}
		}
		seats = append(seats, sv)
	}
	return Snapshot{
		Stage:   stageNames[t.stage],
		Board:   boardCodes(t.board),
		Seats:   seats,
		Payouts: t.payouts,
		Rake:    t.rakeCollected,
	}
}

// playerToActForTest returns the ID of whichever player currentPlayerCanAct
// reports true for — test-only helper so snapshot_test.go can drive a hand to
// completion without hardcoding seat order (which depends on
// dealerIndexWithin).
func (t *Table) playerToActForTest() string {
	for _, p := range t.players {
		if t.currentPlayerCanAct(p.ID) {
			return p.ID
		}
	}
	return ""
}
