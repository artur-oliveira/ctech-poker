package achievements

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamo "gopkg.aoctech.app/api-commons/dynamo"
)

const tableProgress = "poker_achievement_progress"

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableProgress)}
}

func (s *Store) Increment(ctx context.Context, playerID, key string, by int) (int, int, error) {
	if by != 1 {
		return 0, 0, fmt.Errorf("achievements: store supports unit increments only")
	}
	// AtomicIncrement adds one and returns the linearized value. Deriving the
	// previous value from it avoids the racy read-before-write in the plan.
	current, err := s.base.AtomicIncrement(ctx, playerID, new(key), "counter")
	if err != nil {
		return 0, 0, fmt.Errorf("achievements: increment: %w", err)
	}
	return int(current) - 1, int(current), nil
}

// PlayerAchievementProgress is one player's progress row for a single
// achievement key (pk: playerID, sk: achievement key, counter: current
// count) — distinct from Achievement, which describes the catalog
// definition (metric + tier thresholds), not a player's progress.
type PlayerAchievementProgress struct {
	Key   string `dynamodbav:"sk" json:"key"`
	Count int    `dynamodbav:"counter" json:"count"`
}

func (s *Store) ListAchievements(ctx context.Context, playerID string, limit int) ([]PlayerAchievementProgress, error) {
	if playerID == "" || limit < 0 {
		return []PlayerAchievementProgress{}, fmt.Errorf("achievements: invalid playerId or limit values")
	}
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]PlayerAchievementProgress, 0, len(result.Items))
	for _, item := range result.Items {
		e, err := dynamo.Decode[PlayerAchievementProgress](item)
		if err != nil {
			return nil, fmt.Errorf("achievements: decode: %w", err)
		}
		out = append(out, *e)
	}
	return out, nil
}
