package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/social"
)

type apiSocialStore struct {
	mu    sync.Mutex
	edges map[string]social.Edge
}

func newAPISocialStore() *apiSocialStore    { return &apiSocialStore{edges: make(map[string]social.Edge)} }
func apiEdgeKey(owner, other string) string { return owner + "\x00" + other }

func (s *apiSocialStore) Get(_ context.Context, owner, other string) (*social.Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	edge, ok := s.edges[apiEdgeKey(owner, other)]
	if !ok {
		return nil, nil
	}
	return &edge, nil
}
func (s *apiSocialStore) List(_ context.Context, owner string, relationship social.Relationship, blockedOnly bool, _ int, _ map[string]types.AttributeValue) ([]social.Edge, map[string]types.AttributeValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []social.Edge{}
	for _, edge := range s.edges {
		if edge.OwnerPlayerID == owner && (!blockedOnly && edge.Relationship == relationship || blockedOnly && edge.Blocked) {
			result = append(result, edge)
		}
	}
	return result, nil, nil
}
func (s *apiSocialStore) Count(context.Context, string, social.Relationship) (int, error) {
	return 0, nil
}
func (s *apiSocialStore) Apply(_ context.Context, transition social.Transition) (*social.Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	actorKey := apiEdgeKey(transition.ActorPlayerID, transition.TargetPlayerID)
	targetKey := apiEdgeKey(transition.TargetPlayerID, transition.ActorPlayerID)
	var actor, target *social.Edge
	if edge, ok := s.edges[actorKey]; ok {
		copy := edge
		actor = &copy
	}
	if edge, ok := s.edges[targetKey]; ok {
		copy := edge
		target = &copy
	}
	afterActor, afterTarget, err := social.PlanTransition(actor, target, transition)
	if err != nil {
		return nil, err
	}
	if afterActor == nil {
		delete(s.edges, actorKey)
	} else {
		s.edges[actorKey] = *afterActor
	}
	if afterTarget == nil {
		delete(s.edges, targetKey)
	} else {
		s.edges[targetKey] = *afterTarget
	}
	return afterActor, nil
}

func socialTestApp(enabled bool) (*fiber.App, *apiSocialStore, string) {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, "actor")
		c.Locals(localsFirstParty, true)
		return c.Next()
	}
	store := newAPISocialStore()
	svc := social.NewService(store, enabled)
	code := player.FriendCodeForUserID("target")
	profiles := &fakePlayerStore{profile: player.PlayerProfile{Name: "Target", FriendCode: code}, lookupID: "target"}
	RegisterSocial(app.Group("/v1.0"), auth, svc, player.NewService(profiles), &config.Config{SocialGraphEnabled: enabled}, SocialLimiters{})
	return app, store, code
}

func TestSocialFriendRequestRequiresIdempotencyKeyAndCreatesMirroredRequest(t *testing.T) {
	app, store, code := socialTestApp(true)
	body, _ := json.Marshal(friendRequestBody{FriendCode: code})

	missing := httptest.NewRequest(fiber.MethodPost, "/v1.0/social/friend-requests", bytes.NewReader(body))
	missing.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(missing)
	if err != nil || response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("missing idempotency response=%v err=%v", response.StatusCode, err)
	}

	request := httptest.NewRequest(fiber.MethodPost, "/v1.0/social/friend-requests", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	request.Header.Set(idempotencyKeyHeader, "request-1")
	response, err = app.Test(request)
	if err != nil || response.StatusCode != fiber.StatusCreated {
		t.Fatalf("request response=%v err=%v", response.StatusCode, err)
	}
	actor, _ := store.Get(context.Background(), "actor", "target")
	target, _ := store.Get(context.Background(), "target", "actor")
	if actor == nil || target == nil || actor.Relationship != social.RelationshipOutgoing || target.Relationship != social.RelationshipIncoming {
		t.Fatalf("request was not mirrored: actor=%+v target=%+v", actor, target)
	}
	actorVersion, targetVersion := actor.Version, target.Version

	retry := httptest.NewRequest(fiber.MethodPost, "/v1.0/social/friend-requests", bytes.NewReader(body))
	retry.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	retry.Header.Set(idempotencyKeyHeader, "request-1")
	response, err = app.Test(retry)
	if err != nil || response.StatusCode != fiber.StatusCreated {
		t.Fatalf("idempotent retry response=%v err=%v", response.StatusCode, err)
	}
	actor, _ = store.Get(context.Background(), "actor", "target")
	target, _ = store.Get(context.Background(), "target", "actor")
	if actor.Version != actorVersion || target.Version != targetVersion {
		t.Fatalf("idempotent retry changed versions: actor=%d->%d target=%d->%d", actorVersion, actor.Version, targetVersion, target.Version)
	}
}

