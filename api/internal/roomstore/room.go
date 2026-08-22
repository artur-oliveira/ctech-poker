// api/internal/roomstore/room.go
package roomstore

const (
	CurrencyModeSandbox = "sandbox"
	CurrencyModeReal    = "real"
)

// Room is the lobby directory entry — metadata only. Live seat/stack state
// during play lives in Phase 2's table.Actor + snapshot/action-log, not here.
type Room struct {
	ID           string `dynamodbav:"room_id" json:"room_id"`
	Visibility   string `dynamodbav:"visibility" json:"visibility"`       // "public" | "private"
	CurrencyMode string `dynamodbav:"currency_mode" json:"currency_mode"` // "sandbox" | "real" (real requires REAL_MONEY_ENABLED)
	SmallBlind   int64  `dynamodbav:"small_blind" json:"small_blind"`
	BigBlind     int64  `dynamodbav:"big_blind" json:"big_blind"`
	MaxSeats     int    `dynamodbav:"max_seats" json:"max_seats"` // 6 or 9
	BuyInMin     int64  `dynamodbav:"buy_in_min" json:"buy_in_min"`
	BuyInMax     int64  `dynamodbav:"buy_in_max" json:"buy_in_max"`
	// EntryFeeCents is the fixed real-money table-entry fee (BRL cents),
	// charged to every player who takes a seat (buyin.Service.BuyIn) — never
	// recomputed after creation, so a later change to the fee catalog never
	// retroactively changes an already-created room's fee. Always zero for
	// sandbox rooms (sandbox funds itself via rake instead).
	EntryFeeCents int64 `dynamodbav:"entry_fee_cents,omitempty" json:"entry_fee_cents,omitempty"`
	// Tier is the fee-catalog tier ("micro" | "low" | "mid" | "high") this
	// room's stake pair belongs to, recorded at creation so buyin's
	// entitlement rebind never has to re-derive it from blinds. Empty for
	// sandbox rooms (no entitlement).
	Tier                 string           `dynamodbav:"tier,omitempty" json:"tier,omitempty"`
	ShareCode            string           `dynamodbav:"share_code,omitempty" json:"share_code,omitempty"`                     // private rooms only
	BlindEscalation      *BlindEscalation `dynamodbav:"blind_escalation,omitempty" json:"blind_escalation,omitempty"`         // private rooms only
	TurnTimeoutSeconds   int              `dynamodbav:"turn_timeout_seconds,omitempty" json:"turn_timeout_seconds,omitempty"` // private rooms only, 0 = default
	EquityDisplayEnabled bool             `dynamodbav:"equity_display_enabled" json:"equity_display_enabled"`
	RunItTwiceEnabled    bool             `dynamodbav:"run_it_twice_enabled" json:"run_it_twice_enabled"`
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

// InviteGrant authorizes one player to open and join a private room without
// ever learning its share code. ExpiresAt is checked by the API because
// DynamoDB TTL removal is intentionally eventual.
type InviteGrant struct {
	RoomID    string `dynamodbav:"pk" json:"-"`
	SK        string `dynamodbav:"sk" json:"-"`
	PlayerID  string `dynamodbav:"player_id" json:"player_id"`
	EventID   string `dynamodbav:"event_id" json:"event_id"`
	ExpiresAt int64  `dynamodbav:"expires_at" json:"expires_at"`
	TTL       int64  `dynamodbav:"ttl" json:"-"`
}
