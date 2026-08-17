package presence

import (
	"context"
	"log/slog"
	"time"
)

const (
	HeartbeatInterval = 30 * time.Second
	ConnectionTTL     = 75 * time.Second
)

type FriendSource interface {
	FriendIDs(ctx context.Context, playerID string) ([]string, error)
}

type SessionSource interface {
	FindLatestOpenSession(ctx context.Context, playerID string) (bool, error)
}

type NotifyFunc func(context.Context, string, string, Status)

type Service struct {
	store    Store
	friends  FriendSource
	sessions SessionSource
	notify   NotifyFunc
	now      func() time.Time
}

func NewService(store Store, friends FriendSource, sessions SessionSource, notify NotifyFunc) *Service {
	return &Service{store: store, friends: friends, sessions: sessions, notify: notify, now: time.Now}
}

func (s *Service) Open(ctx context.Context, playerID, connectionID string) error {
	becameOnline, err := s.store.Open(ctx, playerID, connectionID, s.now().Add(ConnectionTTL))
	if err != nil {
		return err
	}
	if s.sessions != nil {
		if inTable, sessionErr := s.sessions.FindLatestOpenSession(ctx, playerID); sessionErr != nil {
			slog.Warn("presence: session reconciliation failed", "player", playerID, "err", sessionErr)
		} else if _, setErr := s.store.SetInTable(ctx, playerID, inTable); setErr != nil {
			return setErr
		}
	}
	if becameOnline {
		s.broadcastCurrent(ctx, playerID)
	}
	return nil
}

func (s *Service) Heartbeat(ctx context.Context, playerID, connectionID string) error {
	becameOnline, err := s.store.Heartbeat(ctx, playerID, connectionID, s.now().Add(ConnectionTTL))
	if err == nil && becameOnline {
		s.broadcastCurrent(ctx, playerID)
	}
	return err
}

func (s *Service) Close(ctx context.Context, playerID, connectionID string) error {
	becameOffline, err := s.store.Close(ctx, playerID, connectionID)
	if err == nil && becameOffline {
		s.broadcast(ctx, playerID, StatusOffline)
	}
	return err
}

func (s *Service) SetInTable(ctx context.Context, playerID string, inTable bool) error {
	changed, err := s.store.SetInTable(ctx, playerID, inTable)
	if err == nil && changed {
		statuses, statusErr := s.store.GetMany(ctx, []string{playerID})
		if statusErr != nil {
			return statusErr
		}
		if statuses[playerID] != StatusOffline {
			s.broadcast(ctx, playerID, statuses[playerID])
		}
	}
	return err
}

func (s *Service) Reconcile(ctx context.Context, playerID string) error {
	if s.sessions == nil {
		return nil
	}
	inTable, err := s.sessions.FindLatestOpenSession(ctx, playerID)
	if err != nil {
		return err
	}
	return s.SetInTable(ctx, playerID, inTable)
}

func (s *Service) GetMany(ctx context.Context, playerIDs []string) (map[string]Status, error) {
	return s.store.GetMany(ctx, playerIDs)
}

func (s *Service) broadcastCurrent(ctx context.Context, playerID string) {
	statuses, err := s.store.GetMany(ctx, []string{playerID})
	if err == nil {
		s.broadcast(ctx, playerID, statuses[playerID])
	}
}

func (s *Service) broadcast(ctx context.Context, playerID string, status Status) {
	if s.friends == nil || s.notify == nil {
		return
	}
	friends, err := s.friends.FriendIDs(ctx, playerID)
	if err != nil {
		slog.Warn("presence: friend fanout lookup failed", "player", playerID, "err", err)
		return
	}
	for _, friendID := range friends {
		s.notify(ctx, friendID, playerID, status)
	}
}
