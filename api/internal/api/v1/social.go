package v1

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/social"
)

const (
	socialBasePath              = "/social"
	socialFriendsPath           = "/friends"
	socialRequestsPath          = "/friend-requests"
	socialRelationshipsPath     = "/relationships"
	socialMutesPath             = "/mutes"
	socialBlocksPath            = "/blocks"
	socialBlockedPath           = "/blocked"
	socialLookupPath            = "/lookup"
	idempotencyKeyHeader        = "Idempotency-Key"
	maxSocialIdempotencyKeySize = 128
	maxSocialPlayerIDSize       = 128
	publicRelationshipNone      = social.Relationship("none")
)

type SocialLimiters struct {
	MutationPlayer *RateLimiter
	MutationIP     *RateLimiter
	RequestPlayer  *RateLimiter
	RequestIP      *RateLimiter
}

type socialHandlers struct {
	svc           *social.Service
	players       *player.Service
	avatarBaseURL string
	graphEnabled  bool
}

type socialPlayerResponse struct {
	PlayerID     string              `json:"player_id"`
	Name         string              `json:"name,omitempty"`
	AvatarURL    string              `json:"avatar_url,omitempty"`
	FriendCode   string              `json:"friend_code,omitempty"`
	Relationship social.Relationship `json:"relationship"`
	Muted        bool                `json:"muted"`
	Blocked      bool                `json:"blocked"`
}

type friendRequestBody struct {
	TargetPlayerID string `json:"target_player_id"`
	FriendCode     string `json:"friend_code"`
}

func RegisterSocial(router fiber.Router, auth fiber.Handler, svc *social.Service, players *player.Service, cfg *config.Config, limiters SocialLimiters) {
	h := &socialHandlers{svc: svc, players: players, avatarBaseURL: cfg.AvatarBaseURL, graphEnabled: cfg.SocialGraphEnabled}
	g := router.Group(socialBasePath, auth, firstPartyOnly)

	g.Get(socialFriendsPath, h.listFriends)
	g.Get(socialRequestsPath, h.listRequests)
	g.Get(socialBlockedPath, h.listBlocked)
	g.Get(socialLookupPath+"/:friendCode", h.lookup)
	g.Get(socialRelationshipsPath+"/:playerId", h.relationship)

	mutationPlayer := rateLimit(limiters.MutationPlayer, playerKey("social:mutation"))
	mutationIP := rateLimit(limiters.MutationIP, ipKey("social:mutation"))
	requestPlayer := rateLimit(limiters.RequestPlayer, playerKey("social:friend-request"))
	requestIP := rateLimit(limiters.RequestIP, ipKey("social:friend-request"))
	g.Post(socialRequestsPath, mutationPlayer, mutationIP, requestPlayer, requestIP, h.requestFriend)
	g.Post(socialRequestsPath+"/:playerId/accept", mutationPlayer, mutationIP, h.acceptFriend)
	g.Post(socialRequestsPath+"/:playerId/decline", mutationPlayer, mutationIP, h.declineFriend)
	g.Delete(socialRequestsPath+"/:playerId", mutationPlayer, mutationIP, h.cancelRequest)
	g.Delete(socialFriendsPath+"/:playerId", mutationPlayer, mutationIP, h.removeFriend)
	g.Put(socialMutesPath+"/:playerId", mutationPlayer, mutationIP, h.mute)
	g.Delete(socialMutesPath+"/:playerId", mutationPlayer, mutationIP, h.unmute)
	g.Put(socialBlocksPath+"/:playerId", mutationPlayer, mutationIP, h.block)
	g.Delete(socialBlocksPath+"/:playerId", mutationPlayer, mutationIP, h.unblock)
}

func (h *socialHandlers) listFriends(c fiber.Ctx) error {
	cursor := c.Query("cursor")
	edges, lastKey, err := h.svc.ListFriends(c.Context(), actorID(c), limitParam(c), decodeCursor(cursor))
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	players, err := h.hydrate(c, edges, false)
	if err != nil {
		return problem.InternalServer("failed to hydrate friends", c, err).Send(c)
	}
	return sendPage(c, players, lastKey, cursor)
}

