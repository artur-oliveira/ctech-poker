// Package tablestreak keeps a table's per-player win/loss streak badge in the
// shared cache (Valkey in production) instead of one map per API instance.
//
// Any instance may run an Actor for any table (see tablemanager's package
// doc), and the badge used to live only in that Actor's memory, filled from
// the hands that one process happened to run. Two live actors therefore
// published two different numbers for the same seat, and clients — which
// receive both instances' broadcasts over the same socket — saw the badge
// flip on consecutive snapshots of one hand (V2, V4, V2, ...).
//
// DynamoDB (achievements.Store) remains the durable copy and the only writer
// of record; this is the cross-instance display cache in front of it.
package tablestreak

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

// keyPrefix namespaces every key this package owns, so a table ID can never
// collide with an unrelated key sharing the same Valkey instance.
const keyPrefix = "poker:tablestreak:"

// TTL bounds how long an untouched table keeps its badges cached. Expiry
// costs only the badge — the durable counters in achievements.Store are
// unaffected, and the next completed hand republishes.
const TTL = 24 * time.Hour

// Service reads and merges one table's streak map.
type Service struct{ cache cache.Backend }

func NewService(c cache.Backend) *Service { return &Service{cache: c} }

func key(tableID string) string { return keyPrefix + tableID }

// Load returns every streak recorded for tableID, or an empty map when the
// table has none yet. A nil Service (dev/tests without a cache) returns a nil
// map so callers can tell "nothing shared" from "shared and empty" and keep
// whatever they already had.
func (s *Service) Load(ctx context.Context, tableID string) (map[string]int, error) {
	if s == nil || s.cache == nil {
		return nil, nil
	}
	raw, found, err := s.cache.Get(ctx, key(tableID))
	if err != nil {
		return nil, fmt.Errorf("tablestreak: load %s: %w", tableID, err)
	}
	if !found {
		return map[string]int{}, nil
	}
	streaks := map[string]int{}
	if err := json.Unmarshal(raw, &streaks); err != nil {
		return nil, fmt.Errorf("tablestreak: decode %s: %w", tableID, err)
	}
	return streaks, nil
}

// Merge folds one completed hand's streaks into the shared map and returns
// the merged result. Read-modify-write is safe here: exactly one actor runs
// any given hand to completion, so two instances never merge the same table
// at the same moment, and a lost update would cost one badge refresh that the
// next completed hand corrects. It is display state, never money.
func (s *Service) Merge(ctx context.Context, tableID string, streaks map[string]int) (map[string]int, error) {
	if s == nil || s.cache == nil || len(streaks) == 0 {
		return nil, nil
	}
	merged, err := s.Load(ctx, tableID)
	if err != nil {
		return nil, err
	}
	if merged == nil {
		merged = make(map[string]int, len(streaks))
	}
	maps.Copy(merged, streaks)
	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("tablestreak: encode %s: %w", tableID, err)
	}
	if err := s.cache.Set(ctx, key(tableID), encoded, int(TTL.Seconds())); err != nil {
		return nil, fmt.Errorf("tablestreak: save %s: %w", tableID, err)
	}
	return merged, nil
}
