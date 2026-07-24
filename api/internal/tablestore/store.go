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
	Amount   int64  `dynamodbav:"amount"`
}

// StoredTable is the current authoritative state of one table, as read from
// poker_table_state.
type StoredTable struct {
	TableID      string     `dynamodbav:"pk"`
	Version      int        `dynamodbav:"version"`
	HandID       string     `dynamodbav:"hand_id"`
	State        hand.State `dynamodbav:"state"`
	LastActionAt int64      `dynamodbav:"last_action_at"`
	Archived     bool       `dynamodbav:"archived,omitempty"`
}
