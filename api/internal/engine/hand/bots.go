package hand

import (
	"errors"
	"fmt"
)

var (
	ErrNoReplaceableBotSeat = errors.New("hand: table has no replaceable bot seat")
	ErrBotSeatReserved      = errors.New("hand: bot seat already reserved")
	ErrBotFillUnavailable   = errors.New("hand: bot fill is unavailable")
)

type BotReservation struct {
	ID              string `json:"id" dynamodbav:"id"`
	PlayerID        string `json:"player_id" dynamodbav:"player_id"`
	Amount          int64  `json:"amount" dynamodbav:"amount"`
	AutoRebuy       bool   `json:"auto_rebuy" dynamodbav:"auto_rebuy"`
	IdempotencyKey  string `json:"idempotency_key" dynamodbav:"idempotency_key"`
	ExpiresAtUnixMs int64  `json:"expires_at_unix_ms" dynamodbav:"expires_at_unix_ms"`
}

// BotPolicy is authoritative persisted state; timers merely wake an actor.
type BotPolicy struct {
	Enabled              bool   `json:"enabled" dynamodbav:"enabled"`
	OwnerID              string `json:"owner_id" dynamodbav:"owner_id"`
	BuyIn                int64  `json:"buy_in" dynamodbav:"buy_in"`
	MaxSeats             int    `json:"max_seats" dynamodbav:"max_seats"`
	TargetOccupancy      int    `json:"target_occupancy" dynamodbav:"target_occupancy"`
	ActivateAtUnixMs     int64  `json:"activate_at_unix_ms" dynamodbav:"activate_at_unix_ms"`
	FundingCheckedHandID string `json:"funding_checked_hand_id,omitempty" dynamodbav:"funding_checked_hand_id,omitempty"`
	LastReactionHandID   string `json:"last_reaction_hand_id,omitempty" dynamodbav:"last_reaction_hand_id,omitempty"`
	LastReactionAtUnixMs int64  `json:"last_reaction_at_unix_ms,omitempty" dynamodbav:"last_reaction_at_unix_ms,omitempty"`
}

func BotTargetOccupancy(maxSeats int) (int, error) {
	switch maxSeats {
	case 2:
		return 2, nil
	case 6:
		return 4, nil
	case 9:
		return 6, nil
	}
	return 0, fmt.Errorf("hand: unsupported bot table capacity %d", maxSeats)
}

func (t *Table) ConfigureBotsForActor(ownerID string, buyIn int64, maxSeats int, activateAt int64) error {
	if t.currencyMode != "sandbox" {
		return fmt.Errorf("hand: bots require sandbox mode")
	}
	if ownerID == "" || buyIn <= 0 || activateAt <= 0 {
		return fmt.Errorf("hand: invalid bot policy")
	}
	target, err := BotTargetOccupancy(maxSeats)
	if err != nil {
		return err
	}
	t.botPolicy = BotPolicy{Enabled: true, OwnerID: ownerID, BuyIn: buyIn, MaxSeats: maxSeats, TargetOccupancy: target, ActivateAtUnixMs: activateAt}
	return nil
}

func (t *Table) BotPolicyForActor() BotPolicy { return t.botPolicy }

func (t *Table) MarkBotReactionForActor(handID string, atUnixMs int64) {
	t.botPolicy.LastReactionHandID = handID
	t.botPolicy.LastReactionAtUnixMs = atUnixMs
}

func (t *Table) ExpediteBotsForActor(ownerID string, nowUnixMs int64) error {
	if !t.botPolicy.Enabled || t.botPolicy.OwnerID != ownerID || t.BotSeatsNeededForActor() == 0 {
		return ErrBotFillUnavailable
	}
	if t.botPolicy.ActivateAtUnixMs > nowUnixMs {
		t.botPolicy.ActivateAtUnixMs = nowUnixMs
	}
	return nil
}

func (t *Table) MarkBotFundingCheckedForActor(handID string) {
	t.botPolicy.FundingCheckedHandID = handID
}

