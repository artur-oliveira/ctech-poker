package dailyreward

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableSpins = "poker_daily_reward"
	// streakSK is the one non-day item under a player's partition. It carries
	// no TTL on purpose — the day claims expire after 48h, the streak is the
	// only durable history of them.
	streakSK = "streak"
)

var brt = time.FixedZone("BRT", -3*60*60)

func cooldownKey(now time.Time) string { return now.In(brt).Format("2006-01-02") }

type dailyRewardItem struct {
	PK          string `dynamodbav:"pk"`
	SK          string `dynamodbav:"sk"`
	Amount      int64  `dynamodbav:"amount"`
	Status      string `dynamodbav:"status"`
	CreatedAt   string `dynamodbav:"created_at"`
	CompletedAt string `dynamodbav:"completed_at,omitempty"`
	TTL         int64  `dynamodbav:"ttl"`
}

type streakItem struct {
	PK string `dynamodbav:"pk"`
	SK string `dynamodbav:"sk"`
	StreakRecord
	UpdatedAt string `dynamodbav:"updated_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableSpins)}
}

// Claim writes the day row (create-only) and the streak row in one
// transaction: the day row's condition guards both, so a duplicate claim
// aborts the streak write too and can never advance a streak twice.
func (s *Store) Claim(ctx context.Context, playerID, day string, proposed int64, streak StreakRecord, now time.Time) (DailyRewardRecord, error) {
	item := dailyRewardItem{PK: playerID, SK: day, Amount: proposed, Status: StatusPending, CreatedAt: now.UTC().Format(time.RFC3339Nano), TTL: now.Add(48 * time.Hour).Unix()}
	encoded, err := dynamo.Encode(item)
	if err != nil {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: encode: %w", err)
	}
	encodedStreak, err := dynamo.Encode(streakItem{PK: playerID, SK: streakSK, StreakRecord: streak, UpdatedAt: now.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: encode streak: %w", err)
	}
	tx := []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded), s.base.BuildPutTxItem(encodedStreak)}
	if err := s.base.TransactWrite(ctx, tx); err == nil {
		return DailyRewardRecord{Amount: proposed, Status: StatusPending}, nil
	} else if !dynamo.IsConditionFailed(err) {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: persist claim: %w", err)
	}
	existing, err := s.base.GetItem(ctx, playerID, day)
	if err != nil {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: load claim: %w", err)
	}
	if existing == nil {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: claim disappeared")
	}
	decoded, err := dynamo.Decode[dailyRewardItem](existing)
	if err != nil {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: decode claim: %w", err)
	}
	return DailyRewardRecord{Amount: decoded.Amount, Status: decoded.Status}, nil
}

func (s *Store) Get(ctx context.Context, playerID, day string) (DailyRewardRecord, error) {
	existing, err := s.base.GetItem(ctx, playerID, day)
	if err != nil {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: get: %w", err)
	}
	if existing == nil {
		return DailyRewardRecord{}, nil
	}
	decoded, err := dynamo.Decode[dailyRewardItem](existing)
	if err != nil {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: decode: %w", err)
	}
	return DailyRewardRecord{Amount: decoded.Amount, Status: decoded.Status}, nil
}

func (s *Store) Complete(ctx context.Context, playerID, day string, now time.Time) error {
	ok, err := s.base.UpdateItem(ctx, playerID, &day, map[string]any{"status": StatusCompleted, "completed_at": now.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return fmt.Errorf("dailyreward: complete: %w", err)
	}
	if !ok {
		return fmt.Errorf("dailyreward: pending claim not found")
	}
	return nil
}

// LoadStreak reads the single streak row. An absent row is the zero record —
// a player who has never claimed, not an error.
func (s *Store) LoadStreak(ctx context.Context, playerID string) (StreakRecord, error) {
	existing, err := s.base.GetItem(ctx, playerID, streakSK)
	if err != nil {
		return StreakRecord{}, fmt.Errorf("dailyreward: get streak: %w", err)
	}
	if existing == nil {
		return StreakRecord{}, nil
	}
	decoded, err := dynamo.Decode[streakItem](existing)
	if err != nil {
		return StreakRecord{}, fmt.Errorf("dailyreward: decode streak: %w", err)
	}
	return decoded.StreakRecord, nil
}
