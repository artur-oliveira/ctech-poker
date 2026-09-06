package rest

import (
	"context"
	"net/http"
	"net/url"
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
	page, err := c.FriendsPage(ctx, "")
	return page.Data, err
}

func (c *Client) FriendsPage(ctx context.Context, cursor string) (Page[SocialPlayer], error) {
	var page Page[SocialPlayer]
	err := c.Do(ctx, http.MethodGet, withCursor("/v1.0/social/friends", cursor), nil, &page)
	return page, err
}

// FriendRequests lists pending friend requests in one direction ("incoming"
// or "outgoing"), first page (GET /v1.0/social/friend-requests).
func (c *Client) FriendRequests(ctx context.Context, direction string) ([]SocialPlayer, error) {
	page, err := c.FriendRequestsPage(ctx, direction, "")
	return page.Data, err
}

func (c *Client) FriendRequestsPage(ctx context.Context, direction, cursor string) (Page[SocialPlayer], error) {
	var page Page[SocialPlayer]
	path := "/v1.0/social/friend-requests?direction=" + url.QueryEscape(direction)
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	err := c.Do(ctx, http.MethodGet, path, nil, &page)
	return page, err
}

// Blocked lists players the caller has blocked, first page
// (GET /v1.0/social/blocked).
func (c *Client) Blocked(ctx context.Context) ([]SocialPlayer, error) {
	page, err := c.BlockedPage(ctx, "")
	return page.Data, err
}

func (c *Client) BlockedPage(ctx context.Context, cursor string) (Page[SocialPlayer], error) {
	var page Page[SocialPlayer]
	err := c.Do(ctx, http.MethodGet, withCursor("/v1.0/social/blocked", cursor), nil, &page)
	return page, err
}

// RecentPlayers lists opponents the caller has recently shared a table with,
// first page, newest first (GET /v1.0/social/recent).
func (c *Client) RecentPlayers(ctx context.Context) ([]SocialPlayer, error) {
	page, err := c.RecentPlayersPage(ctx, "")
	return page.Data, err
}

func (c *Client) RecentPlayersPage(ctx context.Context, cursor string) (Page[SocialPlayer], error) {
	var page Page[SocialPlayer]
	err := c.Do(ctx, http.MethodGet, withCursor("/v1.0/social/recent", cursor), nil, &page)
	return page, err
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
	page, err := c.InboxPage(ctx, "")
	return page.Data, err
}

func (c *Client) InboxPage(ctx context.Context, cursor string) (Page[SocialInboxEvent], error) {
	var page Page[SocialInboxEvent]
	err := c.Do(ctx, http.MethodGet, withCursor("/v1.0/social/inbox", cursor), nil, &page)
	return page, err
}

func withCursor(path, cursor string) string {
	if cursor == "" {
		return path
	}
	return path + "?cursor=" + url.QueryEscape(cursor)
}
