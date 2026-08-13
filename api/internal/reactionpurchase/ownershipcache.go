package reactionpurchase

import (
	"context"

	"gopkg.aoctech.app/api-commons/cache"
)

const ownershipCacheTTLSeconds = 30

type isOwnedChecker interface {
	IsOwned(ctx context.Context, playerID, reactionID string) (bool, error)
}

// OwnershipCache wraps Service.IsOwned behind a Valkey-backed cache — latency-
// only, never a correctness mechanism (same category as tablelease). A stale
// "not owned" for up to 30s after a purchase just means the player's first
// attempt right after buying can delay-fail once; the purchase flow already
// returns success from the entitlement write, so the frontend treats
// "just bought" as owned optimistically without waiting on this cache
// (docs/specs/2026-08-12-premium-reactions.md).
type OwnershipCache struct {
	svc     isOwnedChecker
	backend cache.Backend
}

func NewOwnershipCache(svc isOwnedChecker, backend cache.Backend) *OwnershipCache {
	return &OwnershipCache{svc: svc, backend: backend}
}

func (c *OwnershipCache) IsOwned(ctx context.Context, playerID, reactionID string) (bool, error) {
	key := "reaction-owned:" + playerID + ":" + reactionID
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
