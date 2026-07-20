package achievements

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	commonsdynamo "gopkg.aoctech.app/api-commons/dynamo"
)

const tableProgress = "poker_achievement_progress"

type Store struct{ base commonsdynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: commonsdynamo.NewBase(db, env, tableProgress)}
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