func TestSocialLookupIsExactAndReturnsPublicNoneRelationship(t *testing.T) {
	app, _, code := socialTestApp(true)

	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/social/lookup/"+strings.ToLower(code), nil))
	if err != nil || response.StatusCode != fiber.StatusOK {
		t.Fatalf("lookup response=%v err=%v", response.StatusCode, err)
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["friend_code"] != code || payload["relationship"] != "none" {
		t.Fatalf("unexpected lookup payload: %v", payload)
	}

	missing, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/social/lookup/Target", nil))
	if err != nil || missing.StatusCode != fiber.StatusNotFound {
		t.Fatalf("non-code lookup response=%v err=%v", missing.StatusCode, err)
	}
}

func TestSocialSafetyRemainsAvailableWhenGraphDisabled(t *testing.T) {
	app, _, _ := socialTestApp(false)
	list := httptest.NewRequest(fiber.MethodGet, "/v1.0/social/friends", nil)
	response, err := app.Test(list)
	if err != nil || response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("friends response=%v err=%v", response.StatusCode, err)
	}

	block := httptest.NewRequest(fiber.MethodPut, "/v1.0/social/blocks/target", nil)
	block.Header.Set(idempotencyKeyHeader, "block-1")
	response, err = app.Test(block)
	if err != nil || response.StatusCode != fiber.StatusOK {
		t.Fatalf("block response=%v err=%v", response.StatusCode, err)
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload["blocked"] != true || payload["muted"] != true {
		t.Fatalf("block payload=%v decodeErr=%v", payload, err)
	}
	if _, leaked := payload["blocked_by_other"]; leaked {
		t.Fatalf("private reverse-block state leaked: %v", payload)
	}
}

func TestSocialRequestReturnsGenericConflictWhenTargetBlockedActor(t *testing.T) {
	app, store, code := socialTestApp(true)
	store.edges[apiEdgeKey("target", "actor")] = social.Edge{OwnerPlayerID: "target", OtherPlayerID: "actor", Blocked: true, Muted: true, Version: 1}
	body, _ := json.Marshal(friendRequestBody{FriendCode: code})
	request := httptest.NewRequest(fiber.MethodPost, "/v1.0/social/friend-requests", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	request.Header.Set(idempotencyKeyHeader, "request-1")
	response, err := app.Test(request)
	if err != nil || response.StatusCode != fiber.StatusConflict {
		t.Fatalf("response=%v err=%v", response.StatusCode, err)
	}
	var payload map[string]any
	_ = json.NewDecoder(response.Body).Decode(&payload)
	if payload["type"] != "/problems/relationship-conflict" {
		t.Fatalf("unexpected problem payload: %v", payload)
	}
}

func TestSocialRelationshipBatchReturnsSuppressionStateForSeatList(t *testing.T) {
	app, store, _ := socialTestApp(true)
	store.edges[apiEdgeKey("actor", "muted")] = social.Edge{OwnerPlayerID: "actor", OtherPlayerID: "muted", Muted: true, Version: 1}
	store.edges[apiEdgeKey("actor", "blocked")] = social.Edge{OwnerPlayerID: "actor", OtherPlayerID: "blocked", Muted: true, Blocked: true, Version: 1}

	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/social/relationships?player_ids=muted,blocked,stranger", nil))
	if err != nil || response.StatusCode != fiber.StatusOK {
		t.Fatalf("batch response=%v err=%v", response.StatusCode, err)
	}
	var payload struct {
		Data []socialPlayerResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("expected only known edges, got %+v", payload.Data)
	}
	if payload.Data[0].PlayerID != "muted" || !payload.Data[0].Muted || payload.Data[0].Blocked {
		t.Fatalf("unexpected muted entry: %+v", payload.Data[0])
	}
	if payload.Data[1].PlayerID != "blocked" || !payload.Data[1].Blocked {
		t.Fatalf("unexpected blocked entry: %+v", payload.Data[1])
	}

	empty, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/social/relationships", nil))
	if err != nil || empty.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("empty batch response=%v err=%v", empty.StatusCode, err)
	}
	oversized, err := app.Test(httptest.NewRequest(fiber.MethodGet,
		"/v1.0/social/relationships?player_ids="+strings.TrimSuffix(strings.Repeat("p,", maxRelationshipBatch+1), ","), nil))
	if err != nil || oversized.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("oversized batch response=%v err=%v", oversized.StatusCode, err)
	}
}

func TestSocialReadsRejectDelegatedClients(t *testing.T) {
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		c.Locals(localsUserID, "actor")
		c.Locals(localsFirstParty, false)
		return c.Next()
	}
	store := newAPISocialStore()
	profiles := player.NewService(&fakePlayerStore{})
	RegisterSocial(app.Group("/v1.0"), auth, social.NewService(store, true), profiles, &config.Config{SocialGraphEnabled: true}, SocialLimiters{})
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/social/friends", nil))
	if err != nil || response.StatusCode != fiber.StatusForbidden {
		t.Fatalf("response=%v err=%v", response.StatusCode, err)
	}
}
