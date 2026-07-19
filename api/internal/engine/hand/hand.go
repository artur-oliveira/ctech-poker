// Package hand orchestrates one table's full hand lifecycle (OVERVIEW.md
// § 3.1), tying together deck shuffling (Task 5), hand evaluation (Task 6),
// side pots (Task 7), and betting rounds (Task 8). Pure logic — no
// networking, no persistence; Phase 2 wires this to a live table server.
package hand

import (
	"fmt"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
	"gopkg.aoctech.app/poker/api/internal/engine/sidepots"
)

type PlayerState uint8

const (
	Active PlayerState = iota
	Folded
	AllIn
	SittingOut
	Disconnected
	PendingEntry
)

type Stage uint8

const (
	WaitingForPlayers Stage = iota
	PreFlop
	Flop
	Turn
	River
	Showdown
	Complete
)

type Player struct {
	ID          string
	Stack       int64
	Ready       bool
	State       PlayerState
	HoleCards   [2]deck.Card
	Contributed int64 // this hand's total contribution across all rounds, for side-pot math
}

type Table struct {
	players    []*Player
	smallBlind int64
	bigBlind   int64
	dealerSeat int
	stage      Stage
	board      []deck.Card
	shuffle    *deck.ShuffleResult
	nextCard   int
	round      *betting.Round
	roundIdx   map[string]int // playerID -> index into round.Players, for the active betting round

	// roundBaseline records, for each player in the current round, the value
	// round.Players[idx].Contributed held at the moment this round began
	// (0 for a fresh post-flop street; the blind just posted for the two
	// blind seats in the pre-flop round — see seedRoundContribution). Act
	// uses it to compute how much NEW money a player put in since the last
	// time we folded their round contribution into Player.Contributed, so a
	// player acting more than once in the same round (e.g. bet then facing a
	// re-raise) doesn't get double-counted. See Act's doc comment.
	roundBaseline map[string]int64

	payouts map[string]int64

	// handOrder is the seat order of players dealt into the current (or
	// most recently completed) hand — the same slice built as `active` in
	// StartHand. Used at hand-end to rotate dealerSeat forward to the next
	// player who actually played this hand, regardless of PendingEntry
	// joiners appended to t.players since.
	handOrder []*Player

	// seenActionIDs de-dupes ActIdempotent calls by client-supplied
	// action_id within the current hand (OVERVIEW.md § 4) — persisted as
	// part of State (state.go) so any instance recovering mid-hand still
	// rejects a replayed duplicate, not just the instance that originally
	// saw it.
	seenActionIDs map[string]bool
}

func NewTable(players []*Player, smallBlind, bigBlind int64) *Table {
	return &Table{
		players:    players,
		smallBlind: smallBlind,
		bigBlind:   bigBlind,
		stage:      WaitingForPlayers,
	}
}

func (t *Table) Stage() Stage { return t.stage }

func (t *Table) Payouts() map[string]int64 { return t.payouts }

// PlayersForActor exposes the live player slice for Phase 2's table.Actor,
// which needs to toggle Ready before a hand starts (StartHand only reads it,
// nothing in this package previously needed to write it from outside).
func (t *Table) PlayersForActor() []*Player { return t.players }

