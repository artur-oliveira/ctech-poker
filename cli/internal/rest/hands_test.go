package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandsDecodesPageAndCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/players/me/hands" || r.URL.Query().Get("mode") != "sandbox" || r.URL.Query().Get("cursor") != "page/2" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":     []map[string]any{{"hand_id": "h-1", "table_id": "t-1", "outcome": "won", "net_change": 250}},
			"has_next": true, "next_cursor": "next-token",
		})
	}))
	defer srv.Close()

	page, err := New(srv.URL, tokenFunc("t"), srv.Client()).Hands(context.Background(), "page/2")
	if err != nil || len(page.Data) != 1 || page.Data[0].HandID != "h-1" || page.NextCursor != "next-token" {
		t.Fatalf("got page=%+v err=%v", page, err)
	}
}

func TestHandAndHistoryEscapeIDsAndDecodeTimeline(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.EscapedPath() {
		case "/v1.0/players/me/hand/h%2F1":
			_ = json.NewEncoder(w).Encode(map[string]any{"hand_id": "h/1", "table_id": "t/1", "hole_cards": []string{"As", "Kd"}})
		case "/v1.0/tables/t%2F1/hands/h%2F1/history":
			_ = json.NewEncoder(w).Encode(map[string]any{"hand_id": "h/1", "actions": []map[string]any{{"seq": 1, "player_id": "p-1", "action": "raise", "amount": 40}}})
		default:
			t.Fatalf("unexpected path %q", r.URL.EscapedPath())
		}
	}))
	defer srv.Close()

	c := New(srv.URL, tokenFunc("t"), srv.Client())
	hand, err := c.Hand(context.Background(), "h/1")
	if err != nil || hand.HandID != "h/1" || len(hand.HoleCards) != 2 {
		t.Fatalf("hand=%+v err=%v", hand, err)
	}
	history, err := c.HandHistory(context.Background(), "t/1", "h/1")
	if err != nil || len(history.Actions) != 1 || history.Actions[0].Action != "raise" || requests != 2 {
		t.Fatalf("history=%+v requests=%d err=%v", history, requests, err)
	}
}
