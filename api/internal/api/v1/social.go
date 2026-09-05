package v1

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/presence"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/recentplayers"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
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
	socialInboxPath             = "/inbox"
	socialSummaryPath           = "/summary"
	socialTableInvitesPath      = "/table-invites"
	socialRecentPath            = "/recent"
	idempotencyKeyHeader        = "Idempotency-Key"
	maxSocialIdempotencyKeySize = 128
	maxSocialPlayerIDSize       = 128
	// One table holds at most nine seats; the batch is capped a little above
	// that so a seat list is always one request instead of nine.
	maxRelationshipBatch   = 25
	publicRelationshipNone = social.Relationship("none")
)

type SocialLimiters struct {
	MutationPlayer  *RateLimiter
	MutationIP      *RateLimiter
	RequestPlayer   *RateLimiter
	RequestIP       *RateLimiter
	InviteSender    *RateLimiter
	InviteRecipient *RateLimiter
}

// roomLookup is the slice of roomstore.Store the friends list needs to decide
// whether a friend's table may be published as joinable.
type roomLookup interface {
	Get(ctx context.Context, roomID string) (*roomstore.Room, error)
}

type socialHandlers struct {
	svc                    *social.Service
	presence               *presence.Service
	rooms                  roomLookup
	recent                 *recentplayers.Service
	players                *player.Service
	avatarBaseURL          string
	graphEnabled           bool
	inviteRecipientLimiter *RateLimiter
}

type socialPlayerResponse struct {
	PlayerID     string              `json:"player_id"`
	Name         string              `json:"name,omitempty"`
	AvatarURL    string              `json:"avatar_url,omitempty"`
	FriendCode   string              `json:"friend_code,omitempty"`
	Relationship social.Relationship `json:"relationship"`
	Muted        bool                `json:"muted"`
	Blocked      bool                `json:"blocked"`
	Presence     *presence.Status    `json:"presence,omitempty"`
	// RoomID is present only for a friend who opted in (player.TablePublic)
	// and is sitting in a joinable PUBLIC room — see joinableRoomIDs. Every
	// other case omits it, including a private table.
	RoomID        string `json:"room_id,omitempty"`
	LastPlayedAt  int64  `json:"last_played_at,omitempty"`
	HandsTogether int64  `json:"hands_together,omitempty"`
}

type friendRequestBody struct {
	TargetPlayerID string `json:"target_player_id"`
	FriendCode     string `json:"friend_code"`
}

type tableInviteBody struct {
	TargetPlayerID string `json:"target_player_id"`
	RoomID         string `json:"room_id"`
}

type inboxReadBody struct {
	EventIDs []string `json:"event_ids"`
}

// socialInboxEventResponse is the wire shape of an inbox row: the stored
// event plus the actor's display name/avatar resolved at read time. The
// event itself only ever persists actor_id (see social.Event) — resolving a
// batch of ids per feed render, rather than denormalizing a name/avatar copy
// onto the event at write time, keeps the inbox immune to the name-drift bug
// #64 fixed elsewhere (a stale copy surviving a later profile edit). A
// resolve miss (e.g. a deleted profile) leaves both fields empty; the client
// falls back to its own placeholder rather than the server guessing one.
type socialInboxEventResponse struct {
	social.Event
	ActorName      string `json:"actor_name,omitempty"`
	ActorAvatarURL string `json:"actor_avatar_url,omitempty"`
}

