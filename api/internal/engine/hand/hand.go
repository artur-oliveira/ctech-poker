// Package hand orchestrates one table's full hand lifecycle (OVERVIEW.md
// § 3.1), tying together deck shuffling (Task 5), hand evaluation (Task 6),
// side pots (Task 7), and betting rounds (Task 8). Pure logic — no
// networking, no persistence; Phase 2 wires this to a live table server.
package hand

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

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

// ErrPlayerNotFound means playerID already isn't seated at this table — a
// terminal outcome for whoever's trying to remove them (they're already
// gone), unlike RemovePlayerForActor's mid-hand rejection, which is a normal,
// retryable race at a busy table.
var ErrPlayerNotFound = errors.New("hand: player not found")

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
	ID             string `dynamodbav:"id"`
	Name           string `dynamodbav:"name,omitempty"`
	AvatarURL      string `dynamodbav:"avatar_url,omitempty"`
	PlaystyleBadge string `dynamodbav:"playstyle_badge,omitempty"`
	RunItTwice     bool   `dynamodbav:"run_it_twice,omitempty"`
	// AutoRebuy and BuyInAmount are set exactly once, at fresh-seat creation
	// (AddWaitingPlayer/AddMidHandJoiner's new-Player branch) — rebuyExisting
	// never touches either, so a later manual rebuy at a different amount
	// can't change what the server auto-rebuys back to.
	AutoRebuy   bool  `dynamodbav:"auto_rebuy,omitempty"`
	BuyInAmount int64 `dynamodbav:"buy_in_amount,omitempty"`
	Stack       int64 `dynamodbav:"stack"`
	Ready       bool  `dynamodbav:"ready"`
	// PendingExit means the player asked to leave (RequestExit). They are
	// paused (Ready is cleared, same as any sit-out) and, once no longer
	// dealt into the current hand, Actor's post-commit sweep removes and
	// cashes them out automatically. Persisted — not an actor-local map —
	// so it survives an actor restart/handoff (see api/CLAUDE.md's
	// disconnectedSince cautionary note for why that distinction matters).
	PendingExit bool         `dynamodbav:"pending_exit,omitempty"`
	State       PlayerState  `dynamodbav:"state"`
	HoleCards   [2]deck.Card `dynamodbav:"hole_cards"`
	Contributed int64        `dynamodbav:"contributed"` // this hand's total contribution across all rounds, for side-pot math
	HoldID      string       `dynamodbav:"hold_id,omitempty"`
	// HandStartStack is captured before StartHand posts any blind. A pointer
	// preserves presence across rolling deployments and distinguishes a real
	// zero-chip all-in entry from older persisted state that never recorded it.
	HandStartStack *int64 `dynamodbav:"hand_start_stack,omitempty"`

	// VoluntarilyShown is retained for persisted-state compatibility with the
	// original all-or-nothing reveal. New writes use VoluntarilyShownCards.
	VoluntarilyShown      bool    `dynamodbav:"voluntarily_shown,omitempty"`
	VoluntarilyShownCards [2]bool `dynamodbav:"voluntarily_shown_cards,omitempty"`

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

	// TimeBankMs is pointer-backed so a rolling deployment can distinguish
	// an older persisted seat (nil: initialize it once) from a player who
	// genuinely exhausted their bank (non-nil zero). The table actor owns
	// consumption; the engine only persists and recharges it between hands.
	TimeBankMs *int64 `dynamodbav:"time_bank_ms,omitempty"`
}

const (
	DefaultTimeBankMs  int64 = 30_000
	TimeBankRechargeMs int64 = 5_000
)

func initializeTimeBank(p *Player) {
	if p.TimeBankMs == nil {
		initial := DefaultTimeBankMs
		p.TimeBankMs = &initial
	}
}

type Table struct {
	players           []*Player
	smallBlind        int64
	bigBlind          int64
	dealerSeat        int
	dealerDrawn       bool
	stage             Stage
	board             []deck.Card
	boardTwo          []deck.Card
	boardSplitAt      int
	runItTwiceEnabled bool
	runItTwice        bool
	runoutPhase       int
	shuffle           *deck.ShuffleResult
	nextCard          int
	round             *betting.Round
	roundIdx          map[string]int // playerID -> index into round.Players, for the active betting round

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

	// currencyMode is set once by ConfigureRake and never changes for the
	// table's lifetime. It gates RequestRabbitHunt: a per-hand real-money
	// debit for a curiosity feature would move real chips outside any
	// wallet transaction, so real-money tables are closed at this layer,
	// not assumed closed by the UI never showing the button.
	currencyMode string

	// rabbitHuntPaid tracks, for the current hand, which players have paid
	// the big-blind fee to reveal the rabbit-hunt runout. Reset every hand
	// alongside rakeCollected/seenActionIDs (see StartHand).
	rabbitHuntPaid map[string]bool

	// winnerCardsPaid tracks, for the current hand, which dealt-in opponents
	// paid to see the uncontested winner's otherwise-mucked hole cards.
	winnerCardsPaid map[string]bool

	// winnerCardsAsked tracks who has already asked this hand, accepted or
	// not. winnerCardsPaid can't do this job — it also drives the reveal in
	// ViewFor — and without a separate flag a declined requester could re-ask
	// immediately, for as long as the post-hand window lasts, which would make
	// the winner's "no" worth nothing.
	winnerCardsAsked map[string]bool

	// pendingWinnerCards is this hand's single outstanding paid-reveal
	// request, waiting on the winner's answer. Unlike the rabbit hunt — whose
	// secret belongs to the deck — these cards belong to another player, so
	// the fee buys a request, not the cards
	// (docs/specs/2026-08-24-pay-to-see-cards-consent.md, option B). At most
	// one is outstanding per hand; it is always resolved (accepted, declined,
	// expired, or refunded by StartHand) before the next hand deals.
	pendingWinnerCards *WinnerCardsRequest

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
	// AllInPlayers lists every participant who went all-in at any point this
	// hand (win or lose) — drives the all_in achievement, distinct from
	// ComebackWinners which only counts those who also won.
	AllInPlayers  []string
	Participants  []string
	Payouts       map[string]int64
	Contributions map[string]int64
	PotResults    []PotResult
	// Board and PlayerHands are the just-completed hand's community cards and
	// every participant's hole cards, captured here because t.board/HoleCards
	// get overwritten in place by the next StartHand — this is the only
	// durable copy a caller (sessionlog's per-player match history) ever
	// gets. Card codes use Snapshot's 2-char notation ("Ah", "Tc", ...).
	Board       []string
	BoardTwo    []string `json:"board_two,omitempty"`
	PlayerHands map[string]PlayerHandInfo
	// ShowdownResults holds each non-folded participant's OWN best-hand
	// category and result. Unlike PlayerHands/Revealed, this is never
	// exposed to opponents — it only drives per-player achievements
	// (looser/almost_winner/tied/bad_beat/cooler) in achievements.Service,
	// which needs a player's own category regardless of whether that hand
	// was ever shown to the table. Only populated when !WonWithoutShowdown
	// (a real showdown happened, so there's something to compare).
	ShowdownResults map[string]ShowdownResult
	// ServerSeed and CommitHash are this hand's shuffle fairness proof
	// (deck.ShuffleResult), hex-encoded. ServerSeed alone lets anyone
	// recompute the full 52-card shuffle (shuffleWithSeed is a deterministic
	// function of it) and CommitHash lets them verify it matches what the
	// server committed to before dealing (ARCHITECTURE.md § 3.5 / B32).
	ServerSeed     string
	CommitHash     string
	RootCommitHash string
	// FairnessProofs is each participant's viewer-scoped deck proof, keyed by
	// player ID (Table.FairnessProofsForActor). It is filled in by the actor on
	// the copy handed to the hand-complete / hand-updated hooks — never stored
	// on Table.lastOutcome, which is persisted with every table state write and
	// has no need for 52 hashes per seat.
	FairnessProofs map[string]FairnessProof
}

