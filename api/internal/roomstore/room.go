// api/internal/roomstore/room.go
package roomstore

// Room is the lobby directory entry — metadata only. Live seat/stack state
// during play lives in Phase 2's table.Actor + snapshot/action-log, not here.
type Room struct {
	ID                   string           `dynamodbav:"room_id" json:"room_id"`
	Visibility           string           `dynamodbav:"visibility" json:"visibility"`       // "public" | "private"
	CurrencyMode         string           `dynamodbav:"currency_mode" json:"currency_mode"` // "sandbox" only, this plan
	SmallBlind           int64            `dynamodbav:"small_blind" json:"small_blind"`
	BigBlind             int64            `dynamodbav:"big_blind" json:"big_blind"`
	MaxSeats             int              `dynamodbav:"max_seats" json:"max_seats"` // 6 or 9
	BuyInMin             int64            `dynamodbav:"buy_in_min" json:"buy_in_min"`
	BuyInMax             int64            `dynamodbav:"buy_in_max" json:"buy_in_max"`
	ShareCode            string           `dynamodbav:"share_code,omitempty" json:"share_code,omitempty"`             // private rooms only
	BlindEscalation      *BlindEscalation `dynamodbav:"blind_escalation,omitempty" json:"blind_escalation,omitempty"` // private rooms only
	TurnTimeoutSeconds   int              `dynamodbav:"turn_timeout_seconds,omitempty" json:"turn_timeout_seconds,omitempty"` // private rooms only, 0 = default
	EquityDisplayEnabled bool             `dynamodbav:"equity_display_enabled" json:"equity_display_enabled"`
	Status               string           `dynamodbav:"status" json:"status"` // "waiting" | "active"
	// SeatsTaken mirrors the table actor's live occupied-seat count, written
	// through on every join/leave commit (table.Actor's onSeatsChanged hook via
	// tablemanager). Never computed live from tablemanager at read time — the
	// lobby list must work fleet-wide without touching in-memory actor state.
	SeatsTaken int    `dynamodbav:"seats_taken" json:"seats_taken"`
	CreatedBy  string `dynamodbav:"created_by" json:"created_by"`
	CreatedAt  string `dynamodbav:"created_at" json:"created_at"` // RFC3339Nano, see dynamo.NowStr()
}

type BlindEscalation struct {
	IntervalMinutes int   `dynamodbav:"interval_minutes" json:"interval_minutes"`
	Multiplier      int   `dynamodbav:"multiplier" json:"multiplier"` // whole-number percent, e.g. 150 = ×1.5
	Max             int64 `dynamodbav:"max" json:"max"`
}
