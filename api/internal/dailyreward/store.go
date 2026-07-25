package dailyreward

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/cachekit"
)

const tableSpins = "poker_daily_reward"

// claimCacheTTL is a safety net for a missed invalidation — Claim and
// Complete both invalidate their key on write, so this is not the primary
// consistency mechanism. A day's record only ever transitions pending ->
// completed once, so staleness risk is low even at this TTL.
const claimCacheTTL = 5 * time.Minute

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

type Store struct {
	base  dynamo.Base
	cache cache.Backend
}

func NewStore(db *dynamodb.Client, env string, c cache.Backend) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableSpins), cache: c}
}

func claimCacheKey(playerID, day string) string { return "dailyreward:" + playerID + ":" + day }

func (s *Store) Claim(ctx context.Context, playerID, day string, proposed int64, now time.Time) (DailyRewardRecord, error) {
	item := dailyRewardItem{PK: playerID, SK: day, Amount: proposed, Status: StatusPending, CreatedAt: now.UTC().Format(time.RFC3339Nano), TTL: now.Add(48 * time.Hour).Unix()}
	encoded, err := dynamo.Encode(item)
	if err != nil {
		return DailyRewardRecord{}, fmt.Errorf("dailyreward: encode: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err == nil {
		cachekit.Invalidate(ctx, s.cache, claimCacheKey(playerID, day))
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
	record, err := cachekit.GetOrLoad(ctx, s.cache, claimCacheKey(playerID, day), claimCacheTTL, func() (*DailyRewardRecord, error) {
		existing, err := s.base.GetItem(ctx, playerID, day)
		if err != nil {
			return nil, fmt.Errorf("dailyreward: get: %w", err)
		}
		if existing == nil {
			return nil, nil
		}
		decoded, err := dynamo.Decode[dailyRewardItem](existing)
		if err != nil {
			return nil, fmt.Errorf("dailyreward: decode: %w", err)
		}
		return &DailyRewardRecord{Amount: decoded.Amount, Status: decoded.Status}, nil
	})
	if err != nil || record == nil {
		return DailyRewardRecord{}, err
	}
	return *record, nil
}

func (s *Store) Complete(ctx context.Context, playerID, day string, now time.Time) error {
	ok, err := s.base.UpdateItem(ctx, playerID, &day, map[string]any{"status": StatusCompleted, "completed_at": now.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return fmt.Errorf("dailyreward: complete: %w", err)
	}
	if !ok {
		return fmt.Errorf("dailyreward: pending claim not found")
	}
	cachekit.Invalidate(ctx, s.cache, claimCacheKey(playerID, day))
	return nil
}

func (s *Store) IsFirstReward(ctx context.Context, playerID string) (bool, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: 1})
	if err != nil {
		return false, err
	}
	return len(result.Items) == 0, nil
}
