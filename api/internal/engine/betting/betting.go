// Package betting implements one betting round's action rules (OVERVIEW.md
// § 3.3), most importantly: minimum raise sizing, and the rule that a short
// (sub-minimum) all-in does not reopen raising for players who already acted.
package betting

import "fmt"

type Action string

const (
	ActionFold  Action = "fold"
	ActionCheck Action = "check"
	ActionCall  Action = "call"
	ActionBet   Action = "bet"
	ActionRaise Action = "raise"
)

// PlayerState tracks one player's standing within a single betting round.
// ActedSinceLastFullRaise means "has responded to the current bet level
// since the last full raise" — it gates both round-completion and whether
// this player may raise again (a short all-in never resets it to false for
// players who already had it true).
type PlayerState struct {
	ID                      string
	Stack                   int64
	Contributed             int64
	Folded                  bool
	AllIn                   bool
	ActedSinceLastFullRaise bool
}

// Round tracks one betting round (pre-flop/flop/turn/river) across all
// players still dealt into the hand.
type Round struct {
	Players    []*PlayerState
	CurrentBet int64
	MinRaise   int64
	// LastActorID is the durable cursor used by the hand engine to continue
	// clockwise from the player who just acted. It matters after a full raise:
	// that raise reopens earlier players, but must not let them jump ahead of
	// seats later in the same orbit that have not responded yet.
	LastActorID string
}

// NewRound starts a betting round. currentBet/minRaise seed initial state —
// e.g. post-flop rounds start at (0, bigBlind); pre-flop starts at
// (bigBlindAmount, bigBlindAmount) since the blinds are already posted.
func NewRound(players []*PlayerState, currentBet, minRaise int64) *Round {
	return &Round{Players: players, CurrentBet: currentBet, MinRaise: minRaise}
}

// Act applies one player's action. amount is the TOTAL chips this player
// will have contributed this round after a Bet/Raise/Call (not the delta).
func (r *Round) Act(playerIdx int, action Action, amount int64) error {
	if playerIdx < 0 || playerIdx >= len(r.Players) {
		return fmt.Errorf("betting: invalid player index %d", playerIdx)
	}
	p := r.Players[playerIdx]
	if p.Folded || p.AllIn {
		return fmt.Errorf("betting: player %s cannot act (folded=%v allIn=%v)", p.ID, p.Folded, p.AllIn)
	}

	switch action {
	case ActionFold:
		p.Folded = true
		r.LastActorID = p.ID
		return nil

	case ActionCheck:
		if p.Contributed < r.CurrentBet {
			return fmt.Errorf("betting: player %s must call or fold, cannot check (owes %d)", p.ID, r.CurrentBet-p.Contributed)
		}
		p.ActedSinceLastFullRaise = true
		r.LastActorID = p.ID
		return nil

	case ActionCall:
		owed := r.CurrentBet - p.Contributed
		if owed <= 0 {
			return fmt.Errorf("betting: player %s has nothing to call", p.ID)
		}
		if owed >= p.Stack {
			p.Contributed += p.Stack
			p.Stack = 0
			p.AllIn = true
		} else {
			p.Stack -= owed
			p.Contributed += owed
		}
		p.ActedSinceLastFullRaise = true
		r.LastActorID = p.ID
		return nil

	case ActionBet, ActionRaise:
		if p.ActedSinceLastFullRaise {
			return fmt.Errorf("betting: player %s already acted and no full raise has reopened action — may only call or fold", p.ID)
		}
		if amount <= r.CurrentBet {
			return fmt.Errorf("betting: raise amount %d must exceed current bet %d", amount, r.CurrentBet)
		}
		delta := amount - p.Contributed
		if delta <= 0 {
			return fmt.Errorf("betting: raise amount %d must exceed player contribution %d", amount, p.Contributed)
		}
		goingAllIn := delta >= p.Stack
		if goingAllIn {
			delta = p.Stack
			amount = p.Contributed + delta
		}
		// Validate and classify the raise from the amount the player can
		// actually put in, never the untrusted target supplied by the client.
		// Otherwise amount=1_000_000 with a 150-chip stack turns a +50 short
		// all-in into a fabricated full raise that reopens action.
		raiseSize := amount - r.CurrentBet
		if raiseSize <= 0 {
			return fmt.Errorf("betting: all-in amount %d must exceed current bet %d", amount, r.CurrentBet)
		}
		if raiseSize < r.MinRaise && !goingAllIn {
			return fmt.Errorf("betting: raise size %d below minimum raise %d", raiseSize, r.MinRaise)
		}
		if goingAllIn {
			p.AllIn = true
		}
		p.Stack -= delta
		p.Contributed += delta
		r.CurrentBet = amount

		isFullRaise := raiseSize >= r.MinRaise
		if isFullRaise {
			r.MinRaise = raiseSize
			for _, other := range r.Players {
				if other != p {
					other.ActedSinceLastFullRaise = false
				}
			}
		}
		p.ActedSinceLastFullRaise = true
		r.LastActorID = p.ID
		return nil

	default:
		return fmt.Errorf("betting: unknown action %q", action)
	}
}

// IsComplete reports whether every player still in the hand (not folded, not
// all-in) has acted since the last full raise and matches CurrentBet.
func (r *Round) IsComplete() bool {
	notFolded := 0
	allMatched := true
	for _, p := range r.Players {
		if p.Folded {
			continue
		}
		notFolded++
		if !p.AllIn && (!p.ActedSinceLastFullRaise || p.Contributed != r.CurrentBet) {
			allMatched = false
		}
	}
	return notFolded <= 1 || allMatched
}