func (h *socialHandlers) listRequests(c fiber.Ctx) error {
	direction := social.Relationship(c.Query("direction", string(social.RelationshipIncoming)))
	if direction != social.RelationshipIncoming && direction != social.RelationshipOutgoing {
		return problem.BadRequest("direction must be incoming or outgoing").Send(c)
	}
	cursor := c.Query("cursor")
	edges, lastKey, err := h.svc.ListRequests(c.Context(), actorID(c), direction, limitParam(c), decodeCursor(cursor))
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	players, err := h.hydrate(c, edges, false)
	if err != nil {
		return problem.InternalServer("failed to hydrate friend requests", c, err).Send(c)
	}
	return sendPage(c, players, lastKey, cursor)
}

func (h *socialHandlers) listBlocked(c fiber.Ctx) error {
	cursor := c.Query("cursor")
	edges, lastKey, err := h.svc.ListBlocked(c.Context(), actorID(c), limitParam(c), decodeCursor(cursor))
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	players, err := h.hydrate(c, edges, false)
	if err != nil {
		return problem.InternalServer("failed to hydrate blocked players", c, err).Send(c)
	}
	return sendPage(c, players, lastKey, cursor)
}

func (h *socialHandlers) lookup(c fiber.Ctx) error {
	if !h.graphEnabled {
		return socialProblem(social.ErrFeatureDisabled, c).Send(c)
	}
	profile, err := h.players.LookupByFriendCode(c.Context(), c.Params("friendCode"))
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	if profile == nil {
		return problem.NotFound("friend code not found").Send(c)
	}
	edge, err := h.svc.Relationship(c.Context(), actorID(c), profile.UserID)
	if errors.Is(err, social.ErrSelfRelationship) {
		edge = nil
	} else if err != nil {
		return socialProblem(err, c).Send(c)
	}
	response := h.response(profile, edge, true)
	return c.JSON(response)
}

func (h *socialHandlers) relationship(c fiber.Ctx) error {
	targetID, err := pathPlayerID(c)
	if err != nil {
		return problem.BadRequest("player id is invalid").Send(c)
	}
	edge, err := h.svc.Relationship(c.Context(), actorID(c), targetID)
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	profile, err := h.players.Get(c.Context(), targetID)
	if err != nil {
		return problem.InternalServer("failed to load player relationship", c, err).Send(c)
	}
	if profile == nil {
		return problem.NotFound("player not found").Send(c)
	}
	return c.JSON(h.response(profile, edge, false))
}

func (h *socialHandlers) requestFriend(c fiber.Ctx) error {
	key, p := socialIdempotencyKey(c)
	if p != nil {
		return p.Send(c)
	}
	var body friendRequestBody
	if err := c.Bind().Body(&body); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if (strings.TrimSpace(body.TargetPlayerID) == "") == (strings.TrimSpace(body.FriendCode) == "") {
		return problem.BadRequest("provide exactly one of target_player_id or friend_code").Send(c)
	}
	var target *player.PlayerProfile
	var err error
	if body.FriendCode != "" {
		target, err = h.players.LookupByFriendCode(c.Context(), body.FriendCode)
	} else {
		target, err = h.players.Get(c.Context(), strings.TrimSpace(body.TargetPlayerID))
	}
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	if target == nil {
		return problem.NotFound("player not found").Send(c)
	}
	edge, err := h.svc.Request(c.Context(), actorID(c), target.UserID, key)
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	return c.Status(fiber.StatusCreated).JSON(h.response(target, edge, false))
}

func (h *socialHandlers) acceptFriend(c fiber.Ctx) error {
	return h.mutatePath(c, h.svc.Accept, true)
}
func (h *socialHandlers) declineFriend(c fiber.Ctx) error {
	return h.mutatePath(c, h.svc.Decline, false)
}
func (h *socialHandlers) cancelRequest(c fiber.Ctx) error {
	return h.mutatePath(c, h.svc.Cancel, false)
}
func (h *socialHandlers) removeFriend(c fiber.Ctx) error {
	return h.mutatePath(c, h.svc.RemoveFriend, false)
}
func (h *socialHandlers) mute(c fiber.Ctx) error {
	return h.mutatePath(c, h.svc.Mute, true)
}
func (h *socialHandlers) unmute(c fiber.Ctx) error {
	return h.mutatePath(c, h.svc.Unmute, false)
}
func (h *socialHandlers) block(c fiber.Ctx) error {
	return h.mutatePath(c, h.svc.Block, true)
}
func (h *socialHandlers) unblock(c fiber.Ctx) error {
	return h.mutatePath(c, h.svc.Unblock, false)
}

type socialMutation func(context.Context, string, string, string) (*social.Edge, error)