func RegisterSocial(router fiber.Router, auth fiber.Handler, svc *social.Service, players *player.Service, cfg *config.Config, limiters SocialLimiters, extras ...any) {
	var presenceSvc *presence.Service
	var recentSvc *recentplayers.Service
	var roomsStore roomLookup
	for _, extra := range extras {
		switch value := extra.(type) {
		case *presence.Service:
			presenceSvc = value
		case *recentplayers.Service:
			recentSvc = value
		// Last: roomLookup is an interface, so it would otherwise swallow any
		// future extra that happens to carry a matching Get method.
		case roomLookup:
			roomsStore = value
		}
	}
	h := &socialHandlers{svc: svc, presence: presenceSvc, rooms: roomsStore, recent: recentSvc, players: players, avatarBaseURL: cfg.AvatarBaseURL, graphEnabled: cfg.SocialGraphEnabled, inviteRecipientLimiter: limiters.InviteRecipient}
	g := router.Group(socialBasePath, auth, firstPartyOnly)

	g.Get(socialFriendsPath, h.listFriends)
	g.Get(socialRequestsPath, h.listRequests)
	g.Get(socialBlockedPath, h.listBlocked)
	g.Get(socialLookupPath+"/:friendCode", h.lookup)
	g.Get(socialRelationshipsPath, h.relationshipBatch)
	g.Get(socialRelationshipsPath+"/:playerId", h.relationship)
	g.Get(socialSummaryPath, h.summary)
	g.Get(socialInboxPath, h.listInbox)
	g.Get(socialRecentPath, h.listRecent)

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
	inviteSender := rateLimit(limiters.InviteSender, playerKey("social:table-invite"))
	g.Post(socialInboxPath+"/read", mutationPlayer, mutationIP, h.markInboxRead)
	g.Post(socialTableInvitesPath, mutationPlayer, mutationIP, inviteSender, h.sendTableInvite)
	g.Post(socialTableInvitesPath+"/:eventId/accept", mutationPlayer, mutationIP, h.acceptTableInvite)
	g.Post(socialTableInvitesPath+"/:eventId/decline", mutationPlayer, mutationIP, h.declineTableInvite)
}

// joinableRoomIDs resolves which presences may be published as a joinable
// room. All five gates must hold: the friend opted in, presence says in_table
// with a known room, and that room is public, open and not full. Any failure —
// including a room read error — drops the id.
func (h *socialHandlers) joinableRoomIDs(ctx context.Context, presences map[string]presence.PlayerPresence,
	profiles map[string]player.PlayerProfile) map[string]string {
	if h.rooms == nil {
		return nil
	}
	wanted := make(map[string]string)
	rooms := make(map[string]bool)
	for playerID, entry := range presences {
		profile, ok := profiles[playerID]
		if !ok || !profile.TablePublic || entry.Status != presence.StatusInTable || entry.RoomID == "" {
			continue
		}
		wanted[playerID] = entry.RoomID
		rooms[entry.RoomID] = true
	}
	// One read per distinct room, not per friend: a full table of friends
	// resolves to a single lookup.
	joinable := make(map[string]bool, len(rooms))
	for roomID := range rooms {
		room, err := h.rooms.Get(ctx, roomID)
		if err != nil {
			slog.Warn("social: room lookup for friend presence failed", "room", roomID, "err", err)
			continue
		}
		joinable[roomID] = room.Joinable()
	}
	result := make(map[string]string, len(wanted))
	for playerID, roomID := range wanted {
		if joinable[roomID] {
			result[playerID] = roomID
		}
	}
	return result
}

func (h *socialHandlers) summary(c fiber.Ctx) error {
	count, err := h.svc.UnreadCount(c.Context(), actorID(c))
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	return c.JSON(fiber.Map{"unread_count": count})
}

func (h *socialHandlers) listInbox(c fiber.Ctx) error {
	cursor := c.Query("cursor")
	events, lastKey, err := h.svc.ListInbox(c.Context(), actorID(c), limitParam(c), decodeCursor(cursor))
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	result, err := h.hydrateInboxActors(c.Context(), events)
	if err != nil {
		return problem.InternalServer("failed to hydrate inbox", c, err).Send(c)
	}
	return sendPage(c, result, lastKey, cursor)
}

// hydrateInboxActors resolves every distinct actor_id on this page in a
// single players.GetMany batch call — one resolve request per feed render,
// not one per row — so a friend_request from a stranger, a table_invite, or
// any event whose actor is absent from the friends/requests lists the
// frontend used to rely on still gets a real name and avatar.
func (h *socialHandlers) hydrateInboxActors(ctx context.Context, events []social.Event) ([]socialInboxEventResponse, error) {
	ids := make([]string, 0, len(events))
	seen := make(map[string]bool, len(events))
	for i := range events {
		if actorID := events[i].ActorPlayerID; actorID != "" && !seen[actorID] {
			seen[actorID] = true
			ids = append(ids, actorID)
		}
	}
	profiles, err := h.players.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]socialInboxEventResponse, 0, len(events))
	for i := range events {
		response := socialInboxEventResponse{Event: events[i]}
		if profile, ok := profiles[events[i].ActorPlayerID]; ok {
			response.ActorName = profile.Name
			response.ActorAvatarURL = player.AvatarURL(&profile, h.avatarBaseURL)
		}
		result = append(result, response)
	}
	return result, nil
}

