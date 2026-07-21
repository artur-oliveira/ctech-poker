package leaderboard

import (
	"context"
	"fmt"
	"sort"

	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type Entry struct {
	PlayerID          string  `dynamodbav:"pk" json:"player_id"`
	HandsPlayed       int     `dynamodbav:"hands_played" json:"hands_played"`
	HandsWon          int     `dynamodbav:"hands_won" json:"hands_won"`
	AchievementPoints int     `dynamodbav:"achievement_points" json:"achievement_points"`
	WinRate           float64 `dynamodbav:"win_rate_score" json:"win_rate"`
}

type statsStore interface {
	IncrementStats(context.Context, string, int, int) error
	IncrementAchievementPoints(context.Context, string, int) error
	Top(context.Context, string, int) ([]Entry, error)
}
type Service struct{ store statsStore }

func NewServiceWithStore(store statsStore) *Service { return &Service{store: store} }

func (s *Service) RecordHand(ctx context.Context, outcome hand.HandOutcome) error {
	winners := make(map[string]bool, len(outcome.Winners))
	for _, id := range outcome.Winners {
		winners[id] = true
	}
	seen := make(map[string]bool, len(outcome.Participants))
	for _, id := range outcome.Participants {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		won := 0
		if winners[id] {
			won = 1
		}
		if err := s.store.IncrementStats(ctx, id, 1, won); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RecordUnlocks(ctx context.Context, unlocks []achievements.TierUnlock) error {
	for _, unlock := range unlocks {
		if unlock.Stars > 0 {
			if err := s.store.IncrementAchievementPoints(ctx, unlock.PlayerID, unlock.Stars); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) Top(ctx context.Context, metric string, limit int) ([]Entry, error) {
	if metric == "" {
		metric = "hands_won"
	}
	// achievement_points is deliberately NOT rankable (B31): there is no
	// gsi_achievement_points GSI, and ranking it via another metric's GSI
	// silently returned wrong ordering. Add the GSI before re-enabling it.
	if metric != "hands_won" && metric != "hands_played" && metric != "win_rate" {
		return nil, fmt.Errorf("leaderboard: unsupported metric %q", metric)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	entries, err := s.store.Top(ctx, metric, limit)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].HandsPlayed > 0 {
			entries[i].WinRate = float64(entries[i].HandsWon) / float64(entries[i].HandsPlayed)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		var a, b float64
		switch metric {
		case "hands_played":
			a, b = float64(entries[i].HandsPlayed), float64(entries[j].HandsPlayed)
		case "win_rate":
			a, b = entries[i].WinRate, entries[j].WinRate
		default:
			a, b = float64(entries[i].HandsWon), float64(entries[j].HandsWon)
		}
		if a == b {
			return entries[i].PlayerID < entries[j].PlayerID
		}
		return a > b
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}
