// Package hand orchestrates one table's full hand lifecycle (OVERVIEW.md
// § 3.1), tying together deck shuffling (Task 5), hand evaluation (Task 6),
// side pots (Task 7), and betting rounds (Task 8). Pure logic — no
// networking, no persistence; Phase 2 wires this to a live table server.
package hand

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
	"gopkg.aoctech.app/poker/api/internal/engine/sidepots"
)

// ErrAlreadySeated is returned by AddWaitingPlayer/AddMidHandJoiner when the
// player is already at the table. Callers (buyin.BuyIn) treat it as a
// successful no-op rather than an error, so a retried join cannot double-spend
// or fire a spurious refund.
var ErrAlreadySeated = errors.New("hand: already seated")

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
	ID          string       `dynamodbav:"id"`
	Stack       int64        `dynamodbav:"stack"`
	Ready       bool         `dynamodbav:"ready"`
	State       PlayerState  `dynamodbav:"state"`
	HoleCards   [2]deck.Card `dynamodbav:"hole_cards"`
	Contributed int64        `dynamodbav:"contributed"` // this hand's total contribution across all rounds, for side-pot math
	HoldID      string       `dynamodbav:"hold_id,omitempty"`

	VoluntarilyShown bool `dynamodbav:"voluntarily_shown"`

	// LastActionAt is the unix-ms time of this player's last genuine,
	// explicitly user-originated command (an Act, a Ready/SitOut toggle) —
	// never a server-synthesized one (a turn-timeout auto-fold, a disconnect
	// auto-sit-out). table.Actor is the only writer; it deliberately updates
	// this on real inbound commands only, so a connected-but-unresponsive
	// player can't have their own auto-folds mask how long they've actually
	// been silent. Persisted (survives an Actor restart, unlike the
	// in-memory disconnect bookkeeping) so a periodic sweep can detect and
	// remove a stale seat even if no disconnect was ever observed for it.
	LastActionAt int64 `dynamodbav:"last_action_at,omitempty"`
}

type Table struct {
	players     []*Player
	smallBlind  int64
	bigBlind    int64
	dealerSeat  int
	dealerDrawn bool
	stage       Stage
	board       []deck.Card
	shuffle     *deck.ShuffleResult
	nextCard    int
	round       *betting.Round
	roundIdx    map[string]int // playerID -> index into round.Players, for the active betting round

	// roundBaseline records, for each player in the current round, the value
	// round.Players[idx].Contributed held at the moment this round began
	// (0 for a fresh post-flop street; the blind just posted for the two
	// blind seats in the pre-flop round — see seedRoundContribution). Act
	// uses it to compute how much NEW money a player put in since the last
	// time we folded their round contribution into Player.Contributed, so a
	// player acting more than once in the same round (e.g. bet then facing a
	// re-raise) doesn't get double-counted. See Act's doc comment.
	roundBaseline map[string]int64

	payouts       map[string]int64
	rakeBPS       int64
	rakeCollected int64

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
	readyToPost   map[string]bool
	owesBigBlind  map[string]bool
	lastOutcome   *HandOutcome
	wasEverAllIn  map[string]bool
}

// HandOutcome is the durable, server-internal summary consumed by
// gamification. Payouts and winners are net of rake; refunded unmatched
// contributions are not wins.
type HandOutcome struct {
	Winners            []string
	WinningCategory    string
	WonWithoutShowdown bool
	ComebackWinners    []string
	Participants       []string
	Payouts            map[string]int64
	Contributions      map[string]int64
}

