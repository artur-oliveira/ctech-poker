package hand

import (
	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/deck"
)

// State is a complete, unredacted mirror of every field Table carries — used
// only for persistence/reconstruction (tablestore.Store). Unlike Snapshot,
// which deliberately hides information a viewer must never see, State must
// never be sent to a client.
//
// State never grows across hands — StartHand replaces Board/Shuffle/Round/
// SeenActionIDs wholesale for the new hand, so the encoded size stays bounded
// regardless of how long a table has been played (well under DynamoDB's
// 400KB item limit even at a full 9-max table).
type State struct {
	Players       []*Player
	SmallBlind    int64
	BigBlind      int64
	DealerSeat    int
	DealerDrawn   bool
	Stage         Stage
	Board         []deck.Card
	Shuffle       *deck.ShuffleResult
	NextCard      int
	Round         *betting.Round
	RoundIdx      map[string]int
	RoundBaseline map[string]int64
	Payouts       map[string]int64
	RakeBPS       int64
	RakeCollected int64
	HandOrder     []*Player
	SeenActionIDs map[string]bool
	ReadyToPost   map[string]bool
	LastOutcome   *HandOutcome
	WasEverAllIn  map[string]bool
}

// ExportState captures every field this Table carries, for durable storage.
func (t *Table) ExportState() State {
	return State{
		Players:       t.players,
		SmallBlind:    t.smallBlind,
		BigBlind:      t.bigBlind,
		DealerSeat:    t.dealerSeat,
		DealerDrawn:   t.dealerDrawn,
		Stage:         t.stage,
		Board:         t.board,
		Shuffle:       t.shuffle,
		NextCard:      t.nextCard,
		Round:         t.round,
		RoundIdx:      t.roundIdx,
		RoundBaseline: t.roundBaseline,
		Payouts:       t.payouts,
		RakeBPS:       t.rakeBPS,
		RakeCollected: t.rakeCollected,
		HandOrder:     t.handOrder,
		SeenActionIDs: t.seenActionIDs,
		ReadyToPost:   t.readyToPost,
		LastOutcome:   t.lastOutcome,
		WasEverAllIn:  t.wasEverAllIn,
	}
}

// NewTableFromState rebuilds a Table from a previously exported State — the
// only recovery path this revision needs (ARCHITECTURE.md §3: "recovery is
// trivial", there is no log to replay).
func NewTableFromState(s State) *Table {
	return &Table{
		players:    s.Players,
		smallBlind: s.SmallBlind,
		bigBlind:   s.BigBlind,
		dealerSeat: s.DealerSeat,
		// Pre-Phase-3 snapshots have no DealerDrawn field. Any snapshot past
		// the initial waiting state necessarily already assigned a dealer, so
		// infer true to avoid re-drawing after recovery.
		dealerDrawn:   s.DealerDrawn || s.Stage != WaitingForPlayers,
		stage:         s.Stage,
		board:         s.Board,
		shuffle:       s.Shuffle,
		nextCard:      s.NextCard,
		round:         s.Round,
		roundIdx:      s.RoundIdx,
		roundBaseline: s.RoundBaseline,
		payouts:       s.Payouts,
		rakeBPS:       s.RakeBPS,
		rakeCollected: s.RakeCollected,
		handOrder:     s.HandOrder,
		seenActionIDs: s.SeenActionIDs,
		readyToPost:   s.ReadyToPost,
		lastOutcome:   s.LastOutcome,
		wasEverAllIn:  s.WasEverAllIn,
	}
}

// ActIdempotent applies action only if actionID hasn't been seen for this
// table since its last StartHand call (seenActionIDs resets there). applied
// = false, err = nil means the action_id was already seen and nothing
// changed; the caller (table.Actor) should treat this as "already
// committed", not as an error.
func (t *Table) ActIdempotent(actionID, playerID string, action betting.Action, amount int64) (applied bool, err error) {
	if t.seenActionIDs == nil {
		t.seenActionIDs = make(map[string]bool)
	}
	if t.seenActionIDs[actionID] {
		return false, nil
	}
	if err := t.Act(playerID, action, amount); err != nil {
		return false, err
	}
	t.seenActionIDs[actionID] = true
	return true, nil
}
