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
	OwesBigBlind  map[string]bool
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
		OwesBigBlind:  t.owesBigBlind,
		LastOutcome:   t.lastOutcome,
		WasEverAllIn:  t.wasEverAllIn,
	}
}

// NewTableFromState rebuilds a Table from a previously exported State — the
// only recovery path this revision needs (ARCHITECTURE.md §3: "recovery is
// trivial", there is no log to replay).
func NewTableFromState(s State) *Table {
	// s.Players and s.HandOrder decode as independently-allocated *Player
	// structs even for the same player — dynamodbav/JSON never preserves
	// pointer aliasing across two separate fields. Within one continuous
	// in-memory Table this aliasing holds (StartHand seeds handOrder with
	// the exact same pointers as players), so every mutation via
	// t.playerByID — which only ever searches t.players — is instantly
	// visible through t.handOrder too. Re-link handOrder here to point at
	// the matching players entries so that invariant is restored after a
	// reload. Without this, any consumer that reads a mutable field (most
	// importantly runShowdown's Contributed-based pot calculation) off
	// t.handOrder keeps seeing whatever value that player had AT THIS
	// RELOAD, forever, even as later Act() calls keep updating the
	// t.players copy — contributed chips can silently vanish from the pot
	// entirely once a mid-hand reload happens, which is a normal occurrence
	// on any version-conflict retry (e.g. two instances serving the same
	// table directly with no proxying — ARCHITECTURE.md §2).
	byID := make(map[string]*Player, len(s.Players))
	for _, p := range s.Players {
		byID[p.ID] = p
	}
	handOrder := make([]*Player, len(s.HandOrder))
	for i, p := range s.HandOrder {
		if linked, ok := byID[p.ID]; ok {
			handOrder[i] = linked
		} else {
			// Dealt into this hand but no longer seated (e.g. cashed out
			// after the hand completed) — no players entry to link to.
			handOrder[i] = p
		}
	}

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
		handOrder:     handOrder,
		seenActionIDs: s.SeenActionIDs,
		readyToPost:   s.ReadyToPost,
		owesBigBlind:  s.OwesBigBlind,
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
