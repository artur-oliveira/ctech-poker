// Package cachekit is a thin read-through cache-aside helper over
// api-commons/cache.Backend (Valkey in prod, in-memory in dev). Eviction
// under memory pressure is the Valkey server's job (maxmemory-policy
// allkeys-lru, cdk ValkeyStack) — this package only decides what to cache,
// for how long, and when to invalidate. DynamoDB stays the source of truth:
// every cached read has a store method that hits Dynamo directly on a miss,
// and every write path invalidates its key so the next read observes it.
package cachekit

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

// GetOrLoad returns key's cached value if present, otherwise calls load,
// caches a non-nil result for ttl (±20% jitter, so a burst of keys seeded at
// the same time doesn't expire and re-hit Dynamo in the same instant), and
// returns it. A nil result (not found) is never cached — otherwise a
// legitimate write landing right after a "not found" read would stay
// invisible until TTL expiry instead of the write's own invalidation call.
// Cache errors (backend down, corrupt entry) fall back to load rather than
// failing the request — Dynamo is always the source of truth.
func GetOrLoad[T any](ctx context.Context, backend cache.Backend, key string, ttl time.Duration, load func() (*T, error)) (*T, error) {
	if raw, ok, err := backend.Get(ctx, key); err == nil && ok {
		var v T
		if err := json.Unmarshal(raw, &v); err == nil {
			return &v, nil
		}
	}
	v, err := load()
	if err != nil || v == nil {
		return v, err
	}
	if raw, err := json.Marshal(v); err == nil {
		_ = backend.Set(ctx, key, raw, jitteredSeconds(ttl))
	}
	return v, nil
}

// Invalidate drops key so the next read repopulates from Dynamo. Best-effort:
// a failed delete just means the entry lives until its TTL, never a
// correctness issue since Dynamo is authoritative.
func Invalidate(ctx context.Context, backend cache.Backend, key string) {
	_ = backend.Delete(ctx, key)
}

// jitteredSeconds spreads a batch of same-TTL keys' expiry over a ±20% window
// so they don't all miss and hammer Dynamo at the same instant.
func jitteredSeconds(ttl time.Duration) int {
	secs := int(ttl.Seconds())
	if secs <= 0 {
		return secs
	}
	delta := secs / 5
	if delta == 0 {
		return secs
	}
	return secs - delta + rand.IntN(2*delta+1)
}
