package hand

import (
	"encoding/hex"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval/ref"
	"gopkg.aoctech.app/poker/api/internal/engine/sidepots"
)

// Snapshot is the wire-safe view of a Table for exactly one viewer. Building
// it here (not in a networking package) is what makes "never leak another
// player's hole cards" a single-source-of-truth guarantee instead of a
// convention every caller has to remember.
type Snapshot struct {
	Stage        string           `json:"stage"`
	Board        []string         `json:"board"`
	BoardTwo     []string         `json:"board_two,omitempty"`
	BoardSplitAt int              `json:"board_split_at,omitempty"`
	Seats        []SeatView       `json:"seats"`
	Payouts      map[string]int64 `json:"payouts,omitempty"`
	// Winners lists who actually won a contested pot this hand, as opposed to
	// merely appearing in Payouts — a payout also fires for an uncalled
	// all-in's excess or an orphaned side-pot refund (runShowdown), neither of
	// which is a win. The client must use this, not "payout > 0", to decide
	// who gets the win banner/pill.
	Winners                  []string                 `json:"winners,omitempty"`
	Rake                     int64                    `json:"rake,omitempty"`
	CurrentPlayerID          string                   `json:"current_player_id,omitempty"`
	LegalActions             *LegalActions            `json:"legal_actions,omitempty"`
	ActionDeadlineUnixMs     int64                    `json:"action_deadline_unix_ms,omitempty"`
	ActionBaseDeadlineUnixMs int64                    `json:"action_base_deadline_unix_ms,omitempty"`
	NextHandUnixMs           int64                    `json:"next_hand_unix_ms,omitempty"`
	IdleRemovalUnixMs        int64                    `json:"idle_removal_unix_ms,omitempty"`
	WonWithoutShowdown       bool                     `json:"won_without_showdown,omitempty"`
	ShuffleCommitHash        string                   `json:"shuffle_commit_hash,omitempty"`
	ShuffleServerSeedHex     string                   `json:"shuffle_server_seed_hex,omitempty"`
	RootCommitHash           string                   `json:"root_commit_hash,omitempty"`
	RevealedCardSalts        map[int]RevealedSaltView `json:"revealed_card_salts,omitempty"`
	UnrevealedCardHashes     map[int]string           `json:"unrevealed_card_hashes,omitempty"`
	RunoutCards              []string                 `json:"runout_cards,omitempty"`
	SmallBlindPlayerID       string                   `json:"small_blind_player_id,omitempty"`
	BigBlindPlayerID         string                   `json:"big_blind_player_id,omitempty"`
	DealerPlayerID           string                   `json:"dealer_player_id,omitempty"`
	SnapshotVersion          uint64                   `json:"snapshot_version,omitempty"`
	Pots                     []PotView                `json:"pots,omitempty"`
	PotResults               []PotResultView          `json:"pot_results,omitempty"`
	HandID                   string                   `json:"hand_id,omitempty"`
	ChatMessages             []ChatMessageView        `json:"chat_messages,omitempty"`
	Reactions                []ReactionView           `json:"reactions,omitempty"`
	ActionPreselection       string                   `json:"action_preselection,omitempty"`
	ActionPreselectionAmount int64                    `json:"action_preselection_amount,omitempty"`
	ProspectiveCallAmount    int64                    `json:"prospective_call_amount,omitempty"`
}

type RevealedSaltView struct {
	Card    string `json:"card"`
	SaltHex string `json:"salt_hex"`
}