func (t *Table) BotForActor(id string) (*Player, bool) {
	p := t.playerByID(id)
	return p, p != nil && p.IsBot
}

func (t *Table) HumanSeatCountForActor() int {
	count := 0
	for _, p := range t.players {
		if !p.IsBot {
			count++
		}
	}
	return count
}

func (t *Table) BotSeatCountForActor() int {
	count := 0
	for _, p := range t.players {
		if p.IsBot {
			count++
		}
	}
	return count
}

// RetireBotsForHumanArrival keeps bots in the live hand long enough to fold
// through the normal turn machinery, while making them ineligible for the
// next deal. Between hands the actor removes them without wallet settlement.
func (t *Table) RetireBotsForHumanArrival() error {
	for _, p := range t.players {
		if p.IsBot && !p.PendingExit {
			if err := t.RequestExit(p.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
func (t *Table) BotSeatsNeededForActor() int {
	if !t.botPolicy.Enabled || t.botReservation != nil {
		return 0
	}
	humans, bots := 0, 0
	ownerPresent := false
	for _, p := range t.players {
		if p.IsBot {
			bots++
		} else {
			humans++
			ownerPresent = ownerPresent || p.ID == t.botPolicy.OwnerID
		}
	}
	if humans != 1 || !ownerPresent {
		return 0
	}
	needed := t.botPolicy.TargetOccupancy - humans - bots
	if needed < 0 {
		return 0
	}
	return needed
}

func (t *Table) ReserveBotSeatForActor(reservation BotReservation, nowUnixMs int64) error {
	if reservation.ID == "" || reservation.PlayerID == "" || reservation.Amount <= 0 || reservation.ExpiresAtUnixMs <= nowUnixMs {
		return fmt.Errorf("hand: invalid bot reservation")
	}
	if t.botReservation != nil && t.botReservation.ExpiresAtUnixMs > nowUnixMs {
		if t.botReservation.PlayerID == reservation.PlayerID && t.botReservation.IdempotencyKey == reservation.IdempotencyKey {
			return nil
		}
		return ErrBotSeatReserved
	}
	if t.HumanSeatCountForActor() == 0 {
		return fmt.Errorf("hand: no human-hosted bot match to reserve")
	}
	hasBot := false
	for _, p := range t.players {
		hasBot = hasBot || p.IsBot
	}
	if !hasBot {
		return ErrNoReplaceableBotSeat
	}
	t.botReservation = &reservation
	return t.RetireBotsForHumanArrival()
}

func (t *Table) BotReservationForActor() *BotReservation {
	if t.botReservation == nil {
		return nil
	}
	copy := *t.botReservation
	return &copy
}

func (t *Table) CancelBotReservationForActor(id, playerID string) bool {
	if t.botReservation == nil || t.botReservation.ID != id || t.botReservation.PlayerID != playerID {
		return false
	}
	t.botReservation = nil
	return true
}

func (t *Table) ConsumeBotReservationForActor(id, playerID string) error {
	if t.botReservation == nil || t.botReservation.ID != id || t.botReservation.PlayerID != playerID {
		return fmt.Errorf("hand: bot reservation not found")
	}
	t.botReservation = nil
	return nil
}

func (t *Table) BotReservationReadyForActor() bool {
	if t.botReservation == nil {
		return false
	}
	for _, p := range t.players {
		if p.IsBot {
			return false
		}
	}
	return t.stage == WaitingForPlayers || t.stage == Complete
}
func (t *Table) AddBotForActor(id, name, profile string) error {
	if !t.botPolicy.Enabled || id == "" || name == "" {
		return fmt.Errorf("hand: bot policy is not active")
	}
	if t.BotSeatsNeededForActor() <= 0 {
		return fmt.Errorf("hand: bot target already reached")
	}
	p := &Player{ID: id, Name: name, IsBot: true, BotProfile: profile, Stack: t.botPolicy.BuyIn, BuyInAmount: t.botPolicy.BuyIn, Ready: true, State: Active}
	return t.AddWaitingPlayer(p)
}
func (t *Table) DisableBotsForActor() { t.botPolicy.Enabled = false }
