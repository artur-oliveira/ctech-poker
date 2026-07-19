package v1

import "gopkg.aoctech.app/poker/api/internal/roomstore"

type CreateRoomRequest struct {
	Visibility           string                     `json:"visibility"`
	SmallBlind           int64                      `json:"small_blind"`
	BigBlind             int64                      `json:"big_blind"`
	MaxSeats             int                        `json:"max_seats"`
	BuyInMin             int64                      `json:"buy_in_min"`
	BuyInMax             int64                      `json:"buy_in_max"`
	EquityDisplayEnabled *bool                      `json:"equity_display_enabled,omitempty"`
	BlindEscalation      *roomstore.BlindEscalation `json:"blind_escalation,omitempty"`
}

type JoinRoomRequest struct {
	Amount int64 `json:"amount"`
}