func (h *socialHandlers) mutatePath(c fiber.Ctx, mutation socialMutation, includeBody bool) error {
	key, p := socialIdempotencyKey(c)
	if p != nil {
		return p.Send(c)
	}
	targetID, err := pathPlayerID(c)
	if err != nil {
		return problem.BadRequest("player id is invalid").Send(c)
	}
	edge, err := mutation(c.Context(), actorID(c), targetID, key)
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	if !includeBody {
		return c.SendStatus(fiber.StatusNoContent)
	}
	return c.JSON(edgeState(edge))
}

func (h *socialHandlers) hydrate(c fiber.Ctx, edges []social.Edge, includeFriendCode bool) ([]socialPlayerResponse, error) {
	result := make([]socialPlayerResponse, 0, len(edges))
	ids := make([]string, 0, len(edges))
	for i := range edges {
		ids = append(ids, edges[i].OtherPlayerID)
	}
	profiles, err := h.players.GetMany(c.Context(), ids)
	if err != nil {
		return nil, err
	}
	for i := range edges {
		profile, ok := profiles[edges[i].OtherPlayerID]
		if !ok {
			continue
		}
		result = append(result, h.response(&profile, &edges[i], includeFriendCode))
	}
	return result, nil
}

func (h *socialHandlers) response(profile *player.PlayerProfile, edge *social.Edge, includeFriendCode bool) socialPlayerResponse {
	state := edgeState(edge)
	state.PlayerID = profile.UserID
	state.Name = profile.Name
	state.AvatarURL = player.AvatarURL(profile, h.avatarBaseURL)
	if includeFriendCode {
		state.FriendCode = profile.FriendCode
	}
	return state
}

func edgeState(edge *social.Edge) socialPlayerResponse {
	result := socialPlayerResponse{Relationship: publicRelationshipNone}
	if edge != nil {
		result.PlayerID = edge.OtherPlayerID
		result.Relationship = edge.Relationship
		result.Muted = edge.Muted
		result.Blocked = edge.Blocked
	}
	return result
}

func actorID(c fiber.Ctx) string { return c.Locals(localsUserID).(string) }

func pathPlayerID(c fiber.Ctx) (string, error) {
	playerID, err := url.PathUnescape(c.Params("playerId"))
	playerID = strings.TrimSpace(playerID)
	if err != nil || playerID == "" || len(playerID) > maxSocialPlayerIDSize || strings.ContainsAny(playerID, "\x00\r\n") {
		return "", errors.New("invalid player id")
	}
	return playerID, nil
}

func socialIdempotencyKey(c fiber.Ctx) (string, *problem.Problem) {
	key := strings.TrimSpace(c.Get(idempotencyKeyHeader))
	if key == "" {
		return "", problem.BadRequest("Idempotency-Key is required")
	}
	if len(key) > maxSocialIdempotencyKeySize {
		return "", problem.BadRequest("Idempotency-Key must have at most 128 characters")
	}
	return key, nil
}

func socialProblem(err error, c fiber.Ctx) *problem.Problem {
	switch {
	case errors.Is(err, social.ErrFeatureDisabled):
		return problem.New(http.StatusServiceUnavailable, "/problems/social-disabled", "Social Features Disabled", "social features are temporarily unavailable")
	case errors.Is(err, social.ErrFriendLimitReached):
		return problem.New(http.StatusConflict, "/problems/friend-limit-reached", "Friend Limit Reached", "friend limit reached")
	case errors.Is(err, social.ErrRequestLimitReached):
		return problem.New(http.StatusConflict, "/problems/request-limit-reached", "Request Limit Reached", "pending friend request limit reached")
	case errors.Is(err, social.ErrRelationshipConflict), errors.Is(err, social.ErrConcurrentTransition):
		return problem.New(http.StatusConflict, "/problems/relationship-conflict", "Relationship Conflict", "the relationship cannot be changed")
	case errors.Is(err, social.ErrSelfRelationship):
		return problem.BadRequest("target must be another player")
	case errors.Is(err, player.ErrFriendCodeCollision):
		return problem.New(http.StatusServiceUnavailable, "/problems/friend-code-unavailable", "Friend Code Unavailable", "friend code is temporarily unavailable")
	case err != nil:
		return problem.InternalServer("social operation failed", c, err)
	default:
		return problem.InternalServer("social operation failed", c, errors.New("unknown social error"))
	}
}
