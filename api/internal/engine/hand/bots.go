package hand

import "fmt"

// BotPolicy is authoritative persisted state; timers merely wake an actor.
type BotPolicy struct {
	Enabled          bool   `json:"enabled" dynamodbav:"enabled"`
	OwnerID          string `json:"owner_id" dynamodbav:"owner_id"`
	BuyIn            int64  `json:"buy_in" dynamodbav:"buy_in"`
	MaxSeats         int    `json:"max_seats" dynamodbav:"max_seats"`
	TargetOccupancy  int    `json:"target_occupancy" dynamodbav:"target_occupancy"`
	ActivateAtUnixMs int64  `json:"activate_at_unix_ms" dynamodbav:"activate_at_unix_ms"`
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
	if !t.botPolicy.Enabled {
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
