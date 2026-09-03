package v1

import "gopkg.aoctech.app/poker/api/internal/roomstore"

type CreateRoomRequest struct {
	Visibility           string                     `json:"visibility"`
	CurrencyMode         string                     `json:"currency_mode,omitempty"` // "sandbox" (default) or "real"; real requires REAL_MONEY_ENABLED
	SmallBlind           int64                      `json:"small_blind"`
	BigBlind             int64                      `json:"big_blind"`
	MaxSeats             int                        `json:"max_seats"`
	BuyInMin             int64                      `json:"buy_in_min"`
	BuyInMax             int64                      `json:"buy_in_max"`
	EquityDisplayEnabled *bool                      `json:"equity_display_enabled,omitempty"`
	RunItTwiceEnabled    *bool                      `json:"run_it_twice_enabled,omitempty"`
	BlindEscalation      *roomstore.BlindEscalation `json:"blind_escalation,omitempty"`
	TurnTimeoutSeconds   *int                       `json:"turn_timeout_seconds,omitempty"`
}

type JoinRoomRequest struct {
	Amount         int64  `json:"amount"`
	ShareCode      string `json:"share_code,omitempty"` // required to join a private room (unless creator)
	IdempotencyKey string `json:"idem_key,omitempty"`   // stable per buy-in click; reused across network retries so a retry can't double-debit
	AutoRebuy      bool   `json:"auto_rebuy,omitempty"` // only meaningful on a fresh join; ignored on a rebuy of an existing seat
}

// JoinOrCreateRoomRequest is a bucket spec, not a room id: the client names
// the stakes/seats it wants and the server picks (or creates) the table and
// seats the player there, so two clients racing on the same lobby tile can
// never both navigate to a seat only one of them got (#76).
type JoinOrCreateRoomRequest struct {
	SmallBlind     int64  `json:"small_blind"`
	BigBlind       int64  `json:"big_blind"`
	MaxSeats       int    `json:"max_seats"`
	CurrencyMode   string `json:"currency_mode,omitempty"` // "sandbox" (default) or "real"
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idem_key,omitempty"`   // stable per click; a retry re-seats at the same table instead of a second one
	AutoRebuy      bool   `json:"auto_rebuy,omitempty"` // only meaningful on a fresh join
}

// JoinOrCreateRoomResponse tells the client exactly which table to open.
type JoinOrCreateRoomResponse struct {
	RoomID  string `json:"room_id"`
	Created bool   `json:"created"` // true when no open table in the bucket could seat the player
}

// RoomBucket is one lobby tile's server-computed availability, aggregated
// over EVERY page of the public index rather than the first one the client
// happened to fetch (#76).
type RoomBucket struct {
	SmallBlind     int64  `json:"small_blind"`
	BigBlind       int64  `json:"big_blind"`
	MaxSeats       int    `json:"max_seats"`
	CurrencyMode   string `json:"currency_mode"`
	Rooms          int    `json:"rooms"`           // public rooms in this bucket
	OpenRooms      int    `json:"open_rooms"`      // those with at least one free seat
	SeatsTaken     int    `json:"seats_taken"`     // players currently seated across the bucket
	SeatsAvailable int    `json:"seats_available"` // free seats across the bucket
}

type LeaveRoomRequest struct {
	IdempotencyKey string `json:"idem_key,omitempty"` // stable per cash-out click; reused across network retries
}
