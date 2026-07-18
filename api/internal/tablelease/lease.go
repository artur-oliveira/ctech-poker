// Package tablelease implements the single-writer-per-table directory
// service described in ARCHITECTURE.md § 2: a Valkey key `table:{id}` holds
// the owning instance's token with a TTL, renewed on a heartbeat for as long
// as that instance keeps processing the table. Ports the acquire/CAS-release
// primitive from ctech-wallet/api/internal/lock/lock.go and adds renewal,
// since a table lease is held for a process's entire lifetime handling that
// table, not for one short operation.
package tablelease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
	"gopkg.aoctech.app/api-commons/cache"
)

// DefaultLeaseTTL bounds how long a lease is held before it auto-expires, so
// a crashed instance can never wedge a table forever.
const DefaultLeaseTTL = 15 * time.Second

// DefaultHeartbeatInterval is how often StartHeartbeat renews an active
// lease — well under DefaultLeaseTTL so a slow renewal never lets it lapse.
const DefaultHeartbeatInterval = 5 * time.Second

const leaseKeyFmt = "table:%s"

type store interface {
	setNX(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	renewIfMatch(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	delIfMatch(ctx context.Context, key, token string) error
}

// Service acquires and renews per-table leases.
type Service struct {
	store    store
	leaseTTL time.Duration

	mu     sync.Mutex
	tokens map[string]string // tableID -> this process's current token, for Renew
}

// NewService returns a Valkey-backed lease service when the cache backend is
// Redis, otherwise an in-memory one (dev/single-replica only).
func NewService(c cache.Backend) *Service {
	s := &Service{leaseTTL: DefaultLeaseTTL, tokens: make(map[string]string)}
	if rb, ok := c.(*cache.RedisBackend); ok {
		s.store = &redisStore{client: rb.Client()}
	} else {
		s.store = newMemStore()
	}
	return s
}

// Acquire takes the lease for one table. On success it returns a release
// func (safe to call once) and ok=true. On contention it returns ok=false.
func (s *Service) Acquire(ctx context.Context, tableID string) (release func(), ok bool, err error) {
	token, err := newToken()
	if err != nil {
		return nil, false, err
	}
	key := fmt.Sprintf(leaseKeyFmt, tableID)
	got, err := s.store.setNX(ctx, key, token, s.leaseTTL)
	if err != nil {
		return nil, false, err
	}
	if !got {
		return nil, false, nil
	}
	s.mu.Lock()
	s.tokens[tableID] = token
	s.mu.Unlock()
	return func() {
		_ = s.store.delIfMatch(context.Background(), key, token)
		s.mu.Lock()
		delete(s.tokens, tableID)
		s.mu.Unlock()
	}, true, nil
}

// Renew extends the TTL of a lease this process currently holds. Returns an
// error if this process no longer holds it (e.g. it already expired and was
// re-acquired elsewhere) — the caller (table server) must treat that as
// "I've lost authority over this table" and stop processing immediately.
func (s *Service) Renew(ctx context.Context, tableID string) error {
	s.mu.Lock()
	token, held := s.tokens[tableID]
	s.mu.Unlock()
	if !held {
		return fmt.Errorf("tablelease: no lease held locally for table %s", tableID)
	}
	key := fmt.Sprintf(leaseKeyFmt, tableID)
	ok, err := s.store.renewIfMatch(ctx, key, token, s.leaseTTL)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("tablelease: lease for table %s was lost (token mismatch or expired)", tableID)
	}
	return nil
}

// StartHeartbeat renews the lease for tableID on DefaultHeartbeatInterval
// until the returned stop func is called or Renew fails (lease lost) — in
// which case it calls onLost, if provided.
func (s *Service) StartHeartbeat(ctx context.Context, tableID string, onLost func()) (stop func()) {
	loopCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(DefaultHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				if err := s.Renew(loopCtx, tableID); err != nil {
					if onLost != nil {
						onLost()
					}
					return
				}
			}
		}
	}()
	return cancel
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- redis-backed store ---

type redisStore struct {
	client valkey.Client
}

func (s *redisStore) setNX(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	_, err := s.client.Do(ctx, s.client.B().Set().Key(key).Value(token).Nx().Ex(ttl).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

const casRenewScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("expire", KEYS[1], ARGV[2])
end
return 0
`

func (s *redisStore) renewIfMatch(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	n, err := s.client.Do(ctx, s.client.B().Eval().Script(casRenewScript).Numkeys(1).Key(key).Arg(token, fmt.Sprintf("%d", int(ttl.Seconds()))).Build()).ToInt64()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

const casDelScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`

func (s *redisStore) delIfMatch(ctx context.Context, key, token string) error {
	return s.client.Do(ctx, s.client.B().Eval().Script(casDelScript).Numkeys(1).Key(key).Arg(token).Build()).Error()
}

// --- in-memory store (single replica / tests) ---

type memEntry struct {
	token   string
	expires time.Time
}

type memStore struct {
	mu   sync.Mutex
	keys map[string]memEntry
}

func newMemStore() *memStore { return &memStore{keys: make(map[string]memEntry)} }

func (s *memStore) setNX(_ context.Context, key, token string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.keys[key]; ok && time.Now().Before(e.expires) {
		return false, nil
	}
	s.keys[key] = memEntry{token: token, expires: time.Now().Add(ttl)}
	return true, nil
}

func (s *memStore) renewIfMatch(_ context.Context, key, token string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.keys[key]
	if !ok || e.token != token || time.Now().After(e.expires) {
		return false, nil
	}
	s.keys[key] = memEntry{token: token, expires: time.Now().Add(ttl)}
	return true, nil
}

func (s *memStore) delIfMatch(_ context.Context, key, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.keys[key]; ok && e.token == token {
		delete(s.keys, key)
	}
	return nil
}