// FairnessProof lets a player rebuild RootCommitHash for one hand without
// seeing cards they aren't entitled to: revealed positions carry the card plus
// its salt, every other position carries only its committed hash. ServerSeedHex
// is set only when the whole deck is public anyway (see fairnessProofFor).
type FairnessProof struct {
	ServerSeedHex        string
	RevealedCardSalts    map[int]RevealedSaltView
	UnrevealedCardHashes map[int]string
}

// PlayerHandInfo is one participant's hole cards from HandOutcome. Revealed
// mirrors ViewFor's exact visibility rule (a genuine showdown reveal, or a
// voluntary show — never a bare folded hand) so a consumer can show these
// cards to other players without re-deriving that logic; HoleCards is always
// populated regardless of Revealed so the player can see their own cards.
type PlayerHandInfo struct {
	HoleCards     [2]string
	Revealed      bool
	RevealedCards [2]bool
}

// PotResult records one contribution layer independently. Multiple IDs in
// Winners means that specific pot was split; winners of different layers are
// not a tie.
type PotResult struct {
	Amount            int64
	PayoutAmount      int64
	EligiblePlayerIDs []string
	Winners           []string
	Payouts           map[string]int64
	Refund            bool
	Runout            int
}

// ShowdownResult is one participant's own showdown outcome, see
// HandOutcome.ShowdownResults.
type ShowdownResult struct {
	Category string
	Won      bool
	// Tied is the participant's overall history result: they won only split
	// pots and no pot outright. SplitPot separately records that any pot they
	// won was shared, including mixed outright+split outcomes.
	Tied     bool
	SplitPot bool
}