type ChatMessageView struct {
	ID        string `json:"id"`
	PlayerID  string `json:"player_id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

type ReactionView struct {
	ID             string `json:"id"`
	PlayerID       string `json:"player_id"`
	ReactionID     string `json:"reaction_id"`
	TargetPlayerID string `json:"target_player_id,omitempty"`
	Timestamp      int64  `json:"timestamp"`
	ExpiresAt      int64  `json:"expires_at"`
}

// LegalActions is the authoritative set of moves the viewer may make right
// now, with the chip math the UI needs to render the raise control. The server
// is the single source of truth — the client must not derive these itself.
type LegalActions struct {
	Actions             []string `json:"actions"`      // subset of fold|check|call|raise
	CallAmount          int64    `json:"call_amount"`  // chips owed to call (0 when a check is available)
	MinRaiseTo          int64    `json:"min_raise_to"` // smallest total bet a raise may reach
	MaxRaiseTo          int64    `json:"max_raise_to"` // largest total bet (all-in): viewer stack + already contributed
	Step                int64    `json:"step"`         // raise increment for the + / - stepper
	CurrentContribution int64    `json:"current_contribution"`
	CurrentBet          int64    `json:"current_bet"`
	OneThirdPotRaiseTo  int64    `json:"one_third_pot_raise_to"`
	HalfPotRaiseTo      int64    `json:"half_pot_raise_to"`
	TwoThirdsPotRaiseTo int64    `json:"two_thirds_pot_raise_to"`
	PotRaiseTo          int64    `json:"pot_raise_to"`
}

type PotView struct {
	Amount            int64    `json:"amount"`
	EligiblePlayerIDs []string `json:"eligible_player_ids"`
}

type PotResultView struct {
	Amount            int64            `json:"amount"`
	PayoutAmount      int64            `json:"payout_amount"`
	EligiblePlayerIDs []string         `json:"eligible_player_ids"`
	WinnerPlayerIDs   []string         `json:"winner_player_ids,omitempty"`
	Payouts           map[string]int64 `json:"payouts,omitempty"`
	Refund            bool             `json:"refund,omitempty"`
	Runout            int              `json:"runout,omitempty"`
}

type SeatView struct {
	PlayerID        string `json:"player_id"`
	Name            string `json:"name,omitempty"`
	AvatarURL       string `json:"avatar_url,omitempty"`
	PlaystyleBadge  string `json:"playstyle_badge,omitempty"`
	ConnectionState string `json:"connection_state,omitempty"`
	Stack           int64  `json:"stack"`
	State           string `json:"state"`
	// DealtIn is true when this seat belongs to the hand identified by the
	// snapshot's HandID. Seat state is deliberately not used for this: a
	// player may become Active mid-hand while waiting for the next deal.
	DealtIn           bool     `json:"dealt_in"`
	Ready             bool     `json:"ready"`
	Contributed       int64    `json:"contributed"`
	HoleCards         []string `json:"hole_cards,omitempty"`
	HoleCardsRevealed []bool   `json:"hole_cards_revealed,omitempty"`
	StackAtHandStart  *int64   `json:"stack_at_hand_start,omitempty"`
	Equity            *float64 `json:"equity,omitempty"`
	HandCategory      string   `json:"hand_category,omitempty"`
	// HandScore is the server evaluator's canonical comparable strength.
	// Higher wins and equality is a real split; clients may render cards
	// locally, but must use this value for outcome decisions.
	HandScore  uint32 `json:"hand_score,omitempty"`
	TimeBankMs int64  `json:"time_bank_ms"`
	RunItTwice bool   `json:"run_it_twice,omitempty"`
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

func cloneInt64Map(source map[string]int64) map[string]int64 {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]int64, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
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
	wonWithoutShowdown := t.stage == Complete && t.lastOutcome != nil && t.lastOutcome.WonWithoutShowdown
	revealAll := t.stage == Complete && t.lastOutcome != nil && !wonWithoutShowdown
	var winners []string
	var potResults []PotResultView
	if t.stage == Complete && t.lastOutcome != nil {
		winners = t.lastOutcome.Winners
		potResults = make([]PotResultView, len(t.lastOutcome.PotResults))
		for i, result := range t.lastOutcome.PotResults {
			potResults[i] = PotResultView{
				Amount:            result.Amount,
				PayoutAmount:      result.PayoutAmount,
				EligiblePlayerIDs: append([]string(nil), result.EligiblePlayerIDs...),
				WinnerPlayerIDs:   append([]string(nil), result.Winners...),
				Payouts:           cloneInt64Map(result.Payouts),
				Refund:            result.Refund,
				Runout:            result.Runout,
			}
		}
	}
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
			PlayerID:         p.ID,
			Name:             p.Name,
			AvatarURL:        p.AvatarURL,
			PlaystyleBadge:   p.PlaystyleBadge,
			Stack:            p.Stack,
			State:            playerStateNames[p.State],
			DealtIn:          dealtIn[p.ID],
			Ready:            p.Ready,
			Contributed:      p.Contributed,
			StackAtHandStart: p.HandStartStack,
			TimeBankMs:       t.TimeBankForActor(p.ID),
		}
		if p.ID == viewerID {
			sv.RunItTwice = p.RunItTwice
		}
		publicReveal := []bool{
			p.VoluntarilyShown || p.VoluntarilyShownCards[0],
			p.VoluntarilyShown || p.VoluntarilyShownCards[1],
		}
		if revealAll && p.State != Folded {
			publicReveal[0], publicReveal[1] = true, true
		}
		sv.HoleCardsRevealed = publicReveal
		if dealtIn[p.ID] && (p.ID == viewerID || publicReveal[0] || publicReveal[1]) {
			sv.HoleCards = []string{"back", "back"}
			for i := range sv.HoleCards {
				if p.ID == viewerID || publicReveal[i] {
					sv.HoleCards[i] = cardCode(p.HoleCards[i])
				}
			}
			evaluationBoard := t.board
			if t.runItTwice && t.runoutPhase == 2 {
				evaluationBoard = append(append([]deck.Card(nil), t.board[:t.boardSplitAt]...), t.boardTwo...)
			}
			if p.ID == viewerID || (publicReveal[0] && publicReveal[1]) {
				if len(evaluationBoard) == 5 {
					var full [7]deck.Card
					full[0], full[1] = p.HoleCards[0], p.HoleCards[1]
					copy(full[2:], evaluationBoard)
					score := handeval.Best7(full)
					if t.stage == Complete && t.runItTwice && len(t.board) == 5 {
						copy(full[2:], t.board)
						if firstScore := handeval.Best7(full); firstScore > score {
							score = firstScore
						}
					}
					sv.HandCategory = categoryNames[score.Category()]
					sv.HandScore = uint32(score)
				} else if isBettingStage(t.stage) {
					// Pre-river the hand is still made of fewer than 7 cards,
					// which the perfect-hash tables cannot index — see
					// partialCategory. HandScore stays unset: only the 7-card
					// Score is comparable, and clients decide outcomes with it.
					// Only live betting stages get this: a hand that ENDED on a
					// short board (everyone folded) has no made hand worth
					// naming, and the client reads the absence of a category as
					// exactly that.
					sv.HandCategory = categoryNames[partialCategory(p.HoleCards, evaluationBoard)]
				}
			}
		}
		seats = append(seats, sv)
	}
	current := t.currentPlayerToAct()
	out := Snapshot{
		Stage:                 stageNames[t.stage],
		Board:                 boardCodes(t.board),
		BoardTwo:              boardCodes(t.boardTwo),
		BoardSplitAt:          t.boardSplitAt,
		Seats:                 seats,
		Payouts:               t.payouts,
		Winners:               winners,
		Rake:                  t.rakeCollected,
		CurrentPlayerID:       current,
		LegalActions:          t.legalActionsFor(viewerID, current),
		ProspectiveCallAmount: t.ProspectiveCallAmountForActor(viewerID),
		WonWithoutShowdown:    wonWithoutShowdown,
		Pots:                  t.potViews(),
		PotResults:            potResults,
	}
	if len(t.handOrder) >= 2 {
		sb, bb := t.blindSeats(t.handOrder)
		out.SmallBlindPlayerID = t.handOrder[sb].ID
		out.BigBlindPlayerID = t.handOrder[bb].ID
		out.DealerPlayerID = t.handOrder[t.dealerIndexWithin(t.handOrder)].ID
	}
	if t.shuffle != nil {
		out.ShuffleCommitHash = hex.EncodeToString(t.shuffle.CommitHash[:])
		rootCommit := deck.RootCommitHash(t.shuffle.ServerSeed, t.shuffle.Cards)
		out.RootCommitHash = hex.EncodeToString(rootCommit[:])

		if t.stage == Complete {
			proof, runout := t.fairnessProofFor(viewerID, wonWithoutShowdown)
			out.ShuffleServerSeedHex = proof.ServerSeedHex
			out.RevealedCardSalts = proof.RevealedCardSalts
			out.UnrevealedCardHashes = proof.UnrevealedCardHashes
			out.RunoutCards = runout
		}
	}
	return out
}

// fairnessProofFor builds viewerID's provably-fair proof for the just-completed
// hand. The full seed only comes out when nothing stays hidden (a real showdown
// with no folded hand); otherwise the viewer gets card+salt reveals for the
// positions they may see plus the committed hash of every position they may
// not, which still rebuilds RootCommitHash without leaking mucked cards. Second
// return is the rabbit-hunt runout (missing community cards) if any.
//
// Callers must hold t.stage == Complete and t.shuffle != nil.
func (t *Table) fairnessProofFor(viewerID string, wonWithoutShowdown bool) (FairnessProof, []string) {
	showFinalCards := !wonWithoutShowdown
	hasUnrevealedFold := wonWithoutShowdown
	for _, p := range t.handOrder {
		if p.State == Folded {
			hasUnrevealedFold = true
			break
		}
	}

	proof := FairnessProof{
		RevealedCardSalts:    make(map[int]RevealedSaltView),
		UnrevealedCardHashes: make(map[int]string),
	}
	if !hasUnrevealedFold {
		proof.ServerSeedHex = hex.EncodeToString(t.shuffle.ServerSeed[:])
	}
	var runout []string

	numActive := len(t.handOrder)
	holeTotal := numActive * 2
	dealerIdx := 0
	if numActive > 0 {
		dealerIdx = t.dealerIndexWithin(t.handOrder)
	}
	rabbitIndices := make(map[int]bool)
	if wonWithoutShowdown && len(t.board) < 5 {
		// Hole cards are followed by a burn+flop, burn+turn and
		// burn+river. Reveal only missing community-card positions;
		// burns and private cards remain committed hashes.
		flopStart := holeTotal + 1
		if len(t.board) < 3 {
			for i := range 3 {
				rabbitIndices[flopStart+i] = true
			}
		}
		if len(t.board) < 4 {
			rabbitIndices[holeTotal+5] = true
		}
		if len(t.board) < 5 {
			rabbitIndices[holeTotal+7] = true
		}
	}

	for i, c := range t.shuffle.Cards {
		cCode := cardCode(c)
		isRevealed := rabbitIndices[i]

		if numActive > 0 && i < holeTotal {
			pass := i / numActive
			offset := (i % numActive) + 1
			p := t.handOrder[(dealerIdx+offset)%numActive]

			if p.ID == viewerID || (showFinalCards && p.State != Folded) || p.VoluntarilyShownCards[pass] {
				isRevealed = true
			}
		} else if i < t.nextCard {
			isRevealed = true
		}

		if isRevealed {
			salt := deck.CardSalt(t.shuffle.ServerSeed, i)
			proof.RevealedCardSalts[i] = RevealedSaltView{
				Card:    cCode,
				SaltHex: hex.EncodeToString(salt[:]),
			}
			if rabbitIndices[i] {
				runout = append(runout, cCode)
			}
		} else {
			h := deck.CardHash(t.shuffle.ServerSeed, i, c)
			proof.UnrevealedCardHashes[i] = hex.EncodeToString(h[:])
		}
	}
	return proof, runout
}

// FairnessProofsForActor returns one fairness proof per dealt-in participant of
// the just-completed hand, keyed by player ID — the durable-history counterpart
// of what ViewFor puts on the live snapshot, so a player can still verify the
// deck from their match history after the table has moved on. Nil unless the
// hand is complete and a shuffle exists.
func (t *Table) FairnessProofsForActor() map[string]FairnessProof {
	if t.shuffle == nil || t.stage != Complete {
		return nil
	}
	wonWithoutShowdown := t.lastOutcome != nil && t.lastOutcome.WonWithoutShowdown
	out := make(map[string]FairnessProof, len(t.handOrder))
	for _, p := range t.handOrder {
		proof, _ := t.fairnessProofFor(p.ID, wonWithoutShowdown)
		out[p.ID] = proof
	}
	return out
}

// ProspectiveCallAmountForActor returns what playerID would owe if action
// reached them in the current betting round. Unlike LegalActions it is
// intentionally available before the player's turn, but only appears in that
// player's viewer-scoped snapshot.
func (t *Table) ProspectiveCallAmountForActor(playerID string) int64 {
	if !isBettingStage(t.stage) || t.round == nil {
		return 0
	}
	idx, ok := t.roundIdx[playerID]
	if !ok {
		return 0
	}
	player := t.round.Players[idx]
	if player.Folded || player.AllIn {
		return 0
	}
	return max(0, t.round.CurrentBet-player.Contributed)
}

func (t *Table) potViews() []PotView {
	contributions := make([]sidepots.Contribution, 0, len(t.handOrder))
	folded := make(map[string]bool, len(t.handOrder))
	for _, p := range t.handOrder {
		if p.Contributed > 0 {
			contributions = append(contributions, sidepots.Contribution{PlayerID: p.ID, Amount: p.Contributed})
		}
		folded[p.ID] = p.State == Folded
	}
	layers := sidepots.ComputeSidePots(contributions)
	out := make([]PotView, 0, len(layers))
	for _, layer := range layers {
		eligible := make([]string, 0, len(layer.Eligible))
		for _, id := range layer.Eligible {
			if !folded[id] {
				eligible = append(eligible, id)
			}
		}
		out = append(out, PotView{Amount: layer.Amount, EligiblePlayerIDs: eligible})
	}
	return out
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
	for _, id := range t.actionScanOrder() {
		if t.currentPlayerCanAct(id) {
			return id
		}
	}
	return ""
}

// actionScanOrder returns every player dealt into this hand (t.handOrder --
// stable for the whole hand, unlike t.roundIdx which drops folded players
// street to street) rotated to start at the seat that must act first this
// street: left of the big blind pre-flop, left of the button post-flop
// (heads-up's button IS the small blind, so post-flop "left of the button"
// already resolves to the big blind seat -- no extra heads-up special case
// needed there; blindSeats already special-cases heads-up for the pre-flop
// assignment itself). currentPlayerCanAct (called by every consumer of this
// order) already filters out anyone folded/all-in/not in the current round,
// so a still-included-but-ineligible seat here is harmless.
//
// The anchor MUST be handOrder-relative, not roundIdx-relative: roundIdx is
// rebuilt fresh (and shrinks) at the start of every street via
// startBettingRound(t.activePlayers(), ...), which excludes anyone already
// folded. If the button itself had folded, computing the anchor against that
// shrunken set would make dealerIndexWithin's "not found" fallback silently
// default to index 0 of whatever remained -- an arbitrary seat, not the
// actual button -- corrupting the rotation for every other seat still in the
// hand. handOrder never shrinks mid-hand, so the button is always found at
// its real seat regardless of who has folded since.
//
// currentPlayerToAct previously scanned t.players in raw join order, which
// only happened to match real action order when the dealer draw/rotation put
// the correct first-to-act player at the lowest join-order index -- any other
// case (most hands, since the dealer rotates every hand while join order
// never changes) let the wrong seat act first, e.g. the big blind checking
// before the small blind/dealer had a chance to act heads-up. Recomputed
// fresh from persisted state alone (t.players + t.handOrder + t.dealerSeat +
// t.stage) on every call rather than cached, so an instance recovering
// mid-round needs no separate restore path for it.
func (t *Table) actionScanOrder() []string {
	// Membership is checked by ID, not pointer identity: t.players and
	// t.handOrder are the SAME *Player pointers only within one in-memory
	// Table built by a single StartHand call. Once a Table round-trips
	// through NewTableFromState (every real command after the first, via
	// table.Actor's ensureLoaded), State.Players and State.HandOrder are
	// decoded as independently-allocated structs -- comparing by pointer
	// would silently match nobody and empty out `active` every time.
	dealt := make(map[string]bool, len(t.handOrder))
	for _, p := range t.handOrder {
		dealt[p.ID] = true
	}
	active := make([]*Player, 0, len(t.handOrder))
	for _, p := range t.players {
		if dealt[p.ID] {
			active = append(active, p)
		}
	}
	n := len(active)
	if n == 0 {
		return nil
	}
	startIdx := (t.dealerIndexWithin(active) + 1) % n
	if t.stage == PreFlop {
		_, bbIdx := t.blindSeats(active)
		startIdx = (bbIdx + 1) % n
	}
	// Once the street has started, the next decision is always clockwise from
	// the last actor. The fixed street anchor above is only correct for the
	// first action. In particular, a full raise resets ActedSinceLastFullRaise
	// for players that acted earlier; scanning again from the anchor would let
	// one of them act twice before a later seat had answered the raise.
	if t.round != nil && t.round.LastActorID != "" {
		for i, p := range active {
			if p.ID == t.round.LastActorID {
				startIdx = (i + 1) % n
				break
			}
		}
	}
	order := make([]string, n)
	for i := 0; i < n; i++ {
		order[i] = active[(startIdx+i)%n].ID
	}
	return order
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
	la := &LegalActions{
		Actions:             []string{"fold"},
		CurrentContribution: bs.Contributed,
		CurrentBet:          t.round.CurrentBet,
	}
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
		la.Step = t.bigBlind
		if la.Step <= 0 {
			la.Step = 1
		}
		la.OneThirdPotRaiseTo = t.potFractionRaiseTo(bs, owed, minRaiseTo, maxTo, 1, 3)
		la.HalfPotRaiseTo = t.potFractionRaiseTo(bs, owed, minRaiseTo, maxTo, 1, 2)
		la.TwoThirdsPotRaiseTo = t.potFractionRaiseTo(bs, owed, minRaiseTo, maxTo, 2, 3)
		la.PotRaiseTo = t.potFractionRaiseTo(bs, owed, minRaiseTo, maxTo, 1, 1)
	}
	return la
}

func (t *Table) potFractionRaiseTo(
	bs *betting.PlayerState,
	owed, minRaiseTo, maxRaiseTo, numerator, denominator int64,
) int64 {
	if denominator <= 0 {
		return minRaiseTo
	}
	if owed < 0 {
		owed = 0
	}
	var pot int64
	for _, p := range t.handOrder {
		pot += p.Contributed
	}
	// A fraction-of-pot raise is sized after first matching the current bet:
	// raise-to = own street contribution + call + fraction*(pot + call).
	target := bs.Contributed + owed + (pot+owed)*numerator/denominator
	if target < minRaiseTo {
		target = minRaiseTo
	}
	if target > maxRaiseTo {
		target = maxRaiseTo
	}
	return target
}

// partialCategory names the best five-card hand a viewer holds before the
// river, so the client can label it on every street instead of only at
// showdown. handeval's tables are indexed by 7-card rank multisets and cannot
// answer for 5 or 6 cards, so this goes through the reference evaluator's
// combinatorial BestN. Only the Category crosses over: ref.Score is a
// different encoding from handeval.Score and the two must never be compared
// (the categories themselves are one enum by construction — ref defines the
// ordering the tables are generated from). Preflop there is no board and the
// only made hand two cards can be is a pocket pair.
func partialCategory(hole [2]deck.Card, board []deck.Card) handeval.Category {
	if len(board) < 3 {
		if hole[0].Rank == hole[1].Rank {
			return handeval.Pair
		}
		return handeval.HighCard
	}
	cards := make([]deck.Card, 0, 2+len(board))
	cards = append(cards, hole[0], hole[1])
	cards = append(cards, board...)
	return handeval.Category(ref.BestN(cards).Category())
}

// playerToActForTest returns the ID of whoever must act now — test-only
// alias for currentPlayerToAct so snapshot_test.go can drive a hand to
// completion without hardcoding seat order (which depends on
// dealerIndexWithin). Unlike currentPlayerToAct, it isn't gated on
// isBettingStage/t.round==nil, matching its previous behavior of scanning
// currentPlayerCanAct directly regardless of stage.
func (t *Table) playerToActForTest() string {
	for _, id := range t.actionScanOrder() {
		if t.currentPlayerCanAct(id) {
			return id
		}
	}
	return ""
}
