// Package tablestore persists table state as a single DynamoDB item per
// table, guarded by a version counter — DynamoDB's conditional writes are
// the correctness mechanism (ARCHITECTURE.md §2, revised), not an in-memory
// lock or a Redis lease.
package tablestore

import (
	"errors"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

// ErrVersionConflict means another instance's action committed first —
// CommitAction's caller (table.Actor) must re-read the table's current
// state via LoadTable and retry validation against it.
var ErrVersionConflict = errors.New("tablestore: version conflict")

// ErrDuplicateAction means actionID was already committed for this hand —
// the caller should treat this the same as a successful no-op.
var ErrDuplicateAction = errors.New("tablestore: duplicate action_id")

// ActionLogEntry is one durable audit/hand-history record (ARCHITECTURE.md
// §8.2) — never read back for recovery; recovery reads StoredTable directly.
type ActionLogEntry struct {
	TableID  string `dynamodbav:"table_id"`
	HandID   string `dynamodbav:"hand_id"`
	Version  int    `dynamodbav:"version"`
	Seq      int    `dynamodbav:"seq,omitempty"`
	PlayerID string `dynamodbav:"player_id"`
	ActionID string `dynamodbav:"action_id"`
	Action   string `dynamodbav:"action"`
	// BettingAction is the normalized poker decision before Action is
	// decorated as "all_in". It distinguishes an all-in call from an all-in
	// raise for exact VPIP/PFR/3-bet accounting. Older rows omit it and are
	// handled conservatively by pokerstats.
	BettingAction string `dynamodbav:"betting_action,omitempty"`
	Amount        int64  `dynamodbav:"amount"`
	Timestamp     int64  `dynamodbav:"timestamp"` // unix millis, set by CommitAction
	// Frame is a public, hole-card-free projection of the authoritative table
	// immediately after this action. It makes recent hands replayable without
	// exposing the unredacted persisted hand.State.
	Frame *ReplayFrame `dynamodbav:"frame,omitempty"`
}

type ReplayFrame struct {
	Stage              string           `dynamodbav:"stage" json:"stage"`
	Board              []string         `dynamodbav:"board,omitempty" json:"board,omitempty"`
	Seats              []ReplaySeat     `dynamodbav:"seats,omitempty" json:"seats,omitempty"`
	CurrentPlayerID    string           `dynamodbav:"current_player_id,omitempty" json:"current_player_id,omitempty"`
	DealerPlayerID     string           `dynamodbav:"dealer_player_id,omitempty" json:"dealer_player_id,omitempty"`
	SmallBlindPlayerID string           `dynamodbav:"small_blind_player_id,omitempty" json:"small_blind_player_id,omitempty"`
	BigBlindPlayerID   string           `dynamodbav:"big_blind_player_id,omitempty" json:"big_blind_player_id,omitempty"`
	Pot                int64            `dynamodbav:"pot" json:"pot"`
	Payouts            map[string]int64 `dynamodbav:"payouts,omitempty" json:"payouts,omitempty"`
	Winners            []string         `dynamodbav:"winners,omitempty" json:"winners,omitempty"`
}

type ReplaySeat struct {
	PlayerID    string `dynamodbav:"player_id" json:"player_id"`
	Name        string `dynamodbav:"name,omitempty" json:"name,omitempty"`
	Stack       int64  `dynamodbav:"stack" json:"stack"`
	State       string `dynamodbav:"state" json:"state"`
	Contributed int64  `dynamodbav:"contributed" json:"contributed"`
	DealtIn     bool   `dynamodbav:"dealt_in" json:"dealt_in"`
}

// StoredTable is the current authoritative state of one table, as read from
// poker_table_state.
type StoredTable struct {
	TableID string     `dynamodbav:"pk"`
	Version int        `dynamodbav:"version"`
	HandID  string     `dynamodbav:"hand_id"`
	State   hand.State `dynamodbav:"state"`
	// TurnDeadlineUnixMs is the current player's absolute action deadline
	// (unix millis), committed atomically with the state that made them
	// current. It lives here, not inside hand.State, because it is wall-clock
	// actor bookkeeping, not game state — hand.State has no time.Time fields
	// by design. Any Actor that (re)loads this table (a fresh instance after
	// node restart/lease handoff/autoscale-in, or just another instance's
	// connection landing here first — ARCHITECTURE.md §2 lets any node serve
	// any table) uses this to resume the true remaining time instead of
	// granting the current player a fresh full turnTimeout window, which is
	// what a bare in-process deadline would do since nothing else persists
	// when a turn actually started. Zero means no active turn.
	TurnDeadlineUnixMs int64 `dynamodbav:"turn_deadline_unix_ms,omitempty"`
	LastActionAt       int64 `dynamodbav:"last_action_at"`
	Archived           bool  `dynamodbav:"archived,omitempty"`
}
