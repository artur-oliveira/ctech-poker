package botfunding

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"gopkg.aoctech.app/poker/api/internal/metrics"
)

type windowStore interface {
	Load(context.Context, string) (*Window, error)
	Create(context.Context, string, string, string, int64, time.Time) (*Window, error)
	Update(context.Context, *Window, string, string, string, int64, time.Time) (*Window, error)
}

type Service struct {
	store windowStore
	limit int64
	now   func() time.Time
	mu    sync.Mutex
	cache map[string]*Window
	locks [64]sync.Mutex // bounded, independently serialized player shards
}

func NewService(store windowStore, limit int64) *Service {
	return &Service{store: store, limit: limit, now: time.Now, cache: make(map[string]*Window)}
}

func (s *Service) Eligibility(ctx context.Context, playerID string) (bool, int64, error) {
	// This marks a new/restored bot session and deliberately refreshes the
	// process cache once. Record then performs no read on the normal per-hand
	// path; its conditional write remains authoritative across instances.
	playerLock := s.playerLock(playerID)
	playerLock.Lock()
	defer playerLock.Unlock()
	w, err := s.store.Load(ctx, playerID)
	s.mu.Lock()
	if err == nil {
		s.cache[playerID] = w
	}
	s.mu.Unlock()
	if err != nil {
		return false, 0, err
	}
	available := w == nil || w.ExpiresAt <= s.now().UnixMilli() || w.NetProfit < s.limit
	if w == nil {
		return true, 0, nil
	}
	return available, w.ExpiresAt, nil
}

func (s *Service) Available(ctx context.Context, playerID string) (bool, error) {
	available, _, err := s.Eligibility(ctx, playerID)
	return available, err
}

func (s *Service) Record(ctx context.Context, playerID, tableID, handID string, delta int64) (bool, error) {
	started := time.Now()
	outcome := "error"
	defer func() {
		metrics.Record("BotFundingUpdateDuration", metrics.Milliseconds, nil, float64(time.Since(started).Milliseconds()))
		metrics.Record("BotFundingUpdates", metrics.Count, metrics.Dims{"Outcome": outcome}, 1)
	}()
	if playerID == "" || tableID == "" || handID == "" {
		return false, fmt.Errorf("bot funding: player, table and hand are required")
	}
	playerLock := s.playerLock(playerID)
	playerLock.Lock()
	defer playerLock.Unlock()
	now := s.now()
	s.mu.Lock()
	current, ok := s.cache[playerID]
	s.mu.Unlock()
	if !ok {
		var err error
		current, err = s.store.Load(ctx, playerID)
		if err != nil {
			return false, err
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if current != nil && current.ExpiresAt > now.UnixMilli() {
			if current.Hands[handKey(tableID, handID)] != 0 {
				s.setCached(playerID, current)
				outcome = "duplicate"
				return current.NetProfit < s.limit, nil
			}
			updated, err := s.store.Update(ctx, current, playerID, tableID, handID, delta, now)
			if err == nil {
				s.setCached(playerID, updated)
				outcome = "recorded"
				if updated.NetProfit >= s.limit {
					outcome = "limit_reached"
				}
				metrics.Record("BotFundingNetDelta", metrics.None, nil, float64(delta))
				return updated.NetProfit < s.limit, nil
			}
			if !IsConflict(err) {
				return false, err
			}
		} else {
			created, err := s.store.Create(ctx, playerID, tableID, handID, delta, now)
			if err == nil {
				s.setCached(playerID, created)
				outcome = "recorded"
				if created.NetProfit >= s.limit {
					outcome = "limit_reached"
				}
				metrics.Record("BotFundingNetDelta", metrics.None, nil, float64(delta))
				return created.NetProfit < s.limit, nil
			}
			if !IsConflict(err) {
				return false, err
			}
		}
		var err error
		current, err = s.store.Load(ctx, playerID)
		if err != nil {
			return false, err
		}
	}
	return false, fmt.Errorf("bot funding: concurrent window update did not converge")
}

func (s *Service) playerLock(playerID string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(playerID))
	return &s.locks[h.Sum32()%uint32(len(s.locks))]
}

func (s *Service) setCached(playerID string, window *Window) {
	s.mu.Lock()
	s.cache[playerID] = window
	s.mu.Unlock()
}
