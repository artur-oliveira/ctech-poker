package chatprefs

import (
	"context"
	"encoding/json"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/observability"
)

// extraWordsCacheTTLSeconds mirrors reactionpurchase.OwnershipCache's own
// TTL: this is a fallback for a missed invalidation, not expected staleness,
// but chat has no purchase-style event to invalidate on, so a personal
// preference change can take up to this long to reach a table already
// mid-hand.
const extraWordsCacheTTLSeconds = 30

type extraWordsGetter interface {
	Get(ctx context.Context, playerID string) (*Preferences, error)
}

// ExtraWordsCache wraps Store.Get behind a Valkey-backed cache (#327: "cache
// para não virar leitura extra de DynamoDB por mensagem de chat em mesa
// cheia"). ExtraWords is what internal/table.Actor calls, at most once per
// ChatPrefsRefreshInterval per seated player — never per message.
type ExtraWordsCache struct {
	store   extraWordsGetter
	backend cache.Backend
}

func NewExtraWordsCache(store extraWordsGetter, backend cache.Backend) *ExtraWordsCache {
	return &ExtraWordsCache{store: store, backend: backend}
}

func extraWordsCacheKey(playerID string) string { return "chatprefs:extra:" + playerID }

// Invalidate drops the cached entry so a preference change published through
// the HTTP handler is visible the next time a table refreshes, rather than
// waiting out the TTL.
func (c *ExtraWordsCache) Invalidate(ctx context.Context, playerID string) error {
	return c.backend.Delete(ctx, extraWordsCacheKey(playerID))
}

// ExtraWords answers playerID's personal extra-word list, or nil if they
// have none. Matches the func(ctx, playerID) ([]string, error) shape
// internal/table.Actor's chatPrefsLookup expects.
func (c *ExtraWordsCache) ExtraWords(ctx context.Context, playerID string) ([]string, error) {
	key := extraWordsCacheKey(playerID)
	if cached, ok, err := c.backend.Get(ctx, key); err != nil {
		observability.Warn(ctx, "chatprefs cache read failed", err, "player_id", playerID)
	} else if ok {
		var words []string
		if err := json.Unmarshal(cached, &words); err != nil {
			observability.Warn(ctx, "chatprefs cache decode failed", err, "player_id", playerID)
		} else {
			return words, nil
		}
	}
	prefs, err := c.store.Get(ctx, playerID)
	if err != nil {
		return nil, err
	}
	var words []string
	if prefs != nil {
		words = prefs.ExtraWords
	}
	encoded, err := json.Marshal(words)
	if err != nil {
		observability.Warn(ctx, "chatprefs cache encode failed", err, "player_id", playerID)
		return words, nil
	}
	if err := c.backend.Set(ctx, key, encoded, extraWordsCacheTTLSeconds); err != nil {
		observability.Warn(ctx, "chatprefs cache write failed", err, "player_id", playerID)
	}
	return words, nil
}
