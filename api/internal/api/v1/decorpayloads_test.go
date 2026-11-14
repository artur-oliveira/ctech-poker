package v1

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/social"
)

var decoredProfile = player.PlayerProfile{
	Name: "Ana", ShowcasePublic: true,
	EquippedFrameID: "f1", EquippedHatID: "h1", EquippedAccessoryID: "a1", EquippedBadgeIDs: []string{"b1", "b2"},
}

func assertDecor(t *testing.T, got map[string]any) {
	t.Helper()
	want := map[string]any{"equipped_frame_id": "f1", "equipped_hat_id": "h1", "equipped_accessory_id": "a1", "equipped_badge_ids": []any{"b1", "b2"}}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Fatalf("%s = %v, want %v (payload %v)", k, got[k], v, got)
		}
	}
}

func assertNoDecor(t *testing.T, got map[string]any) {
	t.Helper()
	for _, k := range []string{"equipped_frame_id", "equipped_hat_id", "equipped_accessory_id", "equipped_badge_ids"} {
		if _, ok := got[k]; ok {
			t.Fatalf("%s leaked: %v", k, got)
		}
	}
}

func getJSON(t *testing.T, app *fiber.App, path string) (int, map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, path, nil))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestShowcaseExposesDecor(t *testing.T) {
	h := &playerHandlers{players: player.NewService(&fakePlayerStore{profile: decoredProfile}), sessions: &mockHistoryReader{}, achievements: mockAchievementReader{}}
	app := fiber.New()
	app.Get("/players/:playerId/showcase", h.showcase)
	status, body := getJSON(t, app, "/players/u1/showcase")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	assertDecor(t, body)
}

func TestPrivateShowcaseExposesNoDecor(t *testing.T) {
	get := func(public bool) (int, map[string]any) {
		profile := decoredProfile
		profile.ShowcasePublic = public
		h := &playerHandlers{players: player.NewService(&fakePlayerStore{profile: profile}), sessions: &mockHistoryReader{}, achievements: mockAchievementReader{}}
		app := fiber.New()
		app.Get("/players/:playerId/showcase", h.showcase)
		return getJSON(t, app, "/players/u1/showcase")
	}
	if status, body := get(true); status != fiber.StatusOK {
		t.Fatalf("public status = %d", status)
	} else {
		assertDecor(t, body)
	}
	status, body := get(false)
	if status != fiber.StatusNotFound {
		t.Fatalf("private status = %d, want 404", status)
	}
	assertNoDecor(t, body)
}

func TestHandOpponentsCarryDecorOnlyWhenProfileResolves(t *testing.T) {
	h := &playerHandlers{
		players: player.NewService(&fakeMultiPlayerStore{profiles: map[string]player.PlayerProfile{"known": decoredProfile}}),
		cfg:     &config.Config{AvatarBaseURL: "https://cdn.example.com"},
	}
	hands := []sessionlog.HandItem{{Opponents: []sessionlog.OpponentSummary{{PlayerID: "known"}, {PlayerID: "erased"}}}}
	if err := h.resolveOpponentProfiles(t.Context(), hands); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(hands[0].Opponents)
	var opps []map[string]any
	_ = json.Unmarshal(raw, &opps)
	assertDecor(t, opps[0])
	assertNoDecor(t, opps[1])
}

func TestSocialResponseCarriesDecor(t *testing.T) {
	h := &socialHandlers{}
	raw, _ := json.Marshal(h.response(&decoredProfile, nil, false))
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	assertDecor(t, body)
}

func TestInboxActorCarriesDecor(t *testing.T) {
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "actor"); c.Locals(localsFirstParty, true); return c.Next() }
	events := &fakeInboxEventStore{events: []social.Event{{RecipientPlayerID: "actor", EventID: "e1", Type: social.EventFriendRequest, ActorPlayerID: "ana", Status: social.EventStatusPending, CreatedAt: 1}}}
	svc := social.NewService(newAPISocialStore(), true).WithInbox(events)
	profiles := &fakeMultiProfileStore{byID: map[string]player.PlayerProfile{"ana": decoredProfile}}
	RegisterSocial(app.Group("/v1.0"), auth, svc, player.NewService(profiles), &config.Config{SocialGraphEnabled: true}, SocialLimiters{})
	status, body := getJSON(t, app, "/v1.0/social/inbox")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d", status)
	}
	assertDecor(t, body["data"].([]any)[0].(map[string]any))
}
