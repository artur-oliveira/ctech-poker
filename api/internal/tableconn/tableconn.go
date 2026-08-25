// Package tableconn shares which players currently hold a live table socket,
// across every API instance serving one table.
//
// An Actor's activeConns map only ever knows about sockets terminating on its
// own process, and any instance may run an Actor for any table. The seat's
// connection dot is therefore wrong in both directions: an instance holding a
// stale disconnect mark shows a phantom "disconnected" for someone who
// reconnected elsewhere, and an instance that never saw the socket at all
// shows "connected" (applyPresence's default) for someone genuinely gone.
//
// This is display state only. The auto-kick decision deliberately does NOT
// read it — a removal must rest on persisted evidence (hand.Player's
// LastActionAt), never on a cache that a Valkey outage can blank. See
// Actor.handleKickTimeout.
package tableconn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

// keyPrefix namespaces every key this package owns.
const keyPrefix = "poker:tableconn:"

// EntryTTL is how long one Sync's claim stays valid. It has to outlive
// SyncInterval by enough that an instance still serving a player never lets
// their entry lapse, while bounding how long a hard-crashed instance's
// players keep showing as connected.
const EntryTTL = 45 * time.Second

// SyncInterval paces the round trip. Lifecycle events (connect, disconnect)
// sync immediately; steady-state broadcasts sync at most this often, so a busy
// table costs one Valkey round trip per interval rather than one per action.
const SyncInterval = 15 * time.Second

// KeyTTL expires the whole table's entry once nobody syncs it any more.
const KeyTTL = 10 * time.Minute

var timeNowFunc = time.Now

// Service reads and refreshes one table's fleet-wide connection set.
type Service struct{ cache cache.Backend }

func NewService(c cache.Backend) *Service { return &Service{cache: c} }

func key(tableID string) string { return keyPrefix + tableID }

// Sync publishes localPlayerIDs as still-connected on this instance and
// returns every player the fleet currently considers connected to tableID.
//
// A nil Service (dev/tests without a cache) returns a nil map, which callers
// read as "nothing shared — trust the local view", exactly as
// tablestreak.Load does.
//
// Read-modify-write is deliberate and sufficient. Two instances syncing at the
// same instant can drop one of the two writes, which costs one interval of a
// wrong dot for those seats before the next Sync restores it. It is never
// consulted for anything but the dot.
func (s *Service) Sync(ctx context.Context, tableID string, localPlayerIDs []string) (map[string]bool, error) {
	if s == nil || s.cache == nil {
		return nil, nil
	}
	raw, found, err := s.cache.Get(ctx, key(tableID))
	if err != nil {
		return nil, fmt.Errorf("tableconn: load %s: %w", tableID, err)
	}
	expiries := map[string]int64{}
	if found {
		if err := json.Unmarshal(raw, &expiries); err != nil {
			return nil, fmt.Errorf("tableconn: decode %s: %w", tableID, err)
		}
	}
	now := timeNowFunc()
	for playerID, expiry := range expiries {
		if expiry <= now.UnixMilli() {
			delete(expiries, playerID)
		}
	}
	refreshed := now.Add(EntryTTL).UnixMilli()
	for _, playerID := range localPlayerIDs {
		if playerID != "" {
			expiries[playerID] = refreshed
		}
	}
	encoded, err := json.Marshal(expiries)
	if err != nil {
		return nil, fmt.Errorf("tableconn: encode %s: %w", tableID, err)
	}
	if err := s.cache.Set(ctx, key(tableID), encoded, int(KeyTTL.Seconds())); err != nil {
		return nil, fmt.Errorf("tableconn: save %s: %w", tableID, err)
	}
	connected := make(map[string]bool, len(expiries))
	for playerID := range expiries {
		connected[playerID] = true
	}
	return connected, nil
}