func (h *socialHandlers) markInboxRead(c fiber.Ctx) error {
	if _, p := socialIdempotencyKey(c); p != nil {
		return p.Send(c)
	}
	var body inboxReadBody
	if err := c.Bind().Body(&body); err != nil || len(body.EventIDs) == 0 || len(body.EventIDs) > 100 {
		return problem.BadRequest("event_ids must contain between 1 and 100 ids").Send(c)
	}
	for _, id := range body.EventIDs {
		if strings.TrimSpace(id) == "" || len(id) > maxSocialIdempotencyKeySize {
			return problem.BadRequest("event id is invalid").Send(c)
		}
	}
	if err := h.svc.MarkInboxRead(c.Context(), actorID(c), body.EventIDs); err != nil {
		return socialProblem(err, c).Send(c)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *socialHandlers) sendTableInvite(c fiber.Ctx) error {
	key, p := socialIdempotencyKey(c)
	if p != nil {
		return p.Send(c)
	}
	var body tableInviteBody
	if err := c.Bind().Body(&body); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	body.TargetPlayerID, body.RoomID = strings.TrimSpace(body.TargetPlayerID), strings.TrimSpace(body.RoomID)
	if body.TargetPlayerID == "" || body.RoomID == "" || len(body.TargetPlayerID) > maxSocialPlayerIDSize || len(body.RoomID) > maxSocialPlayerIDSize {
		return problem.BadRequest("target_player_id and room_id are required").Send(c)
	}
	if h.inviteRecipientLimiter != nil {
		allowed, err := h.inviteRecipientLimiter.Allow(c.Context(), "rl:social:table-invite:recipient:"+body.TargetPlayerID)
		if err != nil {
			slog.Warn("invite recipient rate limiter failed open", "err", err)
		} else if !allowed {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "rate_limit_exceeded", "message": "too many invitations for this player"})
		}
	}
	event, err := h.svc.SendTableInvite(c.Context(), actorID(c), body.TargetPlayerID, body.RoomID, key)
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	return c.Status(fiber.StatusCreated).JSON(event)
}

func (h *socialHandlers) acceptTableInvite(c fiber.Ctx) error {
	if _, p := socialIdempotencyKey(c); p != nil {
		return p.Send(c)
	}
	eventID := strings.TrimSpace(c.Params("eventId"))
	if eventID == "" || len(eventID) > maxSocialIdempotencyKeySize {
		return problem.BadRequest("event id is invalid").Send(c)
	}
	event, room, err := h.svc.AcceptTableInvite(c.Context(), actorID(c), eventID)
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	return c.JSON(fiber.Map{"event": event, "room": sanitizeRoom(room, actorID(c))})
}

