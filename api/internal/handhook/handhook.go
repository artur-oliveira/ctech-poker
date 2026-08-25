// Package handhook grants the fleet-wide right to run one completed hand's
// post-hand hooks exactly once.
//
// hand.Table.lastOutcome is part of the persisted state (see
// engine/hand/state.go), so ANY instance that loads a table sitting on
// Complete and calls broadcastAll fires notifyHandComplete — and broadcastAll
// runs from every chat message, reaction, connect and disconnect, not only
// from the action that completed the hand. The only thing that used to stop a
// second instance from firing again was the Actor's own completedHandNotified
// field, which is per process. A player typing in chat on instance B during
// the post-hand countdown therefore re-ran achievements.RecordHand,
// RecordTableStreak and the auto-rebuy sweep for a hand instance A had
// already credited, and none of those are idempotent: they use bare
// Increment/IncrementStreak with no per-hand guard, so hands_played moved by
// two, streaks jumped, and tier unlocks were emitted twice.
//
// The claim has to be atomic — a read-then-write would let two instances both
// observe "unclaimed" — so this uses SET NX rather than the cache.Backend
// abstraction, whose Get/Set pair cannot express it. Same reason
// internal/presence talks to valkey directly.
package handhook

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

// keyPrefix namespaces every key this package owns.
const keyPrefix = "poker:handhook:"

// TTL only has to outlive the window in which some instance might still
// broadcast this hand as Complete — the post-hand countdown plus generous
// slack for a reload. Nothing reads the value back, so expiry costs nothing.
const TTL = time.Hour

// Service claims hand hooks against a shared Valkey.
type Service struct{ client valkey.Client }

// NewService returns a Service over client. A nil client (dev/tests with no
// Valkey) grants every claim, leaving the Actor's own per-process guard as
// the only dedupe — correct there, because a single instance is the whole
// fleet.
func NewService(client valkey.Client) *Service { return &Service{client: client} }

func key(tableID, handID string) string { return keyPrefix + tableID + ":" + handID }

// Claim reports whether this caller is the one allowed to run tableID/handID's
// post-hand hooks. Exactly one concurrent caller across the fleet gets true.
func (s *Service) Claim(ctx context.Context, tableID, handID string) (bool, error) {
	if s == nil || s.client == nil || handID == "" {
		return true, nil
	}
	cmd := s.client.B().Set().Key(key(tableID, handID)).Value("1").Nx().ExSeconds(int64(TTL.Seconds())).Build()
	// SET NX answers with a nil reply, not an error, when the key already
	// exists — that is the "someone else already claimed it" signal.
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return false, nil
		}
		return false, fmt.Errorf("handhook: claim %s/%s: %w", tableID, handID, err)
	}
	return true, nil
}
