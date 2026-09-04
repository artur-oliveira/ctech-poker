package leaderboard

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type Entry struct {
	PlayerID          string  `dynamodbav:"pk" json:"player_id"`
	PlayerName        string  `dynamodbav:"player_name,omitempty" json:"player_name,omitempty"`
	HandsPlayed       int     `dynamodbav:"hands_played" json:"hands_played"`
	HandsWon          int     `dynamodbav:"hands_won" json:"hands_won"`
	AchievementPoints int     `dynamodbav:"achievement_points" json:"achievement_points"`
	WinRate           float64 `dynamodbav:"win_rate_score" json:"win_rate"`
}

// RankInfo is a player's exact position on a mode/metric leaderboard,
// computed independently of any fetched page (see Service.MyRank).
type RankInfo struct {
	Entry Entry `json:"entry"`
	Rank  int64 `json:"rank"`
	Total int64 `json:"total"`
}

type statsStore interface {
	IncrementStats(ctx context.Context, playerID, name, mode string, playedDelta, wonDelta int) error
	IncrementAchievementPoints(context.Context, string, string, int) error
	Top(ctx context.Context, mode, metric string, limit int, startKey map[string]types.AttributeValue) ([]Entry, map[string]types.AttributeValue, error)
	PlayerEntry(ctx context.Context, playerID, mode string) (*Entry, error)
	RankOf(ctx context.Context, mode, metric string, entry Entry) (rank int64, total int64, err error)
}
type Service struct{ store statsStore }

func NewServiceWithStore(store statsStore) *Service { return &Service{store: store} }

// RecordHand updates every participant's counters. names supplies the
// already-known display name for each player_id (the table actor resolves it
// once at join, from the canonical poker_player_profiles record) — no extra
// lookup here, just carrying it along to the write that's happening anyway.
func (s *Service) RecordHand(ctx context.Context, mode string, outcome hand.HandOutcome, names map[string]string) error {
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
		if err := s.store.IncrementStats(ctx, id, names[id], mode, 1, won); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RecordUnlocks(ctx context.Context, mode string, unlocks []achievements.TierUnlock) error {
	for _, unlock := range unlocks {
		if unlock.Stars > 0 {
			if err := s.store.IncrementAchievementPoints(ctx, unlock.PlayerID, mode, unlock.Stars); err != nil {
				return err
			}
		}
	}
	return nil
}

// normalizeMetric defaults an empty metric to hands_won and rejects anything
// not backed by a GSI. achievement_points is deliberately NOT rankable (B31):
// there is no gsi_achievement_points GSI, and ranking it via another metric's
// GSI silently returned wrong ordering. Add the GSI before re-enabling it.
func normalizeMetric(metric string) (string, error) {
	if metric == "" {
		metric = "hands_won"
	}
	if metric != "hands_won" && metric != "hands_played" && metric != "win_rate" {
		return "", fmt.Errorf("leaderboard: unsupported metric %q", metric)
	}
	return metric, nil
}

func (s *Service) Top(ctx context.Context, mode, metric string, limit int, startKey map[string]types.AttributeValue) ([]Entry, map[string]types.AttributeValue, error) {
	metric, err := normalizeMetric(metric)
	if err != nil {
		return nil, nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	entries, lastKey, err := s.store.Top(ctx, mode, metric, limit, startKey)
	if err != nil {
		return nil, nil, err
	}
	for i := range entries {
		if entries[i].HandsPlayed > 0 {
			entries[i].WinRate = float64(entries[i].HandsWon) / float64(entries[i].HandsPlayed)
		}
	}
	// Defensive floor for win_rate (issue #63): the sparse gsi_win_rate_pk key
	// should already keep sub-floor rows out of the query, but GSI propagation
	// lag or a not-yet-backfilled legacy row could still slip one in. Drop them
	// before sorting/truncation so they neither appear nor occupy a rank slot.
	if metric == "win_rate" {
		kept := entries[:0]
		for _, e := range entries {
			if e.HandsPlayed >= MinHandsForWinRateRank {
				kept = append(kept, e)
			}
		}
		entries = kept
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
	return entries, lastKey, nil
}

// MyRank computes playerID's exact global rank and the total number of
// ranked players for mode/metric, via a COUNT query against the metric's
// GSI (Store.RankOf) rather than fetching and sorting the whole board — so a
// player far outside Top's page still gets an exact answer. Returns
// (nil, nil) when playerID has no stats row for mode yet (never played a
// hand there this mode) — the "unranked yet" case the caller should render
// as such rather than as an error.
func (s *Service) MyRank(ctx context.Context, mode, metric, playerID string) (*RankInfo, error) {
	metric, err := normalizeMetric(metric)
	if err != nil {
		return nil, err
	}
	entry, err := s.store.PlayerEntry(ctx, playerID, mode)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}
	// A sub-floor row is in no win_rate GSI (issue #63) and no longer carries a
	// freshly materialized win_rate_score (issue #217), so there is no rank to
	// report for it — the caller's "unranked yet" state, same as no row at all.
	if metric == "win_rate" && entry.HandsPlayed < MinHandsForWinRateRank {
		return nil, nil
	}
	// RankOf must compare against the score actually materialized in the
	// GSI right now (entry.WinRate as decoded), not a value recomputed here
	// — the two can differ for a moment during the two-write
	// IncrementStats/syncWinRateRow sequence (see store.go).
	rank, total, err := s.store.RankOf(ctx, mode, metric, *entry)
	if err != nil {
		return nil, err
	}
	// Recompute WinRate fresh for display, same as Top does for every row.
	if entry.HandsPlayed > 0 {
		entry.WinRate = float64(entry.HandsWon) / float64(entry.HandsPlayed)
	}
	return &RankInfo{Entry: *entry, Rank: rank, Total: total}, nil
}
