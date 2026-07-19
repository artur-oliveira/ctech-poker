package achievements

import (
	"context"
	"fmt"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type progressStore interface {
	Increment(context.Context, string, string, int) (previous, current int, err error)
}

type Service struct{ store progressStore }

type TierUnlock struct {
	PlayerID string
	Key      string
	Stars    int
}

func NewService(store *Store) *Service                 { return &Service{store: store} }
func NewServiceWithStore(store progressStore) *Service { return &Service{store: store} }

func (s *Service) RecordHand(ctx context.Context, tableID string, outcome hand.HandOutcome) ([]TierUnlock, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("achievements: progress store is required")
	}
	var unlocks []TierUnlock
	bump := func(playerID, key string) error {
		previous, current, err := s.store.Increment(ctx, playerID, key, 1)
		if err != nil {
			return fmt.Errorf("achievements: table %s player %s key %s: %w", tableID, playerID, key, err)
		}
		if stars, crossed := TierCrossed(key, previous, current); crossed {
			unlocks = append(unlocks, TierUnlock{PlayerID: playerID, Key: key, Stars: stars})
		}
		return nil
	}
	for _, id := range dedupe(outcome.Participants) {
		if err := bump(id, KeyHandsPlayed); err != nil {
			return nil, err
		}
	}
	for _, id := range dedupe(outcome.Winners) {
		if err := bump(id, KeyWins); err != nil {
			return nil, err
		}
		if outcome.WinningCategory != "" {
			if err := bump(id, KeyWinByCategory(outcome.WinningCategory)); err != nil {
				return nil, err
			}
		}
	}
	for _, id := range dedupe(outcome.ComebackWinners) {
		if err := bump(id, KeyComeback); err != nil {
			return nil, err
		}
	}
	return unlocks, nil
}

func TierCrossed(key string, previousTotal, newTotal int) (int, bool) {
	for _, achievement := range Catalog {
		if achievement.Key != key {
			continue
		}
		stars := 0
		for _, tier := range achievement.Tiers {
			if previousTotal < tier.Threshold && newTotal >= tier.Threshold && tier.Stars > stars {
				stars = tier.Stars
			}
		}
		return stars, stars > 0
	}
	return 0, false
}

func dedupe(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
