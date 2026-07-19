// Package tablestore persists the durable state a crashed table server needs
// to resume: the latest per-hand snapshot, and every validated action logged
// since it (log-before-broadcast, ARCHITECTURE.md § 3).
package tablestore

import "gopkg.aoctech.app/poker/api/internal/engine/hand"

// ActionLogEntry is one durable record of a validated player action, written
// before the resulting state is ever broadcast (ARCHITECTURE.md § 3).
type ActionLogEntry struct {
	TableID  string `json:"table_id" dynamodbav:"table_id"`
	HandID   string `json:"hand_id" dynamodbav:"hand_id"`
	Seq      int    `json:"seq" dynamodbav:"seq"`
	PlayerID string `json:"player_id" dynamodbav:"player_id"`
	ActionID string `json:"action_id" dynamodbav:"action_id"`
	Action   string `json:"action" dynamodbav:"action"`
	Amount   int64  `json:"amount" dynamodbav:"amount"`
}

// StoredSnapshot pairs a hand.Snapshot with the hand/seq it was captured at,
// so a recovering instance knows exactly which log entries to replay on top
// of it (only those with seq > Seq for the same HandID).
type StoredSnapshot struct {
	TableID string        `dynamodbav:"pk"`
	HandID  string        `dynamodbav:"hand_id"`
	Seq     int           `dynamodbav:"seq"`
	State   hand.Snapshot `dynamodbav:"state"`
}
