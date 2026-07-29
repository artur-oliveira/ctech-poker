package achievements

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	dynamo "gopkg.aoctech.app/api-commons/dynamo"
)

const tableProgress = "poker_achievement_progress"

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableProgress)}
}

func (s *Store) Increment(ctx context.Context, playerID, mode, key string, by int) (int, int, error) {
	if by != 1 {
		return 0, 0, fmt.Errorf("achievements: store supports unit increments only")
	}
	// AtomicIncrement adds one and returns the linearized value. Deriving the
	// previous value from it avoids the racy read-before-write in the plan.
	sk := mode + "#" + key
	current, err := s.base.AtomicIncrement(ctx, playerID, &sk, "counter")
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

func (s *Store) ListAchievements(ctx context.Context, playerID, mode string, limit int, startKey map[string]dynamotypes.AttributeValue) ([]PlayerAchievementProgress, map[string]dynamotypes.AttributeValue, error) {
	if playerID == "" || limit < 0 {
		return []PlayerAchievementProgress{}, nil, fmt.Errorf("achievements: invalid playerId or limit values")
	}
	prefix := mode + "#"
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, SKPrefix: prefix, Limit: limit, ExclusiveStartKey: startKey})
	if err != nil {
		return nil, nil, err
	}
	out := make([]PlayerAchievementProgress, 0, len(result.Items))
	for _, item := range result.Items {
		e, err := dynamo.Decode[PlayerAchievementProgress](item)
		if err != nil {
			return nil, nil, fmt.Errorf("achievements: decode: %w", err)
		}
		e.Key = strings.TrimPrefix(e.Key, prefix)
		out = append(out, *e)
	}
	return out, result.LastEvaluatedKey, nil
}