func (t *Table) playerByID(id string) *Player {
	for _, p := range t.players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// AddMidHandJoiner seats a new player as PendingEntry (OVERVIEW.md § 2) — not
// dealt in until the hand in progress completes, and required to post the
// big blind on the hand they're first dealt into (handled in StartHand).
func (t *Table) AddMidHandJoiner(p *Player) {
	p.State = PendingEntry
	t.players = append(t.players, p)
}

// StartHand begins a new hand: requires >=2 ready players, posts blinds
// relative to dealerSeat (heads-up special case: dealer posts small blind),
// shuffles via commit-reveal, and deals hole cards. dealerSeat itself is
// rotated forward to the next seat at the END of each hand (see
// rotateDealer, called from runShowdown) so the SECOND and later calls to
// StartHand on the same Table use a new dealer. The very first hand's button
// is NOT drawn by CSPRNG — it defaults to seat 0 (dealerSeat's zero value);
// only rotation across hands is implemented.
func (t *Table) StartHand() error {
	readyCount := 0
	for _, p := range t.players {
		if p.State != PendingEntry && p.Ready {
			readyCount++
		}
	}
	if readyCount < 2 {
		return fmt.Errorf("hand: need at least 2 ready players, have %d", readyCount)
	}

	shuffle, err := deck.NewShuffle()
	if err != nil {
		return fmt.Errorf("hand: shuffle: %w", err)
	}
	t.shuffle = shuffle
	t.nextCard = 0
	t.board = nil
	t.payouts = nil
	t.seenActionIDs = make(map[string]bool)

	active := make([]*Player, 0, len(t.players))
	for _, p := range t.players {
		if p.State == PendingEntry {
			continue // sits out until this hand completes
		}
		p.State = Active
		p.Contributed = 0
		p.HoleCards = [2]deck.Card{t.dealCard(), t.dealCard()}
		active = append(active, p)
	}

	t.handOrder = active
	sbSeat, bbSeat := t.blindSeats(active)
	t.postBlind(active[sbSeat], t.smallBlind)
	t.postBlind(active[bbSeat], t.bigBlind)

	t.startBettingRound(active, t.bigBlind, t.bigBlind)
	// The blinds were posted onto Player.Contributed before the round
	// existed; seed the round's own per-player Contributed (and the
	// baseline that gates Act's bookkeeping) so Check/Call math sees them
	// as already-in-this-street money instead of demanding it again.
	t.seedRoundContribution(active[sbSeat].ID, active[sbSeat].Contributed)
	t.seedRoundContribution(active[bbSeat].ID, active[bbSeat].Contributed)
	t.stage = PreFlop
	return nil
}

// blindSeats returns (smallBlindIdx, bigBlindIdx) as indices into active,
// computed relative to dealerSeat's position within active. Heads-up is a
// special case: the dealer posts the small blind. 3+-way: the two seats
// clockwise after the dealer post small and big blind respectively.
func (t *Table) blindSeats(active []*Player) (sb, bb int) {
	dealerIdx := t.dealerIndexWithin(active)
	numActive := len(active)
	if numActive == 2 {
		return dealerIdx, (dealerIdx + 1) % numActive
	}
	return (dealerIdx + 1) % numActive, (dealerIdx + 2) % numActive
}

// dealerIndexWithin returns dealerSeat's player's position within list (by
// pointer identity), defaulting to 0 if the dealer isn't present in list —
// which also covers dealerSeat's zero value before the very first hand ever
// sets it (seat 0 is the default first dealer).
func (t *Table) dealerIndexWithin(list []*Player) int {
	if t.dealerSeat < 0 || t.dealerSeat >= len(t.players) {
		return 0
	}
	dealer := t.players[t.dealerSeat]
	for i, p := range list {
		if p == dealer {
			return i
		}
	}
	return 0
}

// rotateDealer advances dealerSeat to the next player (by table seat index)
// who was actually dealt into the hand that just completed, wrapping around.
// Called once a hand reaches Complete so the next StartHand call uses a new
// dealer.
func (t *Table) rotateDealer() {
	if len(t.handOrder) == 0 {
		return
	}
	idx := t.dealerIndexWithin(t.handOrder)
	next := t.handOrder[(idx+1)%len(t.handOrder)]
	for i, p := range t.players {
		if p == next {
			t.dealerSeat = i
			return
		}
	}
}

func (t *Table) postBlind(p *Player, amount int64) {
	if amount >= p.Stack {
		amount = p.Stack
		p.State = AllIn
	}
	p.Stack -= amount
	p.Contributed += amount
}

func (t *Table) dealCard() deck.Card {
	c := t.shuffle.Cards[t.nextCard]
	t.nextCard++
	return c
}

func (t *Table) startBettingRound(active []*Player, currentBet, minRaise int64) {
	states := make([]*betting.PlayerState, 0, len(active))
	roundIdx := make(map[string]int, len(active))
	for _, p := range active {
		if p.State == Folded {
			continue
		}
		bs := &betting.PlayerState{
			ID:    p.ID,
			Stack: p.Stack,
			AllIn: p.State == AllIn,
		}
		roundIdx[p.ID] = len(states)
		states = append(states, bs)
	}
	t.round = betting.NewRound(states, currentBet, minRaise)
	t.roundIdx = roundIdx
	t.roundBaseline = make(map[string]int64, len(states))
	for _, bs := range states {
		t.roundBaseline[bs.ID] = bs.Contributed // 0 for a fresh street
	}
}

// seedRoundContribution reflects a blind already posted (before this round
// existed) onto the just-started round's per-player state, and moves the
// bookkeeping baseline to match so Act doesn't re-count it as new money.
func (t *Table) seedRoundContribution(playerID string, amount int64) {
	idx, ok := t.roundIdx[playerID]
	if !ok {
		return
	}
	t.round.Players[idx].Contributed = amount
	t.roundBaseline[playerID] = amount
}

// currentPlayerCanAct reports whether id still has a decision to make in the
// current betting round (used by callers driving the hand to know who to
// prompt — Task 10's CLI harness and, later, Phase 2's table server).
func (t *Table) currentPlayerCanAct(id string) bool {
	idx, ok := t.roundIdx[id]
	if !ok {
		return false
	}
	bs := t.round.Players[idx]
	return !bs.Folded && !bs.AllIn && (!bs.ActedSinceLastFullRaise || bs.Contributed != t.round.CurrentBet)
}

// Act applies one player's betting action, then advances the stage if the
// round is complete.
//
// Two callers-facing actions get normalized before being handed to
// betting.Round, whose Act enforces strict poker semantics that don't map
// 1:1 onto how a caller naturally describes an all-in for less:
//
//   - A raise/bet whose target amount can't reach the round's CurrentBet even
//     by shoving the player's whole remaining stack isn't a "raise" under
//     betting.Round's model (which requires amount > CurrentBet) — it's a
//     short all-in call. Redirect it to Call so it isn't rejected.
//   - A call issued when the player already owes nothing (Contributed already
//     equals CurrentBet — can happen after a short all-in leaves the bet
//     level unchanged for someone who already matched it) is really a check.
//     Redirect it to Check so it correctly records the action instead of
//     erroring on "nothing to call".
//
// Both redirects are silent — the return value gives no signal that the
// requested action was reinterpreted. Both conditions depend only on chip
// totals already fixed before the call (never on the amount argument), so
// they can never misfire on a genuine client mistake: a real raise attempt
// that could still reach CurrentBet, or a real call where money is actually
// owed, always passes through unchanged and still surfaces betting.Round's
// own error. A caller that wants to know when its literal intent (Raise vs.
// Call, Call vs. Check) was reinterpreted must diff the action it requested
// against the resulting PlayerState itself — Act does not report it.
func (t *Table) Act(playerID string, action betting.Action, amount int64) error {
	idx, ok := t.roundIdx[playerID]
	if !ok {
		return fmt.Errorf("hand: player %s has no pending action this round", playerID)
	}
	bs := t.round.Players[idx]

	if (action == betting.ActionRaise || action == betting.ActionBet) && bs.Contributed+bs.Stack <= t.round.CurrentBet {
		action = betting.ActionCall
	}
	if action == betting.ActionCall && bs.Contributed >= t.round.CurrentBet {
		action = betting.ActionCheck
	}

	if err := t.round.Act(idx, action, amount); err != nil {
		return err
	}

	p := t.playerByID(playerID)
	if action == betting.ActionFold {
		p.State = Folded
	}
	if bs.AllIn {
		p.State = AllIn
	}
	p.Stack = bs.Stack
	// bs.Contributed is this round's cumulative total for this player (it
	// never resets between Act calls within the same round — see
	// betting.Round.Act). Player.Contributed is this HAND's cumulative
	// total across all rounds. The delta since the last time we folded this
	// round's progress in is exactly the new money this action added;
	// roundBaseline tracks "last time" per player so a player acting twice
	// in one round (bet, then called back over a raise) isn't double
	// counted, and so blinds seeded into bs.Contributed pre-round aren't
	// re-added on top of what postBlind already put in Player.Contributed.
	p.Contributed += bs.Contributed - t.roundBaseline[playerID]
	t.roundBaseline[playerID] = bs.Contributed

	if t.round.IsComplete() {
		t.advanceStage()
	}
	return nil
}

func (t *Table) advanceStage() {
	remaining := 0
	canStillAct := 0
	for _, p := range t.players {
		if p.State == Active || p.State == AllIn {
			remaining++
			if p.State == Active {
				canStillAct++
			}
		}
	}
	if remaining <= 1 {
		t.runShowdown()
		return
	}
	// Two or more players are still in the hand, but at most one of them is
	// NOT all-in — e.g. two players shoved pre-flop and everyone else folded
	// or called all-in too. There's nobody left who could call Act to ever
	// complete another betting round (a lone non-all-in player has no one to
	// bet against), so dealing the next street and calling startBettingRound
	// would hang the hand forever. Deal out the rest of the board in one go
	// and go straight to showdown, same as a real all-in runout.
	if canStillAct <= 1 {
		t.runoutBoard()
		t.runShowdown()
		return
	}

	switch t.stage {
	case PreFlop:
		t.board = append(t.board, t.dealCard(), t.dealCard(), t.dealCard())
		t.stage = Flop
	case Flop:
		t.board = append(t.board, t.dealCard())
		t.stage = Turn
	case Turn:
		t.board = append(t.board, t.dealCard())
		t.stage = River
	case River:
		t.runShowdown()
		return
	}
	t.startBettingRound(t.activePlayers(), 0, t.bigBlind)
}

// runoutBoard deals every remaining community card, from the current stage
// through the river, without starting a betting round. Used once at most one
// player can still act — there's nothing left to bet on, just cards left to
// reveal before showdown.
func (t *Table) runoutBoard() {
	for t.stage != River {
		switch t.stage {
		case PreFlop:
			t.board = append(t.board, t.dealCard(), t.dealCard(), t.dealCard())
			t.stage = Flop
		case Flop:
			t.board = append(t.board, t.dealCard())
			t.stage = Turn
		case Turn:
			t.board = append(t.board, t.dealCard())
			t.stage = River
		default:
			return
		}
	}
}

func (t *Table) activePlayers() []*Player {
	out := make([]*Player, 0, len(t.players))
	for _, p := range t.players {
		if p.State == Active || p.State == AllIn {
			out = append(out, p)
		}
	}
	return out
}

func (t *Table) runShowdown() {
	t.stage = Showdown
	contributions := make([]sidepots.Contribution, 0, len(t.players))
	for _, p := range t.players {
		if p.Contributed > 0 {
			contributions = append(contributions, sidepots.Contribution{PlayerID: p.ID, Amount: p.Contributed})
		}
	}
	layers := sidepots.ComputeSidePots(contributions)

	payouts := make(map[string]int64)
	for _, layer := range layers {
		var winners []string
		var bestScore handeval.Score
		for _, id := range layer.Eligible {
			p := t.playerByID(id)
			if p.State == Folded {
				continue
			}
			var full [7]deck.Card
			full[0], full[1] = p.HoleCards[0], p.HoleCards[1]
			copy(full[2:], t.board)
			score := handeval.Best7(full)
			switch {
			case score > bestScore:
				bestScore = score
				winners = []string{id}
			case score == bestScore:
				winners = append(winners, id)
			}
		}
		if len(winners) == 0 {
			// Every player who reached this layer's contribution level has
			// since folded — there's no one left to award it to at
			// showdown. These chips were never called by anyone still in
			// the hand, so they aren't "won" or "lost": they go back to
			// whoever put them in. layer.Eligible lists exactly the
			// contributor(s) who reached this layer's boundary, and by
			// construction (ComputeSidePots) each contributed the same
			// amount into this specific layer, so an even split (with the
			// odd chip to the first, same convention as a showdown win) is
			// the correct refund.
			n := int64(len(layer.Eligible))
			share := layer.Amount / n
			for _, id := range layer.Eligible {
				payouts[id] += share
			}
			remainder := layer.Amount - share*n
			if remainder > 0 {
				payouts[layer.Eligible[0]] += remainder
			}
			continue
		}
		share := layer.Amount / int64(len(winners))
		for _, w := range winners {
			payouts[w] += share
		}
		// Odd chip goes to the first winner in seat order (closest to the
		// button, standard convention) — winners is already in table seat
		// order since layer.Eligible preserves contributions' input order.
		remainder := layer.Amount - share*int64(len(winners))
		if remainder > 0 {
			payouts[winners[0]] += remainder
		}
	}
	for id, amount := range payouts {
		t.playerByID(id).Stack += amount
	}
	t.payouts = payouts
	t.stage = Complete
	t.rotateDealer()
}