func (h *socialHandlers) declineTableInvite(c fiber.Ctx) error {
	if _, p := socialIdempotencyKey(c); p != nil {
		return p.Send(c)
	}
	eventID := strings.TrimSpace(c.Params("eventId"))
	if eventID == "" || len(eventID) > maxSocialIdempotencyKeySize {
		return problem.BadRequest("event id is invalid").Send(c)
	}
	if _, err := h.svc.DeclineTableInvite(c.Context(), actorID(c), eventID); err != nil {
		return socialProblem(err, c).Send(c)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *socialHandlers) listFriends(c fiber.Ctx) error {
	cursor := c.Query("cursor")
	edges, lastKey, err := h.svc.ListFriends(c.Context(), actorID(c), limitParam(c), decodeCursor(cursor))
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	players, err := h.hydrate(c, edges, false, true)
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
	players, err := h.hydrate(c, edges, false, false)
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
	players, err := h.hydrate(c, edges, false, false)
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

// relationshipBatch answers the table surface, which needs the mute/block
// state of every seated player at once to suppress chat and reactions before
// they reach the client's state. Profiles are not hydrated: the caller already
// has the names it is rendering.
func (h *socialHandlers) relationshipBatch(c fiber.Ctx) error {
	ids := make([]string, 0, maxRelationshipBatch)
	for _, raw := range strings.Split(c.Query("player_ids"), ",") {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if len(id) > maxSocialPlayerIDSize || strings.ContainsAny(id, "\x00\r\n") {
			return problem.BadRequest("player id is invalid").Send(c)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > maxRelationshipBatch {
		return problem.BadRequest("player_ids must contain between 1 and 25 ids").Send(c)
	}
	edges, err := h.svc.Relationships(c.Context(), actorID(c), ids)
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	result := make([]socialPlayerResponse, 0, len(edges))
	for _, id := range ids {
		edge, ok := edges[id]
		if !ok {
			continue
		}
		result = append(result, edgeState(&edge))
	}
	return c.JSON(fiber.Map{"data": result})
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

func (h *socialHandlers) hydrate(c fiber.Ctx, edges []social.Edge, includeFriendCode, includePresence bool) ([]socialPlayerResponse, error) {
	result := make([]socialPlayerResponse, 0, len(edges))
	ids := make([]string, 0, len(edges))
	for i := range edges {
		ids = append(ids, edges[i].OtherPlayerID)
	}
	profiles, err := h.players.GetMany(c.Context(), ids)
	if err != nil {
		return nil, err
	}
	statuses := map[string]presence.PlayerPresence{}
	var joinable map[string]string
	if includePresence && h.presence != nil {
		statuses, err = h.presence.GetMany(c.Context(), ids)
		if err != nil {
			return nil, err
		}
		joinable = h.joinableRoomIDs(c.Context(), statuses, profiles)
	}
	for i := range edges {
		profile, ok := profiles[edges[i].OtherPlayerID]
		if !ok {
			continue
		}
		response := h.response(&profile, &edges[i], includeFriendCode)
		if includePresence && h.presence != nil {
			status := statuses[edges[i].OtherPlayerID].Status
			response.Presence = &status
			response.RoomID = joinable[edges[i].OtherPlayerID]
		}
		result = append(result, response)
	}
	return result, nil
}

func (h *socialHandlers) listRecent(c fiber.Ctx) error {
	if h.recent == nil {
		return problem.InternalServer("recent players are unavailable", c, errors.New("recent players service unavailable")).Send(c)
	}
	cursor := c.Query("cursor")
	page, err := h.recent.List(c.Context(), actorID(c), decodeCursor(cursor), limitParam(c))
	if err != nil {
		return problem.InternalServer("failed to list recent players", c, err).Send(c)
	}
	ids := make([]string, 0, len(page.Players))
	for _, item := range page.Players {
		ids = append(ids, item.OpponentPlayerID)
	}
	profiles, err := h.players.GetMany(c.Context(), ids)
	if err != nil {
		return problem.InternalServer("failed to hydrate recent players", c, err).Send(c)
	}
	edges, err := h.svc.Relationships(c.Context(), actorID(c), ids)
	if err != nil {
		return socialProblem(err, c).Send(c)
	}
	friendIDs := make([]string, 0)
	for id, edge := range edges {
		if edge.Relationship == social.RelationshipFriend {
			friendIDs = append(friendIDs, id)
		}
	}
	statuses := map[string]presence.PlayerPresence{}
	if len(friendIDs) > 0 && h.presence != nil {
		statuses, err = h.presence.GetMany(c.Context(), friendIDs)
		if err != nil {
			return problem.InternalServer("failed to hydrate recent presence", c, err).Send(c)
		}
	}
	result := make([]socialPlayerResponse, 0, len(page.Players))
	for _, item := range page.Players {
		profile, ok := profiles[item.OpponentPlayerID]
		if !ok {
			continue
		}
		var edge *social.Edge
		if value, exists := edges[item.OpponentPlayerID]; exists {
			edge = &value
		}
		response := h.response(&profile, edge, false)
		response.LastPlayedAt, response.HandsTogether = item.LastPlayedAt, item.HandsTogether
		if edge != nil && edge.Relationship == social.RelationshipFriend && h.presence != nil {
			status := statuses[item.OpponentPlayerID].Status
			response.Presence = &status
		}
		result = append(result, response)
	}
	return sendPage(c, result, page.LastKey, cursor)
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
	case errors.Is(err, social.ErrInviteExpired):
		return problem.New(http.StatusConflict, "/problems/invite-expired", "Invite Expired", "the table invite has expired")
	case errors.Is(err, social.ErrRoomFull):
		return problem.TableFull()
	case errors.Is(err, social.ErrRoomClosed):
		return problem.New(http.StatusConflict, "/problems/room-closed", "Room Closed", "the room is no longer available")
	case errors.Is(err, social.ErrInviteAlreadySent):
		return problem.New(http.StatusConflict, "/problems/invite-already-pending", "Invite Already Pending", "an invitation is already pending")
	case errors.Is(err, social.ErrInviteForbidden), errors.Is(err, social.ErrInviteNotPending):
		return problem.New(http.StatusConflict, "/problems/relationship-conflict", "Relationship Conflict", "the social action cannot be completed")
	case errors.Is(err, social.ErrEventNotFound):
		return problem.NotFound("social event not found")
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