func (s ShowdownResult) Action() string {
	if s.Tied {
		return "tie"
	} else if s.Won {
		return "won"
	}
	return "lost"
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

// ConfigureRake enables the standard 2.5% sandbox rake. Real-money tables
// are always rake-free — Brazilian law treats a cut of the pot/blind on a
// public real-money game as a bet requiring SPA authorization; poker's
// real-money revenue comes entirely from the fixed table-entry fee charged
// at buy-in instead (buyin.Service.BuyIn), never from the pot. The setting
// is persisted with the table state.
func (t *Table) ConfigureRake(currencyMode string) {
	t.currencyMode = currencyMode
	if currencyMode == "sandbox" {
		t.rakeBPS = 250
		return
	}
	t.rakeBPS = 0
}

// ConfigureRunItTwice enables the room-level gate. The per-hand decision is
// still unanimous and is frozen when betting closes.
func (t *Table) ConfigureRunItTwice(enabled bool) { t.runItTwiceEnabled = enabled }

// SetPlayerRunItTwiceForActor updates the viewer-private, table-scoped
// preference and reports whether persistence is necessary.
func (t *Table) SetPlayerRunItTwiceForActor(playerID string, enabled bool) bool {
	p := t.playerByID(playerID)
	if p == nil || p.RunItTwice == enabled {
		return false
	}
	p.RunItTwice = enabled
	return true
}

func (t *Table) shouldRunItTwice() bool {
	if !t.runItTwiceEnabled {
		return false
	}
	remaining := 0
	for _, p := range t.handOrder {
		if p.State != Active && p.State != AllIn {
			continue
		}
		remaining++
		if !p.RunItTwice {
			return false
		}
	}
	return remaining >= 2
}

// RunoutPhaseForActor participates in the actor timer's idempotency key.
func (t *Table) RunoutPhaseForActor() int { return t.runoutPhase }

// PlayersForActor exposes the live player slice for Phase 2's table.Actor,
// which needs to toggle Ready before a hand starts (StartHand only reads it,
// nothing in this package previously needed to write it from outside).
func (t *Table) PlayersForActor() []*Player { return t.players }

// DuplicateSeatIDForActor reports the first player ID occupying more than one
// seat, if any. t.players must never contain the same ID twice — every seat
// mutation (AddMidHandJoiner/AddWaitingPlayer/rebuyExisting/RemovePlayerForActor)
// is written to dedupe by ID, so a duplicate here means some earlier mutation
// was applied to an in-memory cache that a failed, non-rolled-back commit left
// poisoned (see actor.go's commit, which refuses to persist this). Detecting it
// here rather than trusting every call site to get its own rollback right is
// the backstop: a two-line invariant check is far cheaper to keep correct than
// auditing every future seat-mutating code path for a missed rollback.
func (t *Table) DuplicateSeatIDForActor() (string, bool) {
	seen := make(map[string]bool, len(t.players))
	for _, p := range t.players {
		if seen[p.ID] {
			return p.ID, true
		}
		seen[p.ID] = true
	}
	return "", false
}

// SetPlayerIdentityForActor persists display identity with the seat so snapshots
// built by any actor instance carry the same name/avatar. It reports whether the
// value changed, allowing connection setup to avoid a no-op table commit.
func (t *Table) SetPlayerIdentityForActor(playerID, name, avatarURL, playstyleBadge string) bool {
	p := t.playerByID(playerID)
	if p == nil || (name == "" && avatarURL == "" && playstyleBadge == "") ||
		(p.Name == name && p.AvatarURL == avatarURL && p.PlaystyleBadge == playstyleBadge) {
		return false
	}
	p.Name = name
	p.AvatarURL = avatarURL
	p.PlaystyleBadge = playstyleBadge
	return true
}

func (t *Table) HoleAndBoardForActor(playerID string) ([2]deck.Card, []deck.Card, bool) {
	p := t.playerByID(playerID)
	// State alone is not proof that the player belongs to the current hand:
	// a sitting-out seat may request a free return mid-hand and become Active
	// for the next deal. Only handOrder identifies players who actually
	// received cards in this hand.
	if p == nil || !t.dealtIntoCurrentHand(playerID) || (p.State != Active && p.State != AllIn) {
		return [2]deck.Card{}, nil, false
	}
	board := t.board
	if t.runItTwice && t.runoutPhase == 2 {
		board = append(append([]deck.Card(nil), t.board[:t.boardSplitAt]...), t.boardTwo...)
	}
	return p.HoleCards, append([]deck.Card(nil), board...), true
}

// CurrentPlayerCanActForActor exposes currentPlayerCanAct to Phase 2's
// table.Actor (auto-fold deadline arming needs to know whose turn it is
// without duplicating the round-state check outside this package).
// PlayerAllInForActor reports whether id is currently in the AllIn state —
// used by table.Actor to relabel a just-committed bet/call/raise as "all_in"
// in the action log when it pushed the player's whole stack in.
func (t *Table) PlayerAllInForActor(id string) bool {
	p := t.playerByID(id)
	return p != nil && p.State == AllIn
}

// NormalizedActionForActor returns the action Act will actually apply. The
// caller records this before Act potentially advances the street and destroys
// the current round context.
func (t *Table) NormalizedActionForActor(playerID string, action betting.Action) betting.Action {
	idx, ok := t.roundIdx[playerID]
	if !ok || t.round == nil || idx < 0 || idx >= len(t.round.Players) {
		return action
	}
	bs := t.round.Players[idx]
	if (action == betting.ActionRaise || action == betting.ActionBet) && bs.Contributed+bs.Stack <= t.round.CurrentBet {
		return betting.ActionCall
	}
	if action == betting.ActionCall && bs.Contributed >= t.round.CurrentBet {
		return betting.ActionCheck
	}
	return action
}

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
// and stays AllIn through showdown.
//
// Ready is cleared here rather than left to the caller: the fold path below
// leaves State==Folded, and a Folded+Ready seat is eligibleForNextHand — so
// without this the player is dealt right back into the next hand and burns
// another full turn timeout, which is exactly what sitting them out is meant
// to stop. StartHand flips them to SittingOut on the next deal, and a ready
// toggle (RequestReturnFromSitOut) brings them back paying the same
// out-of-position big blind as any other returning player.
func (t *Table) SitOutForActor(playerID string) {
	p := t.playerByID(playerID)
	if p == nil {
		return
	}
	p.Ready = false
	// Once resolution is over, AllIn is only a residue of the finished hand,
	// not a reason to keep the seat eligible for the next deal.
	if t.stage == Complete || t.stage == WaitingForPlayers {
		p.State = SittingOut
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

// RequestExit pauses playerID (no future hands) and, if it is currently
// their turn, folds them out of the live betting round via SitOutForActor —
// same as the disconnect/turn-timeout path. Round.Act has no turn-order
// check of its own, so SitOutForActor would fold ANY Active player it's
// called on, not just the one on the clock; calling it unconditionally here
// would force-fold an exiting BB/SB before their turn ever comes back
// around, breaking an uncontested win they're still owed. So a player who is
// dealt in and Active but not currently on the clock is left exactly as
// they are — Actor.processPendingExitAutoFolds (driven from broadcastAll,
// the same per-commit reconciliation point armTurnTimer/preselections use)
// folds them the instant their own turn actually arrives, via
// CurrentPlayerHasPendingExitForActor below.
func (t *Table) RequestExit(playerID string) error {
	p := t.playerByID(playerID)
	if p == nil {
		return fmt.Errorf("%w: %s", ErrPlayerNotFound, playerID)
	}
	p.PendingExit = true
	if t.currentPlayerToAct() == playerID {
		t.SitOutForActor(playerID)
		return nil
	}
	p.Ready = false
	if p.State == Active {
		return nil
	}
	if p.State != AllIn {
		p.State = SittingOut
	}
	return nil
}

// CurrentPlayerHasPendingExitForActor reports whether the player currently
// on the clock (if any) has a pending exit request. Actor's
// processPendingExitAutoFolds uses this to fold them out the moment it
// becomes their turn, rather than at RequestExit time — see RequestExit's
// doc comment for why that distinction matters.
func (t *Table) CurrentPlayerHasPendingExitForActor() bool {
	p := t.playerByID(t.currentPlayerToAct())
	return p != nil && p.PendingExit
}

// CancelExit reverses a still-pending RequestExit — mirrors the Ready:true
// branch of the ordinary sit-out toggle (RequestReturnFromSitOut) so a
// player who exited when it was not yet their turn resumes eligibility the
// exact same way a voluntary sit-out un-does. A player already folded out of
// the current hand by RequestExit stays folded for THIS hand (canceling
// exit is not "undo my fold") but is Ready again for the next one.
func (t *Table) CancelExit(playerID string) error {
	p := t.playerByID(playerID)
	if p == nil {
		return fmt.Errorf("%w: %s", ErrPlayerNotFound, playerID)
	}
	if !p.PendingExit {
		return fmt.Errorf("hand: player %s has no pending exit to cancel", playerID)
	}
	p.PendingExit = false
	p.Ready = true
	t.RequestReturnFromSitOut(playerID)
	return nil
}

// DealtIntoCurrentHandForActor exposes dealtIntoCurrentHand to
// internal/table (a different package) so Actor's post-commit sweep can
// check removal eligibility before attempting RemovePlayerForActor.
func (t *Table) DealtIntoCurrentHandForActor(playerID string) bool {
	return t.dealtIntoCurrentHand(playerID)
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
	if existing := t.playerByID(p.ID); existing != nil {
		return t.rebuyExisting(existing, p)
	}
	p.Ready = true
	p.State = PendingEntry
	initializeTimeBank(p)
	t.players = append(t.players, p)
	return nil
}

// rebuyExisting tops up a seat that's still occupied by a busted (Stack<=0)
// player — bust never removes a player from t.players (runShowdown's
// Stack<=0 loop just sets State=SittingOut), so without this a rebuy's join
// would hit ErrAlreadySeated and buyin.Service would silently no-op it,
// never crediting the chips the player just paid for. A player who still has
// chips keeps hitting ErrAlreadySeated: that guard is what stops a retried
// join request from double-spending. Re-entry follows the exact same
// out-of-position rule as RequestReturnFromSitOut (owe a big blind if
// rebuying into what would be the very next SB/BB) so a rebuy can't dodge
// the blind everyone else pays for a seat.
func (t *Table) rebuyExisting(existing, incoming *Player) error {
	if existing.Stack > 0 {
		return fmt.Errorf("%w: player %s", ErrAlreadySeated, existing.ID)
	}
	existing.Stack = incoming.Stack
	existing.HoldID = incoming.HoldID
	existing.LastActionAt = incoming.LastActionAt
	existing.Ready = true
	if existing.State == SittingOut {
		if t.wouldBeNextBlind(existing.ID) {
			if t.owesBigBlind == nil {
				t.owesBigBlind = make(map[string]bool)
			}
			t.owesBigBlind[existing.ID] = true
		} else {
			existing.State = Active
		}
	}
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
	if existing := t.playerByID(p.ID); existing != nil {
		return t.rebuyExisting(existing, p)
	}
	p.Ready = true
	initializeTimeBank(p)
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
		// dealerSeat is a raw index into t.players, not a player identity.
		// Splicing out anyone seated before the button shifts every later
		// player's index down by one; without this adjustment dealerSeat
		// would keep pointing at that same numeric slot, which now holds a
		// different player — silently handing them the button/blinds without
		// ever actually rotating. Removing the button itself (i ==
		// dealerSeat) needs no adjustment: the slot's index is unchanged, so
		// whoever shifts into it (the button's former neighbor) becomes
		// dealer, which is the standard "next player takes the empty button"
		// convention.
		if i < t.dealerSeat {
			t.dealerSeat--
		}
		t.players = append(t.players[:i], t.players[i+1:]...)
		if len(t.players) == 0 {
			// The last hand's handOrder/payouts/lastOutcome are otherwise only
			// cleared by the next StartHand — with the table empty that may
			// not happen for a long time (or ever). Left dangling, a later
			// rejoiner reusing this same playerID gets erroneously re-linked
			// to the stale handOrder entry on the next state reload
			// (NewTableFromState matches by ID, not by seating instance),
			// which then leaks that entry's zero-value HoleCards as if the
			// rejoiner were dealt into a hand that doesn't exist. The stale
			// t.payouts also makes the client's isFreshPayout check fire a
			// bogus win/lose banner on the very first snapshot after
			// rejoining. Nobody is left seated to see a recap, so clearing
			// now is safe.
			t.handOrder = nil
			t.payouts = nil
			t.lastOutcome = nil
		}
		return stack, holdID, nil
	}
	return 0, "", fmt.Errorf("%w: %s", ErrPlayerNotFound, playerID)
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
		// Stage) stays stuck on Complete forever. Clearing handOrder/payouts/
		// lastOutcome here too (not just on the success path below) is what
		// actor.go's join comment promises callers: without it, a lone
		// remaining/rejoining player (or any ready-toggle that re-enters this
		// branch) keeps rebroadcasting the previous hand's payouts forever —
		// ViewFor's dealtIn gate leaks stale/zero-value hole cards for anyone
		// whose ID still matches a handOrder entry, and the client's
		// holdOutcomeOpen (driven by Boolean(payouts)) never closes. The same
		// staleness applies to each player's Contributed: it's only reset by
		// the active-player loop further down, which this early return skips,
		// so a lone remaining player can otherwise carry the last hand's
		// contribution into a waiting_for_players snapshot.
		t.handOrder = nil
		t.payouts = nil
		t.lastOutcome = nil
		t.board = nil
		t.boardTwo = nil
		t.boardSplitAt = 0
		t.runItTwice = false
		t.runoutPhase = 0
		t.shuffle = nil
		t.nextCard = 0
		t.stage = WaitingForPlayers
		for _, p := range t.players {
			p.Contributed = 0
		}
		return fmt.Errorf("hand: need at least 2 ready players, have %d", readyCount)
	}

	shuffle, err := deck.NewShuffle()
	if err != nil {
		return fmt.Errorf("hand: shuffle: %w", err)
	}
	t.shuffle = shuffle
	t.nextCard = 0
	t.board = nil
	t.boardTwo = nil
	t.boardSplitAt = 0
	t.runItTwice = false
	t.runoutPhase = 0
	t.payouts = nil
	t.lastOutcome = nil
	t.wasEverAllIn = make(map[string]bool)
	// Backstop: a request nobody answered must never survive into the next
	// hand still holding the requester's chips. Runs before rakeCollected is
	// zeroed so an orphaned fee (requester already gone) is booked against the
	// hand that produced it.
	t.refundPendingWinnerCards()
	t.rakeCollected = 0
	t.rabbitHuntPaid = make(map[string]bool)
	t.winnerCardsPaid = make(map[string]bool)
	t.winnerCardsAsked = make(map[string]bool)
	t.seenActionIDs = make(map[string]bool)
	for _, p := range t.players {
		p.VoluntarilyShown = false
		p.VoluntarilyShownCards = [2]bool{}
		p.HandStartStack = nil
	}

	active := make([]*Player, 0, len(t.players))
	newEntrants := make(map[string]bool)
	for _, p := range t.players {
		initializeTimeBank(p)
		if !t.eligibleForNextHand(p) {
			if p.State != PendingEntry {
				p.State = SittingOut
			}
			// A player sitting out this hand (e.g. busted all-in awaiting
			// rebuy) must not keep broadcasting their last hand's bet chips
			// on the felt once the next hand has started.
			p.Contributed = 0
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
		startingStack := p.Stack
		p.HandStartStack = &startingStack
		p.State = Active
		p.Contributed = 0
		if *p.TimeBankMs < DefaultTimeBankMs {
			recharged := *p.TimeBankMs + TimeBankRechargeMs
			if recharged > DefaultTimeBankMs {
				recharged = DefaultTimeBankMs
			}
			*p.TimeBankMs = recharged
		}
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
	dealerIdx := t.dealerIndexWithin(active)
	for pass := 0; pass < 2; pass++ {
		for offset := 1; offset <= len(active); offset++ {
			active[(dealerIdx+offset)%len(active)].HoleCards[pass] = t.dealCard()
		}
	}
	for _, p := range active {
		// A new entrant who lands in the SB or BB seat this very hand is
		// already posting a blind above via the normal seat-based post — an
		// additional "pay a big blind to enter" charge on top would leave
		// them contributed more than t.round.CurrentBet (still just
		// t.bigBlind, set below), which currentPlayerCanAct/IsComplete can
		// never resolve via a check: both require Contributed to exactly
		// equal CurrentBet, so that seat would stay "current" forever and
		// the hand would hang.
		if newEntrants[p.ID] && p != active[bbSeat] && p != active[sbSeat] {
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

// TimeBankForActor returns the durable balance for a seat. It deliberately
// initializes legacy nil values once, while preserving an exhausted zero.
func (t *Table) TimeBankForActor(playerID string) int64 {
	p := t.playerByID(playerID)
	if p == nil {
		return 0
	}
	initializeTimeBank(p)
	return *p.TimeBankMs
}

// ConsumeTimeBankForActor deducts elapsed bank time and returns the new
// balance. Actor serialization makes the mutation race-free; DynamoDB's
// version condition makes it safe across server instances.
func (t *Table) ConsumeTimeBankForActor(playerID string, elapsedMs int64) int64 {
	p := t.playerByID(playerID)
	if p == nil {
		return 0
	}
	initializeTimeBank(p)
	if elapsedMs < 0 {
		elapsedMs = 0
	}
	if elapsedMs > *p.TimeBankMs {
		elapsedMs = *p.TimeBankMs
	}
	*p.TimeBankMs -= elapsedMs
	return *p.TimeBankMs
}

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

// RevealHoleCard lets a participant reveal either card independently. A nil
// index retains compatibility with the legacy show_cards command and reveals
// both. changed=false is an idempotent no-op and must not create another
// persisted action/version.
func (t *Table) RevealHoleCard(playerID string, cardIndex *int32) (changed bool, err error) {
	if t.stage != Complete {
		return false, fmt.Errorf("hand: cards can only be revealed after the hand is complete")
	}
	dealtIn := false
	for _, hp := range t.handOrder {
		if hp.ID == playerID {
			dealtIn = true
			break
		}
	}
	if !dealtIn {
		return false, fmt.Errorf("hand: player %s was not dealt into this hand", playerID)
	}
	p := t.playerByID(playerID)
	if p == nil {
		return false, fmt.Errorf("hand: player %s is no longer seated", playerID)
	}
	if cardIndex == nil {
		changed = !p.VoluntarilyShownCards[0] || !p.VoluntarilyShownCards[1]
		p.VoluntarilyShownCards = [2]bool{true, true}
		p.VoluntarilyShown = true
		if t.lastOutcome != nil {
			info := t.lastOutcome.PlayerHands[playerID]
			info.Revealed = true
			info.RevealedCards = [2]bool{true, true}
			t.lastOutcome.PlayerHands[playerID] = info
		}
		return changed, nil
	}
	if *cardIndex < 0 || *cardIndex > 1 {
		return false, fmt.Errorf("hand: card index %d is invalid", *cardIndex)
	}
	if p.VoluntarilyShown || p.VoluntarilyShownCards[*cardIndex] {
		return false, nil
	}
	p.VoluntarilyShownCards[*cardIndex] = true
	p.VoluntarilyShown = p.VoluntarilyShownCards[0] && p.VoluntarilyShownCards[1]
	if t.lastOutcome != nil {
		info := t.lastOutcome.PlayerHands[playerID]
		info.RevealedCards[*cardIndex] = true
		info.Revealed = info.RevealedCards[0] && info.RevealedCards[1]
		t.lastOutcome.PlayerHands[playerID] = info
	}
	return true, nil
}

// RevealHoleCards preserves the engine API used by older callers.
func (t *Table) RevealHoleCards(playerID string) error {
	_, err := t.RevealHoleCard(playerID, nil)
	return err
}

// RequestRabbitHunt charges playerID the current hand's big blind to reveal
// the runout that would have come after a hand ends without a showdown.
// Returns the fee charged. Fails without charging anything if the table
// isn't sandbox, the hand isn't eligible, the player wasn't dealt in, they
// already paid this hand, or their stack can't cover the fee.
func (t *Table) RequestRabbitHunt(playerID string) (fee int64, err error) {
	if t.currencyMode != "sandbox" {
		return 0, fmt.Errorf("hand: rabbit hunt is only available on sandbox tables")
	}
	if t.stage != Complete {
		return 0, fmt.Errorf("hand: rabbit hunt is only available after the hand is complete")
	}
	if t.lastOutcome == nil || !t.lastOutcome.WonWithoutShowdown {
		return 0, fmt.Errorf("hand: rabbit hunt is only available when the hand ended without a showdown")
	}
	if len(t.board) >= 5 {
		return 0, fmt.Errorf("hand: rabbit hunt is not available once the full board is dealt")
	}
	dealtIn := false
	for _, hp := range t.handOrder {
		if hp.ID == playerID {
			dealtIn = true
			break
		}
	}
	if !dealtIn {
		return 0, fmt.Errorf("hand: player %s was not dealt into this hand", playerID)
	}
	if t.rabbitHuntPaid[playerID] {
		return 0, fmt.Errorf("hand: player %s already paid for rabbit hunt this hand", playerID)
	}
	p := t.playerByID(playerID)
	if p == nil {
		return 0, fmt.Errorf("hand: player %s is no longer seated", playerID)
	}
	if p.Stack < t.bigBlind {
		return 0, fmt.Errorf("hand: insufficient stack for the rabbit hunt fee")
	}
	p.Stack -= t.bigBlind
	if t.rabbitHuntPaid == nil {
		t.rabbitHuntPaid = make(map[string]bool)
	}
	t.rabbitHuntPaid[playerID] = true
	return t.bigBlind, nil
}

// RefundRabbitHunt reverses a RequestRabbitHunt charge for playerID this
// hand, used when the client reports it couldn't verify the revealed
// runout. Fails if playerID never paid this hand (nothing to refund).
func (t *Table) RefundRabbitHunt(playerID string) error {
	if !t.rabbitHuntPaid[playerID] {
		return fmt.Errorf("hand: player %s has no rabbit hunt payment to refund this hand", playerID)
	}
	p := t.playerByID(playerID)
	if p == nil {
		return fmt.Errorf("hand: player %s is no longer seated", playerID)
	}
	p.Stack += t.bigBlind
	delete(t.rabbitHuntPaid, playerID)
	return nil
}

// WinnerCardsRequest is one outstanding paid-reveal request: the requester
// has already been charged, and the winner has until ExpiresAt to accept or
// decline. Exported because it is persisted in State and surfaced, viewer-
// scoped, on the snapshot.
type WinnerCardsRequest struct {
	RequesterID string `json:"requester_id" dynamodbav:"requester_id"`
	WinnerID    string `json:"winner_id" dynamodbav:"winner_id"`
	Fee         int64  `json:"fee" dynamodbav:"fee"`
	// ExpiresAt is server-set (unix ms). Persisted rather than derived from a
	// timer so any instance that picks this table up can finish the request.
	ExpiresAt int64 `json:"expires_at" dynamodbav:"expires_at"`
}

// PendingWinnerCards returns this hand's outstanding paid-reveal request, or
// nil. The returned value is a copy — callers must not mutate table state.
func (t *Table) PendingWinnerCards() *WinnerCardsRequest {
	if t.pendingWinnerCards == nil {
		return nil
	}
	copied := *t.pendingWinnerCards
	return &copied
}

// refundPendingWinnerCards returns an unresolved request's fee and clears it.
// A requester who has since left the table cannot be paid back — their stack
// was already settled short — so the fee goes to rake, which is this table's
// "chips removed from play" accumulator and therefore the only bookkeeping
// that keeps the chip count balanced.
func (t *Table) refundPendingWinnerCards() {
	if t.pendingWinnerCards == nil {
		return
	}
	if requester := t.playerByID(t.pendingWinnerCards.RequesterID); requester != nil {
		requester.Stack += t.pendingWinnerCards.Fee
	} else {
		t.rakeCollected += t.pendingWinnerCards.Fee
	}
	t.pendingWinnerCards = nil
}

// RequestWinnerCards charges playerID the current hand's big blind and asks
// the sole uncontested winner for permission to reveal their hole cards to
// that viewer only. Nothing is revealed and the winner is paid nothing until
// they accept; a decline or a timeout refunds the requester in full.
func (t *Table) RequestWinnerCards(playerID string, now time.Time) (fee int64, err error) {
	if t.currencyMode != "sandbox" {
		return 0, fmt.Errorf("hand: winner cards are only available on sandbox tables")
	}
	if t.stage != Complete {
		return 0, fmt.Errorf("hand: winner cards are only available after the hand is complete")
	}
	if t.lastOutcome == nil || !t.lastOutcome.WonWithoutShowdown {
		return 0, fmt.Errorf("hand: winner cards are only available when the hand ended without a showdown")
	}
	if len(t.lastOutcome.Winners) != 1 {
		return 0, fmt.Errorf("hand: winner cards require exactly one winner")
	}
	winnerID := t.lastOutcome.Winners[0]
	if playerID == winnerID {
		return 0, fmt.Errorf("hand: winner cannot pay to see their own cards")
	}
	winner := t.playerByID(winnerID)
	if winner == nil {
		return 0, fmt.Errorf("hand: winner %s is no longer seated", winnerID)
	}
	if winner.VoluntarilyShown || winner.VoluntarilyShownCards[0] || winner.VoluntarilyShownCards[1] {
		return 0, fmt.Errorf("hand: winner cards are already revealed")
	}
	dealtIn := false
	for _, hp := range t.handOrder {
		if hp.ID == playerID {
			dealtIn = true
			break
		}
	}
	if !dealtIn {
		return 0, fmt.Errorf("hand: player %s was not dealt into this hand", playerID)
	}
	if t.winnerCardsAsked[playerID] {
		return 0, fmt.Errorf("hand: player %s already asked to see winner cards this hand", playerID)
	}
	// One outstanding request per hand: a second one queues nothing, it is
	// rejected, so the winner is never asked two questions at once.
	t.expirePendingWinnerCards(now)
	if t.pendingWinnerCards != nil {
		return 0, fmt.Errorf("hand: a winner cards request is already pending")
	}
	requester := t.playerByID(playerID)
	if requester == nil {
		return 0, fmt.Errorf("hand: player %s is no longer seated", playerID)
	}
	if requester.Stack < t.bigBlind {
		return 0, fmt.Errorf("hand: insufficient stack for the winner cards fee")
	}
	requester.Stack -= t.bigBlind
	if t.winnerCardsAsked == nil {
		t.winnerCardsAsked = make(map[string]bool)
	}
	t.winnerCardsAsked[playerID] = true
	t.pendingWinnerCards = &WinnerCardsRequest{
		RequesterID: playerID, WinnerID: winnerID, Fee: t.bigBlind,
		ExpiresAt: now.Add(WinnerCardsConsentWindow).UnixMilli(),
	}
	return t.bigBlind, nil
}

// AcceptWinnerCards is the winner agreeing to show. Only now does the fee
// actually move: half to the winner, the rest to rake — the same split the
// unilateral version used to apply at request time.
func (t *Table) AcceptWinnerCards(winnerID string, now time.Time) error {
	t.expirePendingWinnerCards(now)
	req := t.pendingWinnerCards
	if req == nil {
		return fmt.Errorf("hand: no winner cards request is pending")
	}
	if req.WinnerID != winnerID {
		return fmt.Errorf("hand: player %s is not the winner this request is addressed to", winnerID)
	}
	winner := t.playerByID(winnerID)
	if winner == nil {
		// The winner left between the request and their answer; nobody can be
		// paid, so the requester gets their chips back instead.
		t.refundPendingWinnerCards()
		return fmt.Errorf("hand: winner %s is no longer seated", winnerID)
	}
	winner.Stack += req.Fee / 2
	t.rakeCollected += req.Fee - req.Fee/2
	if t.winnerCardsPaid == nil {
		t.winnerCardsPaid = make(map[string]bool)
	}
	t.winnerCardsPaid[req.RequesterID] = true
	t.pendingWinnerCards = nil
	return nil
}

// DeclineWinnerCards is the winner refusing. Nothing is revealed and the
// requester is made whole. Unlike Accept, it does not check the window: a
// decline that lands just after expiry produces exactly the outcome expiry
// would have (full refund, nothing shown), so failing it would only report an
// error for something that already went the way the winner wanted.
func (t *Table) DeclineWinnerCards(winnerID string) error {
	req := t.pendingWinnerCards
	if req == nil {
		return fmt.Errorf("hand: no winner cards request is pending")
	}
	if req.WinnerID != winnerID {
		return fmt.Errorf("hand: player %s is not the winner this request is addressed to", winnerID)
	}
	t.refundPendingWinnerCards()
	return nil
}

// ExpireWinnerCards resolves an unanswered request whose window has closed.
// Reports whether anything changed, so the caller only persists a real
// mutation.
func (t *Table) ExpireWinnerCards(now time.Time) bool {
	before := t.pendingWinnerCards
	t.expirePendingWinnerCards(now)
	return before != nil && t.pendingWinnerCards == nil
}

// expirePendingWinnerCards refunds the request if its window has closed. Every
// entry point calls this first so a stale request can never be accepted just
// because no timer happened to fire on this instance.
func (t *Table) expirePendingWinnerCards(now time.Time) {
	if t.pendingWinnerCards == nil || now.UnixMilli() < t.pendingWinnerCards.ExpiresAt {
		return
	}
	t.refundPendingWinnerCards()
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
// pointer identity). If that seat is sitting out, the button advances to the
// first eligible seat clockwise rather than jumping to list index zero.
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
	for offset := 1; offset <= len(t.players); offset++ {
		candidate := t.players[(t.dealerSeat+offset)%len(t.players)]
		for i, p := range list {
			if p == candidate {
				return i
			}
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
	// Defensive bounds check: a 52-card shuffle can only be over-drawn if hand
	// progression itself is buggy. Panic with context (recovered by the table
	// Actor's handler loop, which then reloads authoritative state) instead of
	// a bare runtime index-out-of-range with no table/stage in the message.
	if t.nextCard < 0 || t.nextCard >= len(t.shuffle.Cards) {
		panic(fmt.Sprintf("hand: deal past end of shuffle (nextCard=%d, deck=%d, stage=%d)",
			t.nextCard, len(t.shuffle.Cards), t.stage))
	}
	c := t.shuffle.Cards[t.nextCard]
	t.nextCard++
	return c
}

func (t *Table) burnCard() { _ = t.dealCard() }

func (t *Table) dealFlop() {
	t.burnCard()
	t.board = append(t.board, t.dealCard(), t.dealCard(), t.dealCard())
}

func (t *Table) dealBoardCard() {
	t.burnCard()
	t.board = append(t.board, t.dealCard())
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
	// t.round/t.roundIdx are never reset on Complete, so they still hold the
	// finished hand's last betting round. That's normally harmless (everyone
	// left in it is Folded/AllIn/already-acted), but if id was since removed
	// from t.players entirely (RemovePlayerForActor, e.g. a disconnect-kick)
	// their leftover roundIdx entry must not be trusted — t.playerByID(id)
	// below would return nil and Act's p.State assignment would panic.
	if t.playerByID(id) == nil {
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
	current := t.currentPlayerToAct()
	if current == "" {
		return fmt.Errorf("hand: no player has a pending action")
	}
	if current != playerID {
		return fmt.Errorf("hand: it is not player %s's turn to act", playerID)
	}
	idx, ok := t.roundIdx[playerID]
	if !ok {
		return fmt.Errorf("hand: player %s has no pending action this round", playerID)
	}
	bs := t.round.Players[idx]

	action = t.NormalizedActionForActor(playerID, action)

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
		if t.runoutPhase == 0 && t.stage != River {
			t.runItTwice = t.shouldRunItTwice()
			t.boardSplitAt = len(t.board)
			t.runoutPhase = 1
		}
		t.AdvanceRunoutStreetForActor()
		return
	}

	switch t.stage {
	case PreFlop:
		t.dealFlop()
		t.stage = Flop
	case Flop:
		t.dealBoardCard()
		t.stage = Turn
	case Turn:
		t.dealBoardCard()
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
	// handOrder is immutable for the duration of a hand. t.players can also
	// contain a mid-hand joiner or a sitting-out player who has requested a
	// return and is already marked Active for the next deal; neither may
	// influence this hand's showdown/runout decisions.
	for _, p := range t.handOrder {
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
	if t.runItTwice && t.runoutPhase == 2 {
		switch t.stage {
		case PreFlop:
			t.burnCard()
			t.boardTwo = append(t.boardTwo, t.dealCard(), t.dealCard(), t.dealCard())
			t.stage = Flop
		case Flop, Turn:
			t.burnCard()
			t.boardTwo = append(t.boardTwo, t.dealCard())
			if t.stage == Flop {
				t.stage = Turn
			} else {
				t.stage = River
			}
		}
		if t.stage == River {
			t.runShowdown()
		}
		return
	}
	switch t.stage {
	case PreFlop:
		t.dealFlop()
		t.stage = Flop
	case Flop:
		t.dealBoardCard()
		t.stage = Turn
	case Turn:
		t.dealBoardCard()
		t.stage = River
	}
	if t.stage == River {
		if t.runItTwice {
			t.runoutPhase = 2
			switch t.boardSplitAt {
			case 0:
				t.stage = PreFlop
			case 3:
				t.stage = Flop
			case 4:
				t.stage = Turn
			default:
				t.runShowdown()
			}
			return
		}
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
	if t.stage != Flop && t.stage != Turn && !(t.runItTwice && t.runoutPhase == 2 && t.stage == PreFlop) {
		return false
	}
	// canStillAct <= 1 alone isn't enough: it goes true the instant a player
	// shoves all-in, one Act() call before the other player (still Active,
	// still owing a call) has had any chance to respond. Without this gate,
	// the paced runout timer starts dealing streets straight to showdown
	// while that call/fold decision is still outstanding — the facing
	// player's action is silently skipped. Only treat it as a genuine
	// all-in runout once the live betting round has actually closed.
	if t.round != nil && !t.round.IsComplete() {
		return false
	}
	remaining, canStillAct := t.countRemainingAndActable()
	return remaining > 1 && canStillAct <= 1
}

func (t *Table) activePlayers() []*Player {
	// Later streets may only contain players dealt into this hand. Iterating
	// the broader seated-player list here used to insert an Active-for-next-
	// hand returner into Round while actionScanOrder (correctly) excluded
	// them via handOrder. Once every dealt player checked, Round.IsComplete
	// waited on that unreachable ghost actor and current_player_id became
	// empty forever.
	out := make([]*Player, 0, len(t.handOrder))
	for _, p := range t.handOrder {
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
			contributions = append(contributions, sidepots.Contribution{
				PlayerID: p.ID, Amount: p.Contributed, Folded: p.State == Folded,
			})
		}
	}
	layers := sidepots.ComputeSidePots(contributions)

	payouts := make(map[string]int64)
	winningIDs := make([]string, 0)
	potResults := make([]PotResult, 0, len(layers))
	var winningScore handeval.Score
	remainingRakeCap := t.rakeCap()
	// deadWinnerlessCarry accumulates chips from a layer that resolves with
	// zero winners (see below) so they roll forward into the next layer that
	// actually has one, per the folded-money-is-dead-money ruling — never
	// refunded to their contributors.
	var deadWinnerlessCarry int64
	for _, layer := range layers {
		if layer.Uncalled {
			// Only one player ever put chips into this layer — it's an
			// uncalled bet, not a contested pot. Nobody else ever had a
			// stake in it, so it's never a "win": no rake, and the sole
			// contributor must not land in Winners/ComebackWinners (that
			// falsely fires win/comeback achievements for a player who lost
			// their actual showdown and just got their own excess back).
			// (Eligible normally holds exactly that one player; the pooled
			// everyone-folded fallback splits evenly.)
			n := int64(len(layer.Eligible))
			share := layer.Amount / n
			layerPayouts := make(map[string]int64, len(layer.Eligible))
			for _, id := range layer.Eligible {
				payouts[id] += share
				layerPayouts[id] += share
			}
			if remainder := layer.Amount - share*n; remainder > 0 {
				payouts[layer.Eligible[0]] += remainder
				layerPayouts[layer.Eligible[0]] += remainder
			}
			potResults = append(potResults, PotResult{
				Amount: layer.Amount, PayoutAmount: layer.Amount,
				EligiblePlayerIDs: append([]string(nil), layer.Eligible...),
				Payouts:           layerPayouts,
				Refund:            true,
			})
			continue
		}
		winners, eligible, bestScore := t.evaluateLayer(layer, t.board)
		if len(winners) == 0 {
			// This should be unreachable: sidepots.ComputeSidePots never puts
			// a folded player in a contested layer's Eligible (it rolls a
			// fully-folded band into dead money itself, before this layer is
			// even built), and RemovePlayerForActor refuses to remove anyone
			// still in t.handOrder for the entire hand (stage stays
			// != WaitingForPlayers/Complete until the very end of this
			// function) — so every id in layer.Eligible resolves to a live,
			// non-folded *Player here. If it is somehow reached anyway,
			// per the folded-money-is-dead-money ruling these chips are
			// still not "refundable": they were called money, not an
			// uncalled bet, so they must roll forward as dead money into
			// whichever later layer actually has a winner (that layer's
			// normal rake still applies to them there) rather than being
			// split back to their contributors.
			deadWinnerlessCarry += layer.Amount
			continue
		}
		if deadWinnerlessCarry > 0 {
			layer.Amount += deadWinnerlessCarry
			deadWinnerlessCarry = 0
		}
		layerRake := t.rakeForLayer(layer.Amount, remainingRakeCap)
		remainingRakeCap -= layerRake
		t.rakeCollected += layerRake
		netAmount := layer.Amount - layerRake
		award := func(amount int64, runout int, runoutWinners, runoutEligible []string, score handeval.Score) {
			if amount > 0 {
				winningIDs = append(winningIDs, runoutWinners...)
				if score > winningScore {
					winningScore = score
				}
			}
			share := amount / int64(len(runoutWinners))
			layerPayouts := make(map[string]int64, len(runoutWinners))
			for _, w := range runoutWinners {
				payouts[w] += share
				layerPayouts[w] += share
			}
			remainder := amount - share*int64(len(runoutWinners))
			if remainder > 0 {
				oddChipWinner := t.oddChipWinner(runoutWinners)
				payouts[oddChipWinner] += remainder
				layerPayouts[oddChipWinner] += remainder
			}
			potResults = append(potResults, PotResult{
				Amount: layer.Amount, PayoutAmount: amount, Runout: runout,
				EligiblePlayerIDs: append([]string(nil), runoutEligible...),
				Winners:           append([]string(nil), runoutWinners...), Payouts: layerPayouts,
			})
		}
		if t.runItTwice && len(t.boardTwo) > 0 {
			// The odd chip created by halving belongs to runout one. Rake has
			// already been charged once against the full layer above.
			firstAmount := netAmount/2 + netAmount%2
			award(firstAmount, 1, winners, eligible, bestScore)
			secondBoard := append(append([]deck.Card(nil), t.board[:t.boardSplitAt]...), t.boardTwo...)
			secondWinners, secondEligible, secondScore := t.evaluateLayer(layer, secondBoard)
			award(netAmount/2, 2, secondWinners, secondEligible, secondScore)
		} else {
			award(netAmount, 0, winners, eligible, bestScore)
		}
	}
	if deadWinnerlessCarry > 0 && len(winningIDs) > 0 {
		// Every layer built by ComputeSidePots resolved winnerless (see the
		// comment above — provably unreachable today) and none of the later
		// layers had a winner to absorb the carry either. As an ultimate
		// fallback, still never refund it: hand it to the hand's actual
		// winner rather than to the (now-nil) contributors.
		fallbackWinner := winningIDs[len(winningIDs)-1]
		payouts[fallbackWinner] += deadWinnerlessCarry
	}
	for id, amount := range payouts {
		// A payout recipient who left t.players entirely before showdown
		// resolved (RemovePlayerForActor allows removal once stage reaches
		// WaitingForPlayers/Complete, but never clears t.handOrder — a stale
		// handOrder entry can outlive that removal until the next StartHand)
		// has nowhere left to credit these chips. Their own stack was
		// already cashed out at removal time, before this outcome was known,
		// so the amount is an orphaned artifact of the race rather than
		// money either side is still owed — drop it rather than panic.
		if p := t.playerByID(id); p != nil {
			p.Stack += amount
		}
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
		PotResults:         potResults,
	}
	if t.shuffle != nil {
		outcome.ServerSeed = hex.EncodeToString(t.shuffle.ServerSeed[:])
		outcome.CommitHash = hex.EncodeToString(t.shuffle.CommitHash[:])
		rootCommit := deck.RootCommitHash(t.shuffle.ServerSeed, t.shuffle.Cards)
		outcome.RootCommitHash = hex.EncodeToString(rootCommit[:])
	}
	if !wonWithoutShowdown {
		outcome.WinningCategory = categoryNames[winningScore.Category()]
		winnerSet := make(map[string]bool, len(outcome.Winners))
		for _, w := range outcome.Winners {
			winnerSet[w] = true
		}
		outcome.ShowdownResults = make(map[string]ShowdownResult, len(t.handOrder))
		for _, p := range t.handOrder {
			if p.State == Folded {
				continue
			}
			var full [7]deck.Card
			full[0], full[1] = p.HoleCards[0], p.HoleCards[1]
			copy(full[2:], t.board)
			playerScore := handeval.Best7(full)
			if t.runItTwice && len(t.boardTwo) > 0 {
				secondBoard := append(append([]deck.Card(nil), t.board[:t.boardSplitAt]...), t.boardTwo...)
				copy(full[2:], secondBoard)
				if secondScore := handeval.Best7(full); secondScore > playerScore {
					playerScore = secondScore
				}
			}
			won := winnerSet[p.ID]
			wonOutright, splitPot := false, false
			for _, result := range potResults {
				for _, winner := range result.Winners {
					if winner != p.ID {
						continue
					}
					if len(result.Winners) > 1 {
						splitPot = true
					} else {
						wonOutright = true
					}
				}
			}
			outcome.ShowdownResults[p.ID] = ShowdownResult{
				Category: categoryNames[playerScore.Category()],
				Won:      won,
				Tied:     won && splitPot && !wonOutright,
				SplitPot: splitPot,
			}
		}
	}
	for _, id := range outcome.Winners {
		if t.wasEverAllIn[id] {
			outcome.ComebackWinners = append(outcome.ComebackWinners, id)
		}
	}
	for id := range t.wasEverAllIn {
		outcome.AllInPlayers = append(outcome.AllInPlayers, id)
	}
	outcome.Board = boardCodes(t.board)
	if t.runItTwice && len(t.boardTwo) > 0 {
		outcome.BoardTwo = boardCodes(append(append([]deck.Card(nil), t.board[:t.boardSplitAt]...), t.boardTwo...))
	}
	outcome.PlayerHands = make(map[string]PlayerHandInfo, len(t.handOrder))
	for _, p := range t.handOrder {
		revealed := (!wonWithoutShowdown && p.State != Folded) || p.VoluntarilyShown
		outcome.PlayerHands[p.ID] = PlayerHandInfo{
			HoleCards:     [2]string{cardCode(p.HoleCards[0]), cardCode(p.HoleCards[1])},
			Revealed:      revealed,
			RevealedCards: [2]bool{revealed, revealed},
		}
	}
	t.lastOutcome = &outcome
	t.stage = Complete
	t.rotateDealer()
}

func (t *Table) evaluateLayer(layer sidepots.PotLayer, board []deck.Card) ([]string, []string, handeval.Score) {
	winners := make([]string, 0, len(layer.Eligible))
	eligible := make([]string, 0, len(layer.Eligible))
	var bestScore handeval.Score
	for _, id := range layer.Eligible {
		p := t.playerByID(id)
		if p == nil || p.State == Folded {
			continue
		}
		eligible = append(eligible, id)
		var full [7]deck.Card
		full[0], full[1] = p.HoleCards[0], p.HoleCards[1]
		copy(full[2:], board)
		score := handeval.Best7(full)
		switch {
		case score > bestScore:
			bestScore = score
			winners = []string{id}
		case score == bestScore:
			winners = append(winners, id)
		}
	}
	return winners, eligible, bestScore
}

func (t *Table) oddChipWinner(winners []string) string {
	if len(winners) == 0 {
		return ""
	}
	winnerSet := make(map[string]bool, len(winners))
	for _, id := range winners {
		winnerSet[id] = true
	}
	dealer := t.dealerIndexWithin(t.handOrder)
	for offset := 1; offset <= len(t.handOrder); offset++ {
		id := t.handOrder[(dealer+offset)%len(t.handOrder)].ID
		if winnerSet[id] {
			return id
		}
	}
	return winners[0]
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

// WinnerCardsConsentWindow is how long the winner has to accept or decline a
// paid-reveal request. Deliberately shorter than table.NextHandDelay (12s):
// the request lives entirely inside the post-hand window, so a longer one
// would routinely be cut off by the next deal — StartHand refunds in that
// case, but a window players can't actually use is worse than a tight one.
const WinnerCardsConsentWindow = 8 * time.Second
