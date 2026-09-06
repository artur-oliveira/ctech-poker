package rest

import (
	"context"
	"net/http"
)

// SocialPlayer is one row in a friends/friend-requests/blocked/recent list
// (api/internal/api/v1.socialPlayerResponse, mirrored). Presence is one of
// "online", "in_table", "offline", or "" when not resolved for this list.
type SocialPlayer struct {
	PlayerID      string `json:"player_id"`
	Name          string `json:"name,omitempty"`
	FriendCode    string `json:"friend_code,omitempty"`
	Relationship  string `json:"relationship"`
	Muted         bool   `json:"muted"`
	Blocked       bool   `json:"blocked"`
	Presence      string `json:"presence,omitempty"`
	RoomID        string `json:"room_id,omitempty"` // set only for a friend in a joinable public room
	LastPlayedAt  int64  `json:"last_played_at,omitempty"`
	HandsTogether int64  `json:"hands_together,omitempty"`
}

// Friends lists the caller's friends, first page, presence included
// (GET /v1.0/social/friends).
func (c *Client) Friends(ctx context.Context) ([]SocialPlayer, error) {
	var page Page[SocialPlayer]
	if err := c.Do(ctx, http.MethodGet, "/v1.0/social/friends", nil, &page); err != nil {
		return nil, err
	}
	return page.Data, nil
}

// FriendRequests lists pending friend requests in one direction ("incoming"
// or "outgoing"), first page (GET /v1.0/social/friend-requests).
func (c *Client) FriendRequests(ctx context.Context, direction string) ([]SocialPlayer, error) {
	var page Page[SocialPlayer]
	if err := c.Do(ctx, http.MethodGet, "/v1.0/social/friend-requests?direction="+direction, nil, &page); err != nil {
		return nil, err
	}
	return page.Data, nil
}

// Blocked lists players the caller has blocked, first page
// (GET /v1.0/social/blocked).
func (c *Client) Blocked(ctx context.Context) ([]SocialPlayer, error) {
	var page Page[SocialPlayer]
	if err := c.Do(ctx, http.MethodGet, "/v1.0/social/blocked", nil, &page); err != nil {
		return nil, err
	}
	return page.Data, nil
}

// RecentPlayers lists opponents the caller has recently shared a table with,
// first page, newest first (GET /v1.0/social/recent).
func (c *Client) RecentPlayers(ctx context.Context) ([]SocialPlayer, error) {
	var page Page[SocialPlayer]
	if err := c.Do(ctx, http.MethodGet, "/v1.0/social/recent", nil, &page); err != nil {
		return nil, err
	}
	return page.Data, nil
}

// SocialInboxEvent is one row in the social inbox — a friend request,
// acceptance, or table invite (api/internal/social.Event plus the actor's
// display name, mirrored).
type SocialInboxEvent struct {
	EventID       string `json:"event_id"`
	Type          string `json:"type"` // "friend_request" | "friend_accepted" | "table_invite"
	Status        string `json:"status"`
	ActorPlayerID string `json:"actor_id"`
	ActorName     string `json:"actor_name,omitempty"`
	RoomID        string `json:"room_id,omitempty"`
	Unread        bool   `json:"unread"`
	CreatedAt     int64  `json:"created_at"`
}

// Inbox lists the caller's social inbox, first page, newest first
// (GET /v1.0/social/inbox).
func (c *Client) Inbox(ctx context.Context) ([]SocialInboxEvent, error) {
	var page Page[SocialInboxEvent]
	if err := c.Do(ctx, http.MethodGet, "/v1.0/social/inbox", nil, &page); err != nil {
		return nil, err
	}
	return page.Data, nil
}