var categoryNames = map[handeval.Category]string{
	handeval.HighCard: "high_card", handeval.Pair: "pair", handeval.TwoPair: "two_pair",
	handeval.ThreeOfAKind: "three_of_a_kind", handeval.Straight: "straight", handeval.Flush: "flush",
	handeval.FullHouse: "full_house", handeval.FourOfAKind: "four_of_a_kind",
	handeval.StraightFlush: "straight_flush", handeval.RoyalFlush: "royal_flush",
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

func (t *Table) RakeCollected() int64 { return t.rakeCollected }

func (t *Table) LastOutcomeForActor() *HandOutcome { return t.lastOutcome }

// ConfigureRake enables the standard 2.5% real-money rake. Sandbox tables
// always remain rake-free. The setting is persisted with the table state.
func (t *Table) ConfigureRake(currencyMode string) {
	if currencyMode == "real" {
		t.rakeBPS = 250
		return
	}
	t.rakeBPS = 0
}

// PlayersForActor exposes the live player slice for Phase 2's table.Actor,
// which needs to toggle Ready before a hand starts (StartHand only reads it,
// nothing in this package previously needed to write it from outside).
func (t *Table) PlayersForActor() []*Player { return t.players }

func (t *Table) HoleAndBoardForActor(playerID string) ([2]deck.Card, []deck.Card, bool) {
	p := t.playerByID(playerID)
	if p == nil || (p.State != Active && p.State != AllIn) {
		return [2]deck.Card{}, nil, false
	}
	return p.HoleCards, append([]deck.Card(nil), t.board...), true
}

// CurrentPlayerCanActForActor exposes currentPlayerCanAct to Phase 2's
// table.Actor (auto-fold deadline arming needs to know whose turn it is
// without duplicating the round-state check outside this package).
func (t *Table) CurrentPlayerCanActForActor(playerID string) bool {
	return t.currentPlayerCanAct(playerID)
}

// CurrentPlayerIDForActor exposes currentPlayerToAct to Phase 2's table.Actor
// (the universal per-turn timer needs to know who must act now, and whether
// that has changed since the last broadcast, without duplicating round-state
// logic outside this package).
func (t *Table) CurrentPlayerIDForActor() string {
	return t.currentPlayerToAct()
}

// SitOutForActor marks a player SittingOut — used by Phase 2's disconnect
// grace-window handling once a disconnected player exceeds the grace period
// or enough consecutive disconnected hands (OVERVIEW.md § 4), and by a
// player's own voluntary "sit out" toggle. A player still Active in the
// current hand is folded out of the live betting round first: a bare state
// flip left betting.Round still waiting on their decision forever (the round
// never completes, and CurrentPlayerIDForActor never changes, so the
// universal turn timer's idempotent re-arm treats it as a no-op — the hand
// wedges permanently). A player already AllIn has no decision left to make
// and stays AllIn through showdown; Ready is already false by the time a
// caller reaches here for a voluntary sit-out, which alone excludes them from
// the next hand via eligibleForNextHand.
func (t *Table) SitOutForActor(playerID string) {
	p := t.playerByID(playerID)
	if p == nil {
		return
	}
	if p.State != Active {
		if p.State != AllIn {
			p.State = SittingOut
		}
		return
	}
	if idx, ok := t.roundIdx[playerID]; ok && t.round != nil {
		if err := t.round.Act(idx, betting.ActionFold, 0); err == nil {
			p.State = Folded
			if t.round.IsComplete() {
				t.advanceStage()
			}
			return
		}
	}
	p.State = SittingOut
}

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
func (t *Table) AddMidHandJoiner(p *Player) error {
	if t.playerByID(p.ID) != nil {
		return fmt.Errorf("%w: player %s", ErrAlreadySeated, p.ID)
	}
	p.Ready = true
	p.State = PendingEntry
	t.players = append(t.players, p)
	return nil
}

// MarkReadyToPost opts a pending entrant into the next hand. The entrant
// will post one big blind even when their table position is not the regular
// big-blind position.
func (t *Table) MarkReadyToPost(playerID string) {
	p := t.playerByID(playerID)
	if p == nil || p.State != PendingEntry {
		return
	}
	if t.readyToPost == nil {
		t.readyToPost = make(map[string]bool)
	}
	t.readyToPost[playerID] = true
}

// AddWaitingPlayer seats a new player between hands (not PENDING_ENTRY —
// they're eligible for the very next hand once ready, same as anyone seated
// at table construction). Rejects joining while a hand the player would
// otherwise be silently excluded from is already in progress — that path is
// AddMidHandJoiner's job instead.
func (t *Table) AddWaitingPlayer(p *Player) error {
	if t.stage != WaitingForPlayers && t.stage != Complete {
		return fmt.Errorf("hand: cannot add a waiting player while a hand is in progress, use AddMidHandJoiner")
	}
	if t.playerByID(p.ID) != nil {
		return fmt.Errorf("%w: player %s", ErrAlreadySeated, p.ID)
	}
	p.Ready = true
	t.players = append(t.players, p)
	return nil
}

// RemovePlayerForActor removes playerID from the table and returns their
// current stack and holdID (the amount buyin.Service credits back on cash-out).
// Errors if the player was dealt into a hand still in progress — a seat can't
// be pulled out from under a hand it's dealt into, even after folding: a
// folded player's contribution still sits in t.handOrder/side-pot eligibility
// until runShowdown resolves it, and playerByID would panic on a nil lookup
// if t.players no longer had them. The caller must wait for HAND_COMPLETE.
func (t *Table) RemovePlayerForActor(playerID string) (int64, string, error) {
	handInProgress := t.stage != WaitingForPlayers && t.stage != Complete
	for i, p := range t.players {
		if p.ID != playerID {
			continue
		}
		if handInProgress && t.dealtIntoCurrentHand(playerID) {
			return 0, "", fmt.Errorf("hand: cannot remove player %s mid-hand while still dealt in", playerID)
		}
		stack := p.Stack
		holdID := p.HoldID
		t.players = append(t.players[:i], t.players[i+1:]...)
		return stack, holdID, nil
	}
	return 0, "", fmt.Errorf("hand: player %s not found", playerID)
}

// dealtIntoCurrentHand reports whether playerID is part of t.handOrder — the
// seat order snapshotted at the start of the current hand — regardless of
// their present PlayerState (Active, AllIn, or Folded all still count: their
// chips are only fully settled once runShowdown runs).
func (t *Table) dealtIntoCurrentHand(playerID string) bool {
	for _, p := range t.handOrder {
		if p.ID == playerID {
			return true
		}
	}
	return false
}

// eligibleForNextHand reports whether p is dealt into the next hand: ready,
// not sitting out (unless they're owed a free return per
// RequestReturnFromSitOut), and not a still-pending mid-hand joiner (unless
// they've opted in via MarkReadyToPost). StartHand's readyCount gate and its
// active-player selection loop must agree on this, or a table can start a
// hand with fewer real players than it thinks it has (e.g. a busted,
// SittingOut player still counted as ready).
func (t *Table) eligibleForNextHand(p *Player) bool {
	if !p.Ready {
		return false
	}
	if p.State == SittingOut && !t.owesBigBlind[p.ID] {
		return false
	}
	if p.State == PendingEntry && !t.readyToPost[p.ID] {
		return false
	}
	return true
}

// StartHand begins a new hand: requires >=2 ready players, posts blinds
// relative to dealerSeat (heads-up special case: dealer posts small blind),
// shuffles via commit-reveal, and deals hole cards. dealerSeat itself is
// rotated forward to the next seat at the END of each hand (see
// rotateDealer, called from runShowdown) so the SECOND and later calls to
// StartHand on the same Table use a new dealer. The first hand's button is
// drawn uniformly with crypto/rand among the players dealt into that hand.
func (t *Table) StartHand() error {
	readyCount := 0
	for _, p := range t.players {
		if t.eligibleForNextHand(p) {
			readyCount++
		}
	}
	if readyCount < 2 {
		// A Complete table with too few eligible players must fall back to
		// WaitingForPlayers, or it (and any post-hand countdown UI relying on
		// Stage) stays stuck on Complete forever.
		t.stage = WaitingForPlayers
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
	t.lastOutcome = nil
	t.wasEverAllIn = make(map[string]bool)
	t.rakeCollected = 0
	t.seenActionIDs = make(map[string]bool)
	for _, p := range t.players {
		p.VoluntarilyShown = false
	}

	active := make([]*Player, 0, len(t.players))
	newEntrants := make(map[string]bool)
	for _, p := range t.players {
		if !t.eligibleForNextHand(p) {
			if p.State != PendingEntry {
				p.State = SittingOut
			}
			continue
		}
		if p.State == PendingEntry {
			newEntrants[p.ID] = true
			delete(t.readyToPost, p.ID)
		}
		if p.State == SittingOut {
			newEntrants[p.ID] = true
			delete(t.owesBigBlind, p.ID)
		}
		p.State = Active
		p.Contributed = 0
		p.HoleCards = [2]deck.Card{t.dealCard(), t.dealCard()}
		active = append(active, p)
	}

	t.handOrder = active
	if !t.dealerDrawn {
		dealerIdx, err := randomIndex(len(active))
		if err != nil {
			return fmt.Errorf("hand: draw initial dealer: %w", err)
		}
		for i, p := range t.players {
			if p == active[dealerIdx] {
				t.dealerSeat = i
				break
			}
		}
		t.dealerDrawn = true
	}
	sbSeat, bbSeat := t.blindSeats(active)
	t.postBlind(active[sbSeat], t.smallBlind)
	t.postBlind(active[bbSeat], t.bigBlind)
	for _, p := range active {
		if newEntrants[p.ID] && p != active[bbSeat] {
			t.postBlind(p, t.bigBlind)
		}
	}

	t.startBettingRound(active, t.bigBlind, t.bigBlind)
	// The blinds were posted onto Player.Contributed before the round
	// existed; seed the round's own per-player Contributed (and the
	// baseline that gates Act's bookkeeping) so Check/Call math sees them
	// as already-in-this-street money instead of demanding it again.
	t.seedRoundContribution(active[sbSeat].ID, active[sbSeat].Contributed)
	t.seedRoundContribution(active[bbSeat].ID, active[bbSeat].Contributed)
	for _, p := range active {
		if newEntrants[p.ID] {
			t.seedRoundContribution(p.ID, p.Contributed)
		}
	}
	t.stage = PreFlop
	return nil
}

// randomIndex uses rejection sampling so every index has exactly the same
// probability even when n does not divide the uint64 range.
func randomIndex(n int) (int, error) {
	if n <= 0 {
		return 0, fmt.Errorf("invalid upper bound %d", n)
	}
	max := ^uint64(0) - (^uint64(0) % uint64(n))
	for {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		v := binary.BigEndian.Uint64(b[:])
		if v < max {
			return int(v % uint64(n)), nil
		}
	}
}

// EscalateBlindsForActor raises both blinds while preserving their ratio.
// The big blind is capped exactly at maxBigBlind and invalid configs are
// ignored defensively.
func (t *Table) EscalateBlindsForActor(multiplierPct int, maxBigBlind int64) {
	if multiplierPct <= 100 || maxBigBlind <= 0 || t.bigBlind >= maxBigBlind {
		return
	}
	oldBig := t.bigBlind
	newBig := oldBig * int64(multiplierPct) / 100
	if newBig <= oldBig {
		newBig = oldBig + 1
	}
	if newBig > maxBigBlind {
		newBig = maxBigBlind
	}
	t.smallBlind = t.smallBlind * newBig / oldBig
	if t.smallBlind == 0 {
		t.smallBlind = 1
	}
	t.bigBlind = newBig
}

func (t *Table) BigBlindForTest() int64 { return t.bigBlind }

// blindSeats returns (smallBlindIdx, bigBlindIdx) as indices into active,
// computed relative to dealerSeat's position within active. Heads-up is a
// special case: the dealer posts the small blind. 3+-way: the two seats
// clockwise after the dealer post small and big blind respectively.
// wouldBeNextBlind reports whether playerID would post the small or big
// blind if StartHand ran right now with playerID included among the active
// players — used by RequestReturnFromSitOut to decide whether returning from
// sitting-out is free or costs a big blind (the same rule as a brand-new
// mid-hand joiner: "perto do próprio blind" = SB or BB of the very next hand,
// no window).
func (t *Table) wouldBeNextBlind(playerID string) bool {
	active := make([]*Player, 0, len(t.players))
	for _, p := range t.players {
		if p.ID == playerID {
			active = append(active, p) // the returning candidate is always projected as playing
			continue
		}
		if !p.Ready || p.State == SittingOut {
			continue
		}
		if p.State == PendingEntry && !t.readyToPost[p.ID] {
			continue
		}
		active = append(active, p)
	}
	if len(active) < 2 {
		return false
	}
	sb, bb := t.blindSeats(active)
	for i, p := range active {
		if p.ID == playerID {
			return i == sb || i == bb
		}
	}
	return false
}

// RequestReturnFromSitOut lets a sitting-out player rejoin. A no-op if the
// player is not currently SittingOut. Reuses the exact BB-out-of-position
// template StartHand already applies to mid-hand joiners (readyToPost):
// projects whether this player would be SB/BB of the next hand and, if so,
// defers the actual return until StartHand charges that big blind instead of
// clearing SittingOut immediately.
func (t *Table) RequestReturnFromSitOut(playerID string) {
	p := t.playerByID(playerID)
	if p == nil || p.State != SittingOut {
		return
	}
	if t.wouldBeNextBlind(playerID) {
		if t.owesBigBlind == nil {
			t.owesBigBlind = make(map[string]bool)
		}
		t.owesBigBlind[playerID] = true
		return
	}
	p.State = Active
}

// RevealHoleCards lets a player who was dealt into the just-completed hand
// voluntarily show their cards to everyone, even when the hand ended without
// a genuine showdown (fold-to-one) — ViewFor's revealAll gate never covers
// this case on purpose (see the bug3 fix), so this is a separate, per-player
// opt-in. Idempotent: calling it twice for the same player is a no-op, not an
// error.
func (t *Table) RevealHoleCards(playerID string) error {
	if t.stage != Complete {
		return fmt.Errorf("hand: cards can only be revealed after the hand is complete")
	}
	dealtIn := false
	for _, hp := range t.handOrder {
		if hp.ID == playerID {
			dealtIn = true
			break
		}
	}
	if !dealtIn {
		return fmt.Errorf("hand: player %s was not dealt into this hand", playerID)
	}
	t.playerByID(playerID).VoluntarilyShown = true
	return nil
}

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
		if t.wasEverAllIn == nil {
			t.wasEverAllIn = make(map[string]bool)
		}
		t.wasEverAllIn[p.ID] = true
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
	if current := t.currentPlayerToAct(); current != "" && current != playerID {
		return fmt.Errorf("hand: it is not player %s's turn to act", playerID)
	}
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
		if t.wasEverAllIn == nil {
			t.wasEverAllIn = make(map[string]bool)
		}
		t.wasEverAllIn[p.ID] = true
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
	remaining, canStillAct := t.countRemainingAndActable()
	if remaining <= 1 {
		t.runShowdown()
		return
	}
	// Two or more players are still in the hand, but at most one of them is
	// NOT all-in — e.g. two players shoved pre-flop and everyone else folded
	// or called all-in too. There's nobody left who could call Act to ever
	// complete another betting round (a lone non-all-in player has no one to
	// bet against), so dealing the next street and calling startBettingRound
	// would hang the hand forever. Deal the immediate next street now (same
	// as a normal transition below) and let the caller (table.Actor) pace
	// any further streets one at a time via AdvanceRunoutStreetForActor —
	// see IsAwaitingRunoutForActor.
	if canStillAct <= 1 {
		t.AdvanceRunoutStreetForActor()
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

// countRemainingAndActable reports how many players are still in the hand
// (Active or AllIn) and how many of those can still make a betting decision
// (Active only) — shared by advanceStage and IsAwaitingRunoutForActor so both
// agree on exactly the same definition of "nobody left to bet against".
func (t *Table) countRemainingAndActable() (remaining, canStillAct int) {
	for _, p := range t.players {
		if p.State == Active || p.State == AllIn {
			remaining++
			if p.State == Active {
				canStillAct++
			}
		}
	}
	return remaining, canStillAct
}

// AdvanceRunoutStreetForActor deals exactly the next missing community-card
// street (no betting round — at most one player can still act) and, once
// that street is the river, runs showdown immediately. Phase 2's table.Actor
// calls this once synchronously from within Act (via advanceStage, to reveal
// the first missing street right away) and again from a paced timer for
// every further street, checking IsAwaitingRunoutForActor between calls to
// know whether another call is still needed.
func (t *Table) AdvanceRunoutStreetForActor() {
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
	}
	if t.stage == River {
		t.runShowdown()
	}
}

// IsAwaitingRunoutForActor reports whether the table is mid all-in runout —
// the board still has a street left to deal and no betting round can ever
// complete again (at most one player can still act). Excluding PreFlop keeps
// this from ever firing before the single remaining actor has had their own
// pre-flop turn: advanceStage always deals the immediate next missing street
// synchronously inside the same Act call, so by the time anyone observes
// this from outside a hand, PreFlop can never still be the case here.
// Recomputed from player state on every call — no persisted flag needed,
// since dealing a street is the only thing that can change the answer.
func (t *Table) IsAwaitingRunoutForActor() bool {
	if t.stage != Flop && t.stage != Turn {
		return false
	}
	remaining, canStillAct := t.countRemainingAndActable()
	return remaining > 1 && canStillAct <= 1
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
	nonFolded := 0
	for _, p := range t.handOrder {
		if p.State != Folded {
			nonFolded++
		}
	}
	wonWithoutShowdown := nonFolded == 1
	t.stage = Showdown
	contributions := make([]sidepots.Contribution, 0, len(t.handOrder))
	for _, p := range t.handOrder {
		if p.Contributed > 0 {
			contributions = append(contributions, sidepots.Contribution{PlayerID: p.ID, Amount: p.Contributed})
		}
	}
	layers := sidepots.ComputeSidePots(contributions)

	payouts := make(map[string]int64)
	winningIDs := make([]string, 0)
	var winningScore handeval.Score
	remainingRakeCap := t.rakeCap()
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
		layerRake := t.rakeForLayer(layer.Amount, remainingRakeCap)
		remainingRakeCap -= layerRake
		t.rakeCollected += layerRake
		netAmount := layer.Amount - layerRake
		if netAmount > 0 {
			winningIDs = append(winningIDs, winners...)
			if bestScore > winningScore {
				winningScore = bestScore
			}
		}
		share := netAmount / int64(len(winners))
		for _, w := range winners {
			payouts[w] += share
		}
		// Odd chip goes to the first winner in seat order (closest to the
		// button, standard convention) — winners is already in table seat
		// order since layer.Eligible preserves contributions' input order.
		remainder := netAmount - share*int64(len(winners))
		if remainder > 0 {
			payouts[winners[0]] += remainder
		}
	}
	for id, amount := range payouts {
		t.playerByID(id).Stack += amount
	}
	for _, p := range t.handOrder {
		if p.Stack <= 0 {
			p.State = SittingOut
		}
	}
	t.payouts = payouts
	contributionsByID := make(map[string]int64, len(contributions))
	for _, c := range contributions {
		contributionsByID[c.PlayerID] = c.Amount
	}
	outcome := HandOutcome{
		Winners:            dedupeIDs(winningIDs),
		WonWithoutShowdown: wonWithoutShowdown,
		Participants:       participantIDs(t.handOrder),
		Payouts:            payouts,
		Contributions:      contributionsByID,
	}
	if !wonWithoutShowdown {
		outcome.WinningCategory = categoryNames[winningScore.Category()]
	}
	for _, id := range outcome.Winners {
		if t.wasEverAllIn[id] {
			outcome.ComebackWinners = append(outcome.ComebackWinners, id)
		}
	}
	t.lastOutcome = &outcome
	t.stage = Complete
	t.rotateDealer()
}

func dedupeIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func participantIDs(players []*Player) []string {
	out := make([]string, len(players))
	for i, player := range players {
		out[i] = player.ID
	}
	return out
}

func (t *Table) rakeCap() int64 {
	if t.rakeBPS == 0 || len(t.board) < 3 {
		return 0
	}
	players := len(t.handOrder)
	switch {
	case players <= 2:
		return t.bigBlind / 2
	case players <= 4:
		return t.bigBlind * 3 / 4
	default:
		return t.bigBlind
	}
}

func (t *Table) rakeForLayer(amount, remainingCap int64) int64 {
	if remainingCap <= 0 || t.rakeBPS <= 0 {
		return 0
	}
	rake := amount * t.rakeBPS / 10000
	if rake > remainingCap {
		return remainingCap
	}
	return rake
}
