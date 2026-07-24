// Package tablelease implements the single-writer-per-table directory
// service described in ARCHITECTURE.md § 2: a Valkey key `table:{id}` holds
// the owning instance's token with a TTL, renewed on a heartbeat for as long
// as that instance keeps processing the table.
//
// The CAS acquire/renew/release mechanics are shared with ctech-wallet's
// per-wallet lock in gopkg.aoctech.app/api-commons/lock; this package only
// adds poker's own TTL/heartbeat defaults and the "table:" key namespace, so
// a table ID can never collide with an unrelated key sharing the same
// Valkey instance.
package tablelease

import (
	"context"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	sharedlock "gopkg.aoctech.app/api-commons/lock"
)

// DefaultLeaseTTL bounds how long a lease is held before it auto-expires, so
// a crashed instance can never wedge a table forever.
const DefaultLeaseTTL = 15 * time.Second

// DefaultHeartbeatInterval is how often StartHeartbeat renews an active
// lease — well under DefaultLeaseTTL so a slow renewal never lets it lapse.
const DefaultHeartbeatInterval = 5 * time.Second

// leaseKeyPrefix namespaces every key this package locks.
const leaseKeyPrefix = "table:"

// Service acquires and renews per-table leases. The CAS mechanics live in
// the shared gopkg.aoctech.app/api-commons/lock package; this type only
// adds the "table:" key namespace and poker's own TTL/heartbeat defaults.
type Service struct {
	*sharedlock.Locker
}

// NewService returns a Valkey-backed lease service when the cache backend is
// Redis, otherwise an in-memory one (dev/single-replica only).
func NewService(c cache.Backend) *Service {
	return &Service{sharedlock.New(c, DefaultLeaseTTL)}
}

// Acquire takes the lease for one table. On success it returns a release
// func (safe to call once) and ok=true. On contention it returns ok=false.
func (s *Service) Acquire(ctx context.Context, tableID string) (release func(), ok bool, err error) {
	return s.Locker.Acquire(ctx, leaseKeyPrefix+tableID)
}

// StartHeartbeat renews the lease for tableID on DefaultHeartbeatInterval
// until the returned stop func is called or Renew fails (lease lost) — in
// which case it calls onLost, if provided.
func (s *Service) StartHeartbeat(ctx context.Context, tableID string, onLost func()) (stop func()) {
	return s.Locker.StartHeartbeat(ctx, leaseKeyPrefix+tableID, DefaultHeartbeatInterval, onLost)
}
