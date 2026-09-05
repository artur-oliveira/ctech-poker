package presence

import (
	"context"
	"log/slog"
	"time"
)

const (
	HeartbeatInterval = 30 * time.Second
	ConnectionTTL     = 75 * time.Second

	// opBudget bounds one presence operation (#223). Every caller here is a
	// WebSocket lifecycle event: Open and Heartbeat run under the socket's own
	// context, which lives as long as the connection does, and Close runs under
	// context.Background() so a disconnect can still be recorded — none of them
	// carried a deadline, so an unreachable Valkey (or a slow session lookup)
	// parked the connect path, a heartbeat tick, or a socket teardown
	// indefinitely. Presence is decorative state with its own TTL
	// (ConnectionTTL): a dropped update self-heals on the next heartbeat, so
	// failing fast is strictly better than hanging.
	opBudget = 2 * time.Second

	// openBudget is Open's and Reconcile's larger allowance: they also do a
	// DynamoDB session lookup and a friend fan-out on top of the cache write.
	openBudget = 3 * time.Second
)

type FriendSource interface {
	FriendIDs(ctx context.Context, playerID string) ([]string, error)
}

type SessionSource interface {
	FindLatestOpenSession(ctx context.Context, playerID string) (string, error)
}

// NotifyFunc fans a status change out to one friend. roomID is passed through
// unfiltered from presence's own store — it carries no visibility
// decision (this package stays room-blind by design, see model.go); a
// caller wiring this must decide whether to actually publish it (#334),
// mirroring the same gate api/v1/social.go's joinableRoomIDs applies to the
// pull path.
type NotifyFunc func(ctx context.Context, recipientID, playerID string, status Status, roomID string)

type Service struct {
	store    Store
	friends  FriendSource
	sessions SessionSource
	notify   NotifyFunc
	now      func() time.Time
	// opBudget/openBudget are the deadlines above, as fields only so tests can
	// shrink them.
	opBudget   time.Duration
	openBudget time.Duration
}

func NewService(store Store, friends FriendSource, sessions SessionSource, notify NotifyFunc) *Service {
	return &Service{store: store, friends: friends, sessions: sessions, notify: notify, now: time.Now,
		opBudget: opBudget, openBudget: openBudget}
}

func (s *Service) Open(ctx context.Context, playerID, connectionID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.openBudget)
	defer cancel()
	becameOnline, err := s.store.Open(ctx, playerID, connectionID, s.now().Add(ConnectionTTL))
	if err != nil {
		return err
	}
	if s.sessions != nil {
		if roomID, sessionErr := s.sessions.FindLatestOpenSession(ctx, playerID); sessionErr != nil {
			slog.Warn("presence: session reconciliation failed", "player", playerID, "err", sessionErr)
		} else if _, setErr := s.store.SetInTable(ctx, playerID, roomID); setErr != nil {
			return setErr
		}
	}
	if becameOnline {
		s.broadcastCurrent(ctx, playerID)
	}
	return nil
}

func (s *Service) Heartbeat(ctx context.Context, playerID, connectionID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.opBudget)
	defer cancel()
	becameOnline, err := s.store.Heartbeat(ctx, playerID, connectionID, s.now().Add(ConnectionTTL))
	if err == nil && becameOnline {
		s.broadcastCurrent(ctx, playerID)
	}
	return err
}

func (s *Service) Close(ctx context.Context, playerID, connectionID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.opBudget)
	defer cancel()
	becameOffline, err := s.store.Close(ctx, playerID, connectionID)
	if err == nil && becameOffline {
		s.broadcast(ctx, playerID, StatusOffline, "")
	}
	return err
}

func (s *Service) SetInTable(ctx context.Context, playerID, roomID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.opBudget)
	defer cancel()
	changed, err := s.store.SetInTable(ctx, playerID, roomID)
	if err == nil && changed {
		entries, statusErr := s.store.GetMany(ctx, []string{playerID})
		if statusErr != nil {
			return statusErr
		}
		if entries[playerID].Status != StatusOffline {
			s.broadcast(ctx, playerID, entries[playerID].Status, entries[playerID].RoomID)
		}
	}
	return err
}

func (s *Service) Reconcile(ctx context.Context, playerID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.openBudget)
	defer cancel()
	if s.sessions == nil {
		return nil
	}
	roomID, err := s.sessions.FindLatestOpenSession(ctx, playerID)
	if err != nil {
		return err
	}
	return s.SetInTable(ctx, playerID, roomID)
}

func (s *Service) GetMany(ctx context.Context, playerIDs []string) (map[string]PlayerPresence, error) {
	ctx, cancel := context.WithTimeout(ctx, s.opBudget)
	defer cancel()
	return s.store.GetMany(ctx, playerIDs)
}

func (s *Service) broadcastCurrent(ctx context.Context, playerID string) {
	entries, err := s.store.GetMany(ctx, []string{playerID})
	if err == nil {
		entry := entries[playerID]
		s.broadcast(ctx, playerID, entry.Status, entry.RoomID)
	}
}

func (s *Service) broadcast(ctx context.Context, playerID string, status Status, roomID string) {
	if s.friends == nil || s.notify == nil {
		return
	}
	friends, err := s.friends.FriendIDs(ctx, playerID)
	if err != nil {
		slog.Warn("presence: friend fanout lookup failed", "player", playerID, "err", err)
		return
	}
	for _, friendID := range friends {
		s.notify(ctx, friendID, playerID, status, roomID)
	}
}
