package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFriendsDecodesPresence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/social/friends" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"player_id": "caio", "name": "Caio", "presence": "online"}},
		})
	}))
	defer srv.Close()

	friends, err := New(srv.URL, tokenFunc("t"), srv.Client()).Friends(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) != 1 || friends[0].Name != "Caio" || friends[0].Presence != "online" {
		t.Fatalf("got %+v", friends)
	}
}

func TestFriendRequestsSendsDirection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/social/friend-requests" || r.URL.Query().Get("direction") != "outgoing" {
			t.Errorf("unexpected request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"player_id": "duda"}}})
	}))
	defer srv.Close()

	reqs, err := New(srv.URL, tokenFunc("t"), srv.Client()).FriendRequests(context.Background(), "outgoing")
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 || reqs[0].PlayerID != "duda" {
		t.Fatalf("got %+v", reqs)
	}
}

func TestBlockedDecodesEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/social/blocked" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"player_id": "edu", "blocked": true}}})
	}))
	defer srv.Close()

	blocked, err := New(srv.URL, tokenFunc("t"), srv.Client()).Blocked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || !blocked[0].Blocked {
		t.Fatalf("got %+v", blocked)
	}
}

func TestRecentPlayersDecodesHandsTogether(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/social/recent" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"player_id": "caio", "hands_together": 42, "last_played_at": 1000}},
		})
	}))
	defer srv.Close()

	recent, err := New(srv.URL, tokenFunc("t"), srv.Client()).RecentPlayers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].HandsTogether != 42 {
		t.Fatalf("got %+v", recent)
	}
}

func TestInboxDecodesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/social/inbox" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"event_id": "e1", "type": "friend_request", "actor_id": "caio", "actor_name": "Caio", "unread": true}},
		})
	}))
	defer srv.Close()

	events, err := New(srv.URL, tokenFunc("t"), srv.Client()).Inbox(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "friend_request" || !events[0].Unread {
		t.Fatalf("got %+v", events)
	}
}

func TestSocialPagesSendCursorAndPreserveEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/social/friends" || r.URL.Query().Get("cursor") != "page/2" {
			t.Errorf("unexpected request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":     []map[string]any{{"player_id": "bia", "name": "Bia"}},
			"has_next": true, "next_cursor": "page-3", "has_previous": true,
		})
	}))
	defer srv.Close()

	page, err := New(srv.URL, tokenFunc("t"), srv.Client()).FriendsPage(context.Background(), "page/2")
	if err != nil || len(page.Data) != 1 || !page.HasNext || page.NextCursor != "page-3" || !page.HasPrevious {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}
