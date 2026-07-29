package achievements

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type progressStore interface {
	Increment(context.Context, string, string, string, int) (previous, current int, err error)
	ListAchievements(ctx context.Context, playerID, mode string, limit int, startKey map[string]types.AttributeValue) ([]PlayerAchievementProgress, map[string]types.AttributeValue, error)
}

type Service struct{ store progressStore }

type TierUnlock struct {
	PlayerID string
	Key      string
	Stars    int
}

func NewService(store *Store) *Service                 { return &Service{store: store} }
func NewServiceWithStore(store progressStore) *Service { return &Service{store: store} }

func (s *Service) RecordHand(ctx context.Context, tableID, mode string, outcome hand.HandOutcome) ([]TierUnlock, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("achievements: progress store is required")
	}
	var unlocks []TierUnlock
	bump := func(playerID, key string) error {
		previous, current, err := s.store.Increment(ctx, playerID, mode, key, 1)
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
		category := outcome.WinningCategory
		if result, ok := outcome.ShowdownResults[id]; ok && result.Category != "" {
			category = result.Category
		}
		if category != "" {
			if err := bump(id, KeyWinByCategory(category)); err != nil {
				return nil, err
			}
		}
	}
	for _, id := range dedupe(outcome.AllInPlayers) {
		if err := bump(id, KeyAllIn); err != nil {
			return nil, err
		}
	}
	for _, id := range dedupe(outcome.ComebackWinners) {
		if err := bump(id, KeyComeback); err != nil {
			return nil, err
		}
		if wonAgainstBiggerStack(outcome, id) {
			if err := bump(id, KeyGiantSlayer); err != nil {
				return nil, err
			}
		}
	}
	for id, result := range outcome.ShowdownResults {
		if err := bump(id, KeyShowdownWarrior); err != nil {
			return nil, err
		}
		if result.Won {
			if result.Tied || result.SplitPot {
				if err := bump(id, KeyTied); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := bump(id, KeyLooser); err != nil {
			return nil, err
		}
		if lostToSameCategory(outcome, id, result.Category) {
			if err := bump(id, KeyAlmostWinner); err != nil {
				return nil, err
			}
		}
		if categoryAtLeast(result.Category, "three_of_a_kind") {
			if err := bump(id, KeyBadBeat); err != nil {
				return nil, err
			}
		}
		if categoryAtLeast(result.Category, "full_house") {
			if err := bump(id, KeyCooler); err != nil {
				return nil, err
			}
		}
		if hi, ok := outcome.PlayerHands[id]; ok && isPocketAces(hi.HoleCards) {
			if err := bump(id, KeyCrackedAces); err != nil {
				return nil, err
			}
		}
		if hi, ok := outcome.PlayerHands[id]; ok && isPocketKings(hi.HoleCards) {
			if err := bump(id, KeyFallenKing); err != nil {
				return nil, err
			}
		}
	}
	return unlocks, nil
}

func lostToSameCategory(outcome hand.HandOutcome, playerID, category string) bool {
	if len(outcome.PotResults) == 0 {
		return category != "" && category == outcome.WinningCategory
	}
	for _, pot := range outcome.PotResults {
		eligible := false
		for _, id := range pot.EligiblePlayerIDs {
			if id == playerID {
				eligible = true
				break
			}
		}
		if !eligible {
			continue
		}
		for _, winner := range pot.Winners {
			if result, ok := outcome.ShowdownResults[winner]; ok && result.Category == category {
				return true
			}
		}
	}
	return false
}

// categoryAtLeast reports whether category is at least as strong as floor,
// per categoryOrder (catalog.go).
func categoryAtLeast(category, floor string) bool {
	at, af := -1, -1
	for i, c := range categoryOrder {
		if c == category {
			at = i
		}
		if c == floor {
			af = i
		}
	}
	return at >= 0 && af >= 0 && at >= af
}

func isPocketAces(holeCards [2]string) bool {
	return len(holeCards[0]) == 2 && len(holeCards[1]) == 2 &&
		holeCards[0][0] == 'A' && holeCards[1][0] == 'A'
}

func isPocketKings(holeCards [2]string) bool {
	return len(holeCards[0]) == 2 && len(holeCards[1]) == 2 &&
		holeCards[0][0] == 'K' && holeCards[1][0] == 'K'
}

// wonAgainstBiggerStack reports whether playerID (already known to be a
// comeback winner, i.e. won after going all-in) faced an opponent who
// contributed more chips to this hand than they did — a proxy for "beat a
// bigger stack" without needing pre-hand stack snapshots.
func wonAgainstBiggerStack(outcome hand.HandOutcome, playerID string) bool {
	self := outcome.Contributions[playerID]
	for _, opp := range outcome.Participants {
		if opp != playerID && outcome.Contributions[opp] > self {
			return true
		}
	}
	return false
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
