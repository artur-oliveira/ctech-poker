package social

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

type memoryEventStore struct {
	events    map[string]Event
	accepts   int
	createErr error
}

func newMemoryEventStore() *memoryEventStore {
	return &memoryEventStore{events: make(map[string]Event)}
}
func memoryEventKey(recipient, id string) string { return recipient + "\x00" + id }

func (m *memoryEventStore) Create(_ context.Context, event Event, key string) (*Event, error) {
	event = prepareEvent(event, key)
	m.events[memoryEventKey(event.RecipientPlayerID, event.EventID)] = event
	return &event, m.createErr
}
func (m *memoryEventStore) CreateInvite(ctx context.Context, event Event, key string) (*Event, error) {
	return m.Create(ctx, event, key)
}
func (m *memoryEventStore) Get(_ context.Context, recipient, id string) (*Event, error) {
	event, ok := m.events[memoryEventKey(recipient, id)]
	if !ok {
		return nil, ErrEventNotFound
	}
	copy := event
	return &copy, nil
}
func (m *memoryEventStore) List(context.Context, string, int, map[string]types.AttributeValue) ([]Event, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}
func (m *memoryEventStore) UnreadCount(context.Context, string) (int, error) { return 1, nil }
func (m *memoryEventStore) MarkRead(context.Context, string, []string) error { return nil }
func (m *memoryEventStore) AcceptInvite(_ context.Context, event Event, now time.Time) (*Event, error) {
	if event.ExpiresAt <= now.UnixMilli() {
		return nil, ErrInviteExpired
	}
	m.accepts++
	event.Status, event.Unread = EventStatusAccepted, false
	m.events[memoryEventKey(event.RecipientPlayerID, event.EventID)] = event
	return &event, nil
}
func (m *memoryEventStore) DeclineInvite(_ context.Context, event Event, now time.Time) (*Event, error) {
	if event.ExpiresAt <= now.UnixMilli() {
		return nil, ErrInviteExpired
	}
	event.Status, event.Unread = EventStatusDeclined, false
	m.events[memoryEventKey(event.RecipientPlayerID, event.EventID)] = event
	return &event, nil
}

type inviteRooms struct{ room *roomstore.Room }

func (r inviteRooms) Get(context.Context, string) (*roomstore.Room, error) { return r.room, nil }

type inviteSessions struct{ open bool }

func (s inviteSessions) FindOpenSession(context.Context, string, string) (*sessionlog.SessionItem, error) {
	if !s.open {
		return nil, nil
	}
	return &sessionlog.SessionItem{EndedAt: 0}, nil
}

func friendPair(store *memoryEdgeStore, left, right string) {
	store.edges[edgeMapKey(left, right)] = Edge{OwnerPlayerID: left, OtherPlayerID: right, Relationship: RelationshipFriend, Version: 1}
	store.edges[edgeMapKey(right, left)] = Edge{OwnerPlayerID: right, OtherPlayerID: left, Relationship: RelationshipFriend, Version: 1}
}

func TestTableInviteRequiresFriendAndOpenSenderSession(t *testing.T) {
	edges := newMemoryEdgeStore()
	events := newMemoryEventStore()
	room := &roomstore.Room{ID: "room-1", Visibility: "private", Status: "waiting", MaxSeats: 6}
	svc := NewService(edges, true).WithInbox(events).WithInvites(inviteRooms{room}, inviteSessions{open: true})
	svc.now = func() time.Time { return transitionTime }

	if _, err := svc.SendTableInvite(context.Background(), "sender", "guest", room.ID, "invite-1"); !errors.Is(err, ErrInviteForbidden) {
		t.Fatalf("non-friend invite err=%v", err)
	}
	friendPair(edges, "sender", "guest")
	svc.sessions = inviteSessions{open: false}
	if _, err := svc.SendTableInvite(context.Background(), "sender", "guest", room.ID, "invite-1"); !errors.Is(err, ErrInviteForbidden) {
		t.Fatalf("closed-session invite err=%v", err)
	}
	svc.sessions = inviteSessions{open: true}
	event, err := svc.SendTableInvite(context.Background(), "sender", "guest", room.ID, "invite-1")
	if err != nil {
		t.Fatal(err)
	}
	if event.ExpiresAt-event.CreatedAt != InviteLifetime.Milliseconds() || event.RoomID != room.ID {
		t.Fatalf("unexpected invite: %+v", event)
	}
}

func TestTableInviteAcceptanceFailsBeforeGrantWhenExpiredFullOrBlocked(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Service, *memoryEdgeStore, *roomstore.Room, *Event)
		want   error
	}{
		{name: "expired", mutate: func(s *Service, _ *memoryEdgeStore, _ *roomstore.Room, event *Event) {
			s.now = func() time.Time { return time.UnixMilli(event.ExpiresAt) }
		}, want: ErrInviteExpired},
		{name: "full", mutate: func(_ *Service, _ *memoryEdgeStore, room *roomstore.Room, _ *Event) { room.SeatsTaken = room.MaxSeats }, want: ErrRoomFull},
		{name: "blocked after send", mutate: func(_ *Service, edges *memoryEdgeStore, _ *roomstore.Room, _ *Event) {
			edge := edges.edges[edgeMapKey("guest", "sender")]
			edge.Blocked = true
			edges.edges[edgeMapKey("guest", "sender")] = edge
		}, want: ErrInviteForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			edges := newMemoryEdgeStore()
			friendPair(edges, "sender", "guest")
			events := newMemoryEventStore()
			room := &roomstore.Room{ID: "room-1", Visibility: "private", Status: "waiting", MaxSeats: 2}
			svc := NewService(edges, true).WithInbox(events).WithInvites(inviteRooms{room}, inviteSessions{open: true})
			svc.now = func() time.Time { return transitionTime }
			event, err := svc.SendTableInvite(context.Background(), "sender", "guest", room.ID, "invite-1")
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(svc, edges, room, event)
			if _, _, err := svc.AcceptTableInvite(context.Background(), "guest", event.EventID); !errors.Is(err, test.want) {
				t.Fatalf("accept err=%v want=%v", err, test.want)
			}
			if events.accepts != 0 {
				t.Fatal("grant transaction was attempted before validation completed")
			}
		})
	}
}
