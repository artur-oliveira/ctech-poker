package app

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/presence"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/social"
)

type socialRecordingConn struct{ messages [][]byte }

func (c *socialRecordingConn) WriteMessage(_ int, data []byte) error {
	c.messages = append(c.messages, append([]byte(nil), data...))
	return nil
}

// fakePresenceProfiles/fakePresenceRooms fake the two narrow interfaces
// newPresenceNotifier's push gate needs, so #334's gating can be pinned
// without a live DynamoDB.
type fakePresenceProfiles map[string]player.PlayerProfile

func (f fakePresenceProfiles) Get(_ context.Context, userID string) (*player.PlayerProfile, error) {
	profile, ok := f[userID]
	if !ok {
		return nil, nil
	}
	return &profile, nil
}

type fakePresenceRooms map[string]roomstore.Room

func (f fakePresenceRooms) Get(_ context.Context, roomID string) (*roomstore.Room, error) {
	room, ok := f[roomID]
	if !ok {
		return nil, nil
	}
	return &room, nil
}

func decodePresenceFrame(t *testing.T, data []byte) *pokerproto.ServerMessage {
	t.Helper()
	var message pokerproto.ServerMessage
	if err := proto.Unmarshal(data, &message); err != nil {
		t.Fatal(err)
	}
	return &message
}

func TestPresenceNotifierContainsNoTableIdentityAndOnlyTargetsFriend(t *testing.T) {
	registry := ws.NewMemoryRegistry()
	friend, other := &socialRecordingConn{}, &socialRecordingConn{}
	registry.Register("user#friend", "friend-conn", friend)
	registry.Register("user#other", "other-conn", other)

	notify := newPresenceNotifier(registry, fakePresenceProfiles{}, fakePresenceRooms{})
	notify(context.Background(), "friend", "player-1", presence.StatusInTable, "")

	if len(friend.messages) != 1 || len(other.messages) != 0 {
		t.Fatalf("friend messages=%d other messages=%d", len(friend.messages), len(other.messages))
	}
	message := decodePresenceFrame(t, friend.messages[0])
	if message.Type != "social_presence_changed" || message.SocialEvent == nil || message.SocialEvent.Presence == nil ||
		message.SocialEvent.Presence.PlayerId != "player-1" || message.SocialEvent.Presence.Status != "in_table" ||
		message.SocialEvent.RoomId != "" || message.SocialEvent.Presence.RoomId != "" {
		t.Fatalf("unexpected presence frame: %+v", message)
	}
}

// TestPresenceNotifierRoomIDGate pins #334: a pushed RoomID requires the
// subject player's own TablePublic opt-in AND a public, joinable room — the
// same two gates api/v1/social.go's joinableRoomIDs applies to the pull path.
func TestPresenceNotifierRoomIDGate(t *testing.T) {
	publicOpenRoom := roomstore.Room{ID: "room-1", Visibility: "public", Status: "waiting", MaxSeats: 6, SeatsTaken: 3}
	privateRoom := roomstore.Room{ID: "room-2", Visibility: "private", Status: "waiting", MaxSeats: 6, SeatsTaken: 3}
	fullRoom := roomstore.Room{ID: "room-3", Visibility: "public", Status: "waiting", MaxSeats: 6, SeatsTaken: 6}

	tests := []struct {
		name       string
		tablePub   bool
		roomID     string
		rooms      fakePresenceRooms
		wantRoomID string
	}{
		{"opted in, public room with a seat", true, "room-1", fakePresenceRooms{"room-1": publicOpenRoom}, "room-1"},
		{"not opted in", false, "room-1", fakePresenceRooms{"room-1": publicOpenRoom}, ""},
		{"opted in, private room", true, "room-2", fakePresenceRooms{"room-2": privateRoom}, ""},
		{"opted in, full room", true, "room-3", fakePresenceRooms{"room-3": fullRoom}, ""},
		{"opted in, no room known", true, "", fakePresenceRooms{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			registry := ws.NewMemoryRegistry()
			friend := &socialRecordingConn{}
			registry.Register("user#friend", "friend-conn", friend)
			profiles := fakePresenceProfiles{"player-1": player.PlayerProfile{UserID: "player-1", TablePublic: tc.tablePub}}
			notify := newPresenceNotifier(registry, profiles, tc.rooms)
			notify(context.Background(), "friend", "player-1", presence.StatusInTable, tc.roomID)

			message := decodePresenceFrame(t, friend.messages[0])
			if got := message.SocialEvent.Presence.RoomId; got != tc.wantRoomID {
				t.Fatalf("RoomId = %q, want %q", got, tc.wantRoomID)
			}
		})
	}
}

// TestPresenceNotifierNeverPushesRoomIDWhenNotInTable pins the third gate:
// even an opted-in player with a known joinable room never gets a RoomID
// published for a non-in_table status transition.
func TestPresenceNotifierNeverPushesRoomIDWhenNotInTable(t *testing.T) {
	registry := ws.NewMemoryRegistry()
	friend := &socialRecordingConn{}
	registry.Register("user#friend", "friend-conn", friend)
	profiles := fakePresenceProfiles{"player-1": player.PlayerProfile{UserID: "player-1", TablePublic: true}}
	rooms := fakePresenceRooms{"room-1": {ID: "room-1", Visibility: "public", Status: "waiting", MaxSeats: 6, SeatsTaken: 3}}
	notify := newPresenceNotifier(registry, profiles, rooms)
	notify(context.Background(), "friend", "player-1", presence.StatusOnline, "room-1")

	message := decodePresenceFrame(t, friend.messages[0])
	if message.SocialEvent.Presence.RoomId != "" {
		t.Fatalf("expected no RoomId for a non-in_table status, got %+v", message)
	}
}

func TestSocialNotifierFansOutEventAndUnreadCountOnlyToRecipient(t *testing.T) {
	registry := ws.NewMemoryRegistry()
	recipient, other := &socialRecordingConn{}, &socialRecordingConn{}
	registry.Register("user#guest", "guest-conn", recipient)
	registry.Register("user#other", "other-conn", other)

	newSocialNotifier(registry)(context.Background(), social.Event{
		RecipientPlayerID: "guest", EventID: "event-1", Type: social.EventTableInvite,
		ActorPlayerID: "sender", RoomID: "room-1", Status: social.EventStatusPending,
		CreatedAt: 10, ExpiresAt: 20,
	}, 3)

	if len(recipient.messages) != 1 || len(other.messages) != 0 {
		t.Fatalf("recipient messages=%d other messages=%d", len(recipient.messages), len(other.messages))
	}
	var message pokerproto.ServerMessage
	if err := proto.Unmarshal(recipient.messages[0], &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "social_event" || message.UnreadCount != 3 || message.SocialEvent == nil || message.SocialEvent.RoomId != "room-1" {
		t.Fatalf("unexpected message: %+v", &message)
	}
}
