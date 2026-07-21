package roulette

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tableSpins = "poker_roulette_spins"

var brt = time.FixedZone("BRT", -3*60*60)

func cooldownKey(now time.Time) string { return now.In(brt).Format("2006-01-02") }

type spinItem struct {
	PK          string `dynamodbav:"pk"`
	SK          string `dynamodbav:"sk"`
	Amount      int64  `dynamodbav:"amount"`
	Status      string `dynamodbav:"status"`
	CreatedAt   string `dynamodbav:"created_at"`
	CompletedAt string `dynamodbav:"completed_at,omitempty"`
	TTL         int64  `dynamodbav:"ttl"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableSpins)}
}

func (s *Store) Claim(ctx context.Context, playerID, day string, proposed int64, now time.Time) (SpinRecord, error) {
	item := spinItem{PK: playerID, SK: day, Amount: proposed, Status: StatusPending, CreatedAt: now.UTC().Format(time.RFC3339Nano), TTL: now.Add(48 * time.Hour).Unix()}
	encoded, err := dynamo.Encode(item)
	if err != nil {
		return SpinRecord{}, fmt.Errorf("roulette: encode: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err == nil {
		return SpinRecord{Amount: proposed, Status: StatusPending}, nil
	} else if !dynamo.IsConditionFailed(err) {
		return SpinRecord{}, fmt.Errorf("roulette: persist claim: %w", err)
	}
	existing, err := s.base.GetItem(ctx, playerID, day)
	if err != nil {
		return SpinRecord{}, fmt.Errorf("roulette: load claim: %w", err)
	}
	if existing == nil {
		return SpinRecord{}, fmt.Errorf("roulette: claim disappeared")
	}
	decoded, err := dynamo.Decode[spinItem](existing)
	if err != nil {
		return SpinRecord{}, fmt.Errorf("roulette: decode claim: %w", err)
	}
	return SpinRecord{Amount: decoded.Amount, Status: decoded.Status}, nil
}

func (s *Store) Get(ctx context.Context, playerID, day string) (SpinRecord, error) {
	existing, err := s.base.GetItem(ctx, playerID, day)
	if err != nil {
		return SpinRecord{}, fmt.Errorf("roulette: get: %w", err)
	}
	if existing == nil {
		return SpinRecord{}, nil
	}
	decoded, err := dynamo.Decode[spinItem](existing)
	if err != nil {
		return SpinRecord{}, fmt.Errorf("roulette: decode: %w", err)
	}
	return SpinRecord{Amount: decoded.Amount, Status: decoded.Status}, nil
}

func (s *Store) Complete(ctx context.Context, playerID, day string, now time.Time) error {
	ok, err := s.base.UpdateItem(ctx, playerID, &day, map[string]any{"status": StatusCompleted, "completed_at": now.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return fmt.Errorf("roulette: complete: %w", err)
	}
	if !ok {
		return fmt.Errorf("roulette: pending claim not found")
	}
	return nil
}
