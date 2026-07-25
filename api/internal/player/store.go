package player

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

const tablePlayerProfiles = "poker_player_profiles"

// profileCacheTTL bounds how stale a cached profile can get if an
// invalidation call is ever missed — every mutator below invalidates its own
// key, so this is just a safety net, not the primary consistency mechanism.
const profileCacheTTL = 5 * time.Minute

type Store struct {
	base  dynamo.Base
	cache cache.Backend
}

func NewStore(db *dynamodb.Client, env string, c cache.Backend) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePlayerProfiles), cache: c}
}

func profileCacheKey(userID string) string { return "player:profile:" + userID }

func (s *Store) GetOrCreate(ctx context.Context, userID string) (*PlayerProfile, error) {
	if userID == "" {
		return nil, fmt.Errorf("player: empty user id")
	}
	if profile, err := s.get(ctx, userID); err != nil || profile != nil {
		return profile, err
	}
	now := dynamo.NowStr()
	profile := PlayerProfile{UserID: userID, Name: RandomName(), CreatedAt: now, UpdatedAt: now}
	item, err := dynamo.Encode(profile)
	if err != nil {
		return nil, fmt.Errorf("player: encode: %w", err)
	}
	err = s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(item)})
	if err != nil && !dynamo.IsConditionFailed(err) {
		return nil, fmt.Errorf("player: create: %w", err)
	}
	return s.get(ctx, userID)
}

func (s *Store) get(ctx context.Context, userID string) (*PlayerProfile, error) {
	return cachekit.GetOrLoad(ctx, s.cache, profileCacheKey(userID), profileCacheTTL, func() (*PlayerProfile, error) {
		item, err := s.base.GetItem(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("player: get: %w", err)
		}
		if item == nil {
			return nil, nil
		}
		profile, err := dynamo.Decode[PlayerProfile](item)
		if err != nil {
			return nil, fmt.Errorf("player: decode: %w", err)
		}
		return profile, nil
	})
}

func (s *Store) SetName(ctx context.Context, userID, name string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"name":       name,
		"updated_at": dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set name: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting name")
	}
	cachekit.Invalidate(ctx, s.cache, profileCacheKey(userID))
	return nil
}

func (s *Store) SetWalletMode(ctx context.Context, userID, mode string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"wallet_mode": mode,
		"updated_at":  dynamo.NowStr(),
	})
	if err != nil {
		return fmt.Errorf("player: set wallet mode: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while setting wallet mode")
	}
	cachekit.Invalidate(ctx, s.cache, profileCacheKey(userID))
	return nil
}

func (s *Store) AcceptTerms(ctx context.Context, userID string) error {
	if _, err := s.GetOrCreate(ctx, userID); err != nil {
		return err
	}
	now := dynamo.NowStr()
	ok, err := s.base.UpdateItem(ctx, userID, nil, map[string]any{
		"poker_terms_version":     CurrentPokerTermsVersion,
		"poker_terms_accepted_at": now,
		"updated_at":              now,
	})
	if err != nil {
		return fmt.Errorf("player: accept terms: %w", err)
	}
	if !ok {
		return fmt.Errorf("player: profile disappeared while accepting terms")
	}
	cachekit.Invalidate(ctx, s.cache, profileCacheKey(userID))
	return nil
}
