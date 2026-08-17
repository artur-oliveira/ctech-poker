package app

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/presence"
	"gopkg.aoctech.app/poker/api/internal/social"
)

type socialRecordingConn struct{ messages [][]byte }

func (c *socialRecordingConn) WriteMessage(_ int, data []byte) error {
	c.messages = append(c.messages, append([]byte(nil), data...))
	return nil
}

func TestPresenceNotifierContainsNoTableIdentityAndOnlyTargetsFriend(t *testing.T) {
	registry := ws.NewMemoryRegistry()
	friend, other := &socialRecordingConn{}, &socialRecordingConn{}
	registry.Register("user#friend", "friend-conn", friend)
	registry.Register("user#other", "other-conn", other)

	newPresenceNotifier(registry)(context.Background(), "friend", "player-1", presence.StatusInTable)

	if len(friend.messages) != 1 || len(other.messages) != 0 {
		t.Fatalf("friend messages=%d other messages=%d", len(friend.messages), len(other.messages))
	}
	var message pokerproto.ServerMessage
	if err := proto.Unmarshal(friend.messages[0], &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "social_presence_changed" || message.SocialEvent == nil || message.SocialEvent.Presence == nil ||
		message.SocialEvent.Presence.PlayerId != "player-1" || message.SocialEvent.Presence.Status != "in_table" || message.SocialEvent.RoomId != "" {
		t.Fatalf("unexpected presence frame: %+v", &message)
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
