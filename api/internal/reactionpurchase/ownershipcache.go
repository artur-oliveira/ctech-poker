package reactionpurchase

import (
	"context"

	"gopkg.aoctech.app/api-commons/cache"
)

const ownershipCacheTTLSeconds = 30

type isOwnedChecker interface {
	IsOwned(ctx context.Context, playerID, reactionID string) (bool, error)
}

// OwnershipCache wraps Service.IsOwned behind a Valkey-backed cache. Purchase,
// confirmation and refund transitions explicitly invalidate their key, so the
// TTL is only a fallback for missed invalidations rather than expected user-
// visible staleness (docs/specs/2026-08-12-premium-reactions.md).
type OwnershipCache struct {
	svc     isOwnedChecker
	backend cache.Backend
}

func NewOwnershipCache(svc isOwnedChecker, backend cache.Backend) *OwnershipCache {
	return &OwnershipCache{svc: svc, backend: backend}
}

func ownershipCacheKey(playerID, reactionID string) string {
	return "reaction-owned:" + playerID + ":" + reactionID
}

func (c *OwnershipCache) Invalidate(ctx context.Context, playerID, reactionID string) error {
	return c.backend.Delete(ctx, ownershipCacheKey(playerID, reactionID))
}

func (c *OwnershipCache) IsOwned(ctx context.Context, playerID, reactionID string) (bool, error) {
	key := ownershipCacheKey(playerID, reactionID)
	if cached, ok, _ := c.backend.Get(ctx, key); ok && len(cached) == 1 {
		return cached[0] == '1', nil
	}
	owned, err := c.svc.IsOwned(ctx, playerID, reactionID)
	if err != nil {
		return false, err
	}
	value := byte('0')
	if owned {
		value = '1'
	}
	_ = c.backend.Set(ctx, key, []byte{value}, ownershipCacheTTLSeconds)
	return owned, nil
}
