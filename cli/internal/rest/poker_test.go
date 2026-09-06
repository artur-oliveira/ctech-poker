package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStakesDecodesTheStakesArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/rooms/stakes" || r.URL.Query().Get("currency_mode") != "sandbox" {
			t.Errorf("unexpected request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"currency_mode": "sandbox", "unit": "virtual_chip",
			"stakes": []map[string]int64{{"small_blind": 10, "big_blind": 20}},
		})
	}))
	defer srv.Close()

	stakes, err := New(srv.URL, tokenFunc("t"), srv.Client()).Stakes(context.Background(), "sandbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(stakes) != 1 || stakes[0].BigBlind != 20 {
		t.Fatalf("got %+v", stakes)
	}
}

func TestJoinOrCreateSendsBucketAndIdemKey(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]any{"room_id": "room-1", "created": true})
	}))
	defer srv.Close()

	resp, err := New(srv.URL, tokenFunc("t"), srv.Client()).JoinOrCreate(context.Background(), JoinOrCreateReq{
		SmallBlind: 10, BigBlind: 20, MaxSeats: 6, Amount: 2000, IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RoomID != "room-1" || !resp.Created {
		t.Fatalf("got %+v", resp)
	}
	if gotBody["idem_key"] != "idem-1" {
		t.Fatalf("idempotency key must be sent as idem_key: %+v", gotBody)
	}
}

func TestRoomDecodesRoomByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/rooms/room-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"room_id": "room-1", "small_blind": 10, "big_blind": 20, "max_seats": 6})
	}))
	defer srv.Close()

	room, err := New(srv.URL, tokenFunc("t"), srv.Client()).Room(context.Background(), "room-1")
	if err != nil {
		t.Fatal(err)
	}
	if room.ID != "room-1" || room.MaxSeats != 6 {
		t.Fatalf("got %+v", room)
	}
}

func TestJoinRoomAndLeaveRoomHitExpectedPaths(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(srv.URL, tokenFunc("t"), srv.Client())
	if err := c.JoinRoom(context.Background(), "room-1", JoinReq{Amount: 2000}); err != nil {
		t.Fatal(err)
	}
	if err := c.LeaveRoom(context.Background(), "room-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /v1.0/rooms/room-1/join", "POST /v1.0/rooms/room-1/leave"}
	if len(paths) != 2 || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("got %v", paths)
	}
}

func TestMeDecodesProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"user_id": "u-1", "name": "Ana", "wallet_mode": "sandbox", "game_balance": 0, "sandbox_balance": 5000,
		})
	}))
	defer srv.Close()

	p, err := New(srv.URL, tokenFunc("t"), srv.Client()).Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.UserID != "u-1" || p.Name != "Ana" || p.SandboxBalance != 5000 {
		t.Fatalf("got %+v", p)
	}
}

func TestAchievementsDecodesSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("currency_mode") != "sandbox" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"mode":   "sandbox",
			"totals": map[string]int{"revealed": 10, "unlocked": 3, "completed": 1, "stars": 7, "max_stars": 40},
			"achievements": []map[string]any{
				{"key": "first_win", "metric": "hands_won", "progress": 1, "stars": 1, "unlocked": true, "completed": true, "max_target": 1},
			},
		})
	}))
	defer srv.Close()

	s, err := New(srv.URL, tokenFunc("t"), srv.Client()).Achievements(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Totals.Unlocked != 3 || len(s.Achievements) != 1 || s.Achievements[0].Key != "first_win" {
		t.Fatalf("got %+v", s)
	}
}

func TestCurrentSessionReturnsFirstPageEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"table_id": "t-1", "buyin_amount": 2000, "net_pnl": 500, "ended_at": 0}},
		})
	}))
	defer srv.Close()

	s, err := New(srv.URL, tokenFunc("t"), srv.Client()).CurrentSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.TableID != "t-1" || s.NetPnL != 500 {
		t.Fatalf("got %+v", s)
	}
}

func TestCurrentSessionEmptyPageReturnsZeroValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
	}))
	defer srv.Close()

	s, err := New(srv.URL, tokenFunc("t"), srv.Client()).CurrentSession(context.Background())
	if err != nil || s.TableID != "" {
		t.Fatalf("got %+v err=%v", s, err)
	}
}

func TestReactionCatalogDecodesOwnedEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/wallet/reaction-purchase/catalog" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gg", "owned": true}, {"id": "premium_confetti", "premium": true, "owned": false}},
		})
	}))
	defer srv.Close()

	rs, err := New(srv.URL, tokenFunc("t"), srv.Client()).ReactionCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].ID != "gg" || !rs[0].Owned || !rs[1].Premium || rs[1].Owned {
		t.Fatalf("got %+v", rs)
	}
}
