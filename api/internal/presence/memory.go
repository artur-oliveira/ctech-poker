package presence

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is the single-process development and test adapter. Production
// uses ValkeyStore so connections from every API replica share one view.
type MemoryStore struct {
	mu          sync.Mutex
	connections map[string]map[string]time.Time
	inTable     map[string]bool
	now         func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{connections: make(map[string]map[string]time.Time), inTable: make(map[string]bool), now: time.Now}
}

func (s *MemoryStore) activeLocked(playerID string) int {
	now := s.now()
	for id, expiry := range s.connections[playerID] {
		if !expiry.After(now) {
			delete(s.connections[playerID], id)
		}
	}
	if len(s.connections[playerID]) == 0 {
		delete(s.connections, playerID)
	}
	return len(s.connections[playerID])
}

func (s *MemoryStore) Open(_ context.Context, playerID, connectionID string, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wasOffline := s.activeLocked(playerID) == 0
	if s.connections[playerID] == nil {
		s.connections[playerID] = make(map[string]time.Time)
	}
	s.connections[playerID][connectionID] = expiresAt
	return wasOffline, nil
}

func (s *MemoryStore) Heartbeat(ctx context.Context, playerID, connectionID string, expiresAt time.Time) (bool, error) {
	return s.Open(ctx, playerID, connectionID, expiresAt)
}

func (s *MemoryStore) Close(_ context.Context, playerID, connectionID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeLocked(playerID)
	_, existed := s.connections[playerID][connectionID]
	delete(s.connections[playerID], connectionID)
	return existed && s.activeLocked(playerID) == 0, nil
}

func (s *MemoryStore) SetInTable(_ context.Context, playerID string, inTable bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := s.inTable[playerID] != inTable
	if inTable {
		s.inTable[playerID] = true
	} else {
		delete(s.inTable, playerID)
	}
	return changed, nil
}

func (s *MemoryStore) GetMany(_ context.Context, playerIDs []string) (map[string]Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]Status, len(playerIDs))
	for _, playerID := range playerIDs {
		status := StatusOffline
		if s.activeLocked(playerID) > 0 {
			status = StatusOnline
			if s.inTable[playerID] {
				status = StatusInTable
			}
		}
		result[playerID] = status
	}
	return result, nil
}
