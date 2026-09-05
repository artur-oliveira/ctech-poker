package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/recentplayers"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/social"
)

// fakeRecentStore backs recentplayers.Service with a fixed page, regardless
// of viewer/cursor — the rematch handler only ever asks for one bounded page.
type fakeRecentStore struct{ players []recentplayers.Player }

func (f *fakeRecentStore) RecordHand(context.Context, recentplayers.HandCompletion) error { return nil }
func (f *fakeRecentStore) List(context.Context, string, map[string]types.AttributeValue, int) (recentplayers.Page, error) {
	return recentplayers.Page{Players: f.players}, nil
}

type fakeOpenSessionStore struct{ session *sessionlog.SessionItem }

func (f *fakeOpenSessionStore) FindOpenSession(context.Context, string, string) (*sessionlog.SessionItem, error) {
	return f.session, nil
}

func rematchTestApp(t *testing.T, recent *fakeRecentStore, room *roomstore.Room, session *sessionlog.SessionItem, friendEdges map[string]social.Edge) *fiber.App {
	t.Helper()
	edgeStore := newAPISocialStore()
	for key, edge := range friendEdges {
		edgeStore.edges[key] = edge
	}
	events := &fakeInboxEventStore{}
	svc := social.NewService(edgeStore, true).WithInbox(events).WithInvites(fakeRoomLookup{room: room}, &fakeOpenSessionStore{session: session})
	recentSvc := recentplayers.NewService(recent, nil, nil)
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, "viewer")
		c.Locals(localsFirstParty, true)
		return c.Next()
	}
	RegisterSocial(app.Group("/v1.0"), auth, svc, player.NewService(&fakeMultiProfileStore{byID: map[string]player.PlayerProfile{}}),
		&config.Config{SocialGraphEnabled: true}, SocialLimiters{}, recentSvc, fakeRoomLookup{room: room})
	return app
}

func TestRematchInviteReusesSendTableInviteWithRecentContext(t *testing.T) {
	recent := &fakeRecentStore{players: []recentplayers.Player{
		{ViewerPlayerID: "viewer", OpponentPlayerID: "opponent", LastTableID: "room-1", HandsTogether: 5, LastPlayedAt: 1000},
	}}
	room := &roomstore.Room{ID: "room-1", Status: "waiting", MaxSeats: 6, SeatsTaken: 1}
	session := &sessionlog.SessionItem{TableID: "room-1"}
	edges := map[string]social.Edge{
		apiEdgeKey("viewer", "opponent"): {OwnerPlayerID: "viewer", OtherPlayerID: "opponent", Relationship: social.RelationshipFriend},
		apiEdgeKey("opponent", "viewer"): {OwnerPlayerID: "opponent", OtherPlayerID: "viewer", Relationship: social.RelationshipFriend},
	}
	app := rematchTestApp(t, recent, room, session, edges)

	req := httptest.NewRequest(fiber.MethodPost, "/v1.0/social/recent/opponent/rematch", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(idempotencyKeyHeader, "idem-1")
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d err=%v body=%s", resp.StatusCode, err, raw)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Event struct {
			RoomID string `json:"room_id"`
		} `json:"event"`
		HandsTogether int64 `json:"hands_together"`
		LastPlayedAt  int64 `json:"last_played_at"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, raw)
	}
	if body.Event.RoomID != "room-1" || body.HandsTogether != 5 || body.LastPlayedAt != 1000 {
		t.Fatalf("unexpected body: %s", raw)
	}
}

func TestRematchInviteRejectsOpponentNotInRecentList(t *testing.T) {
	recent := &fakeRecentStore{} // empty: covers both "never played" and "blocked" (Service.List already filters blocked)
	app := rematchTestApp(t, recent, nil, nil, nil)
	req := httptest.NewRequest(fiber.MethodPost, "/v1.0/social/recent/stranger/rematch", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(idempotencyKeyHeader, "idem-2")
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
}

func TestRematchInviteWithArchivedLastTableSignalsNewTableFlowNotRawError(t *testing.T) {
	recent := &fakeRecentStore{players: []recentplayers.Player{
		{ViewerPlayerID: "viewer", OpponentPlayerID: "opponent", LastTableID: "gone-room"},
	}}
	// room is nil: the previous table was deleted/archived by cmd/tablecleanup.
	app := rematchTestApp(t, recent, nil, nil, nil)
	req := httptest.NewRequest(fiber.MethodPost, "/v1.0/social/recent/opponent/rematch", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(idempotencyKeyHeader, "idem-3")
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, raw)
	}
	if body.Type != "/problems/rematch-table-unavailable" {
		t.Fatalf("expected a structured new-table-flow signal, got %s", raw)
	}
}
