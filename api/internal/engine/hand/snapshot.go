package hand

import "gopkg.aoctech.app/poker/api/internal/engine/deck"

// Snapshot is the wire-safe view of a Table for exactly one viewer. Building
// it here (not in a networking package) is what makes "never leak another
// player's hole cards" a single-source-of-truth guarantee instead of a
// convention every caller has to remember.
type Snapshot struct {
	Stage                string           `json:"stage"`
	Board                []string         `json:"board"`
	Seats                []SeatView       `json:"seats"`
	Payouts              map[string]int64 `json:"payouts,omitempty"`
	Rake                 int64            `json:"rake,omitempty"`
	CurrentPlayerID      string           `json:"current_player_id,omitempty"`
	LegalActions         *LegalActions    `json:"legal_actions,omitempty"`
	ActionDeadlineUnixMs int64            `json:"action_deadline_unix_ms,omitempty"`
	NextHandUnixMs       int64            `json:"next_hand_unix_ms,omitempty"`
}

// LegalActions is the authoritative set of moves the viewer may make right
// now, with the chip math the UI needs to render the raise control. The server
// is the single source of truth — the client must not derive these itself.
type LegalActions struct {
	Actions    []string `json:"actions"`      // subset of fold|check|call|raise
	CallAmount int64    `json:"call_amount"`  // chips owed to call (0 when a check is available)
	MinRaiseTo int64    `json:"min_raise_to"` // smallest total bet a raise may reach
	MaxRaiseTo int64    `json:"max_raise_to"` // largest total bet (all-in): viewer stack + already contributed
	Step       int64    `json:"step"`         // raise increment for the + / - stepper
}

type SeatView struct {
	PlayerID    string   `json:"player_id"`
	Name        string   `json:"name,omitempty"`
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
// reaches Complete via a genuine showdown, at which point every non-folded
// hand was shown and is safe to reveal to everyone (folded hands are never
// revealed — a folded player's cards were never part of the showdown). A
// hand that ends because every other player folded has no showdown at all,
// so the lone remaining player's cards stay hidden too.
func (t *Table) ViewFor(viewerID string) Snapshot {
	seats := make([]SeatView, 0, len(t.players))
	revealAll := t.stage == Complete && t.lastOutcome != nil && !t.lastOutcome.WonWithoutShowdown
	// Only players actually dealt into the current/last hand have real
	// HoleCards — anyone else (waiting for the first hand, or a mid-hand
	// joiner seated as PendingEntry) still holds deck.Card{}'s zero value,
	// which cardCode would render as a bogus "\x00c" card.
	dealtIn := make(map[string]bool, len(t.handOrder))
	for _, hp := range t.handOrder {
		dealtIn[hp.ID] = true
	}
	for _, p := range t.players {
		sv := SeatView{
			PlayerID:    p.ID,
			Stack:       p.Stack,
			State:       playerStateNames[p.State],
			Contributed: p.Contributed,
		}
		if dealtIn[p.ID] && (p.ID == viewerID || (revealAll && p.State != Folded)) {
			sv.HoleCards = []string{cardCode(p.HoleCards[0]), cardCode(p.HoleCards[1])}
		}
		seats = append(seats, sv)
	}
	current := t.currentPlayerToAct()
	return Snapshot{
		Stage:           stageNames[t.stage],
		Board:           boardCodes(t.board),
		Seats:           seats,
		Payouts:         t.payouts,
		Rake:            t.rakeCollected,
		CurrentPlayerID: current,
		LegalActions:    t.legalActionsFor(viewerID, current),
	}
}

// isBettingStage reports whether the hand is in a street where a player may
// act (waiting/complete/showdown are not).
func isBettingStage(s Stage) bool {
	return s == PreFlop || s == Flop || s == Turn || s == River
}

// currentPlayerToAct returns the ID of the single player who must act now, or
// "" when no decision is pending (waiting, complete, or between stages).
func (t *Table) currentPlayerToAct() string {
	if !isBettingStage(t.stage) || t.round == nil {
		return ""
	}
	for _, p := range t.players {
		if t.currentPlayerCanAct(p.ID) {
			return p.ID
		}
	}
	return ""
}

// legalActionsFor returns the authoritative moves viewerID may make given the
// current round. It is only populated on the viewer's actual turn during a
// betting street; otherwise it is an empty (but present) structure during a
// betting street and nil between hands, so the client never falls back to its
// own (non-authoritative) legality guess.
func (t *Table) legalActionsFor(viewerID, current string) *LegalActions {
	if !isBettingStage(t.stage) || t.round == nil {
		return nil
	}
	if current != viewerID {
		return &LegalActions{}
	}
	idx, ok := t.roundIdx[viewerID]
	if !ok {
		return &LegalActions{}
	}
	bs := t.round.Players[idx]
	if bs.Folded || bs.AllIn {
		return &LegalActions{}
	}
	la := &LegalActions{Actions: []string{"fold"}}
	owed := t.round.CurrentBet - bs.Contributed
	if owed <= 0 {
		la.Actions = append(la.Actions, "check")
	} else {
		la.Actions = append(la.Actions, "call")
		la.CallAmount = owed
	}
	// A raise is available only if the viewer has not yet acted since the last
	// full raise AND still has enough chips to exceed the current bet.
	canRaise := !bs.ActedSinceLastFullRaise && bs.Contributed+bs.Stack > t.round.CurrentBet
	if canRaise {
		la.Actions = append(la.Actions, "raise")
		minRaiseTo := t.round.CurrentBet + t.round.MinRaise
		if minRaiseTo <= t.round.CurrentBet {
			minRaiseTo = t.round.CurrentBet + t.bigBlind
		}
		maxTo := bs.Contributed + bs.Stack
		if minRaiseTo > maxTo {
			minRaiseTo = maxTo
		}
		la.MinRaiseTo = minRaiseTo
		la.MaxRaiseTo = maxTo
		la.Step = t.round.MinRaise
		if la.Step <= 0 {
			la.Step = t.bigBlind
		}
	}
	return la
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
