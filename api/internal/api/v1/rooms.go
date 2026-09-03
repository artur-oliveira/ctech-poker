package v1

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/oklog/ulid/v2"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/observability"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"

	goproto "google.golang.org/protobuf/proto"
)

var availableSeats = []int{2, 6, 9}

// A public table's buy-in window, in big blinds. Enforced on createRoom and
// used verbatim by join-or-create's fresh tables so both produce rooms the
// other endpoint would accept.
const (
	publicBuyInMinBigBlinds = 20
	publicBuyInMaxBigBlinds = 100
)

type roomHandlers struct {
	rooms    *roomstore.Store
	buyin    *buyin.Service
	manager  *tablemanager.Manager
	reg      ws.Registry
	cfg      *config.Config
	sessions *sessionlog.Store
}

func RegisterRooms(router fiber.Router, auth fiber.Handler, rooms *roomstore.Store, buyinSvc *buyin.Service, manager *tablemanager.Manager, reg ws.Registry, cfg *config.Config, sessions *sessionlog.Store, createLimiter, joinLimiter *RateLimiter) {
	h := &roomHandlers{rooms: rooms, buyin: buyinSvc, manager: manager, reg: reg, cfg: cfg, sessions: sessions}
	g := router.Group("/rooms", auth)
	g.Post("/", rateLimit(createLimiter, ipKey("rooms:create")), h.createRoom)
	// Both must be declared before "/:id", which would otherwise match them.
	g.Post("/join-or-create", rateLimit(joinLimiter, ipKey("rooms:join")), h.joinOrCreate)
	g.Get("/buckets", h.listBuckets)
	g.Get("/", h.listPublic)
	g.Get("/stakes", h.listStakes)
	g.Get("/code/:code", h.getByShareCode)
	g.Get("/:id", h.getRoom)
	g.Get("/:id/seated", h.seated)
	g.Post("/:id/join", rateLimit(joinLimiter, ipKey("rooms:join")), h.join)
	g.Post("/:id/leave", h.leave)
	g.Post("/:id/ready", h.ready)
}

func (h *roomHandlers) createRoom(c fiber.Ctx) error {
	var req CreateRoomRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.Visibility != "public" && req.Visibility != "private" {
		return problem.BadRequest("visibility must be public or private").Send(c)
	}
	currencyMode := req.CurrencyMode
	if currencyMode == "" {
		currencyMode = "sandbox"
	}
	if currencyMode != "sandbox" && currencyMode != "real" {
		return problem.BadRequest("currency_mode must be sandbox or real").Send(c)
	}
	if currencyMode == "real" && (h.cfg == nil || !h.cfg.RealMoneyEnabled) {
		return problem.BadRequest("unsupported currency mode").Send(c)
	}
	if req.SmallBlind <= 0 || req.BigBlind <= req.SmallBlind {
		return problem.BadRequest("blinds must be positive and big_blind greater than small_blind").Send(c)
	}
	if !slices.Contains(availableSeats, req.MaxSeats) {
		return problem.BadRequest("max_seats must be 2, 6 or 9").Send(c)
	}
	if req.BuyInMin <= 0 || req.BuyInMax < req.BuyInMin || req.BuyInMin%req.BigBlind != 0 || req.BuyInMax%req.BigBlind != 0 {
		return problem.BadRequest("buy-in limits must be ordered positive multiples of big_blind").Send(c)
	}
	if req.Visibility == "public" && (req.BuyInMin < (req.BigBlind*publicBuyInMinBigBlinds) || req.BuyInMax > (req.BigBlind*publicBuyInMaxBigBlinds)) {
		return problem.BadRequest("minimum buy in value must be at least 20 times the BB value. maximum buy in value must be at most 100 times the BB value").Send(c)
	}
	if req.Visibility == "public" && req.BlindEscalation != nil {
		return problem.BadRequest("blind escalation is only configurable on private rooms").Send(c)
	}
	// Real-money rooms must always use one of the fixed catalog stakes,
	// public or private — the fixed entry fee below only exists for
	// catalog tiers, so an off-catalog real-money room would have nothing
	// correct to charge. Sandbox private rooms keep free-form blinds.
	if (req.Visibility == "public" || currencyMode == "real") && !isAllowedPublicStake(currencyMode, req.SmallBlind, req.BigBlind) {
		return problem.BadRequest("unsupported stake for this currency mode").Send(c)
	}
	if cfg := req.BlindEscalation; cfg != nil && (cfg.IntervalMinutes <= 0 || cfg.Multiplier <= 100 || cfg.Max < req.BigBlind) {
		return problem.BadRequest("invalid blind escalation").Send(c)
	}
	if req.Visibility == "public" && req.TurnTimeoutSeconds != nil {
		return problem.BadRequest("turn timeout is only configurable on private rooms").Send(c)
	}
	if req.TurnTimeoutSeconds != nil && (*req.TurnTimeoutSeconds < 5 || *req.TurnTimeoutSeconds > 60) {
		return problem.BadRequest("turn_timeout_seconds must be between 5 and 60").Send(c)
	}
	userID, ok := c.Locals(localsUserID).(string)
	if !ok || userID == "" {
		return problem.Unauthorized("invalid credentials").Send(c)
	}
	equity := true
	if req.EquityDisplayEnabled != nil {
		equity = *req.EquityDisplayEnabled
	}
	if req.Visibility == "public" {
		equity = true
	}
	runItTwice := false
	if req.RunItTwiceEnabled != nil {
		runItTwice = *req.RunItTwiceEnabled
	}
	entryFeeCents := int64(0)
	tier := ""
	if currencyMode == "real" {
		entryFeeCents, tier, _ = realStakeLookup(req.SmallBlind, req.BigBlind)
	}
	room := roomstore.Room{
		ID:                   newRoomID(),
		Visibility:           req.Visibility,
		CurrencyMode:         currencyMode,
		SmallBlind:           req.SmallBlind,
		BigBlind:             req.BigBlind,
		MaxSeats:             req.MaxSeats,
		BuyInMin:             req.BuyInMin,
		BuyInMax:             req.BuyInMax,
		EntryFeeCents:        entryFeeCents,
		Tier:                 tier,
		EquityDisplayEnabled: equity,
		RunItTwiceEnabled:    runItTwice,
		Status:               "waiting",
		CreatedBy:            userID,
		CreatedAt:            dynamo.NowStr(),
	}
	if req.Visibility == "private" {
		shareCode, err := newShareCode()
		if err != nil {
			return problem.InternalServer("failed to create room", c, err).Send(c)
		}
		room.ShareCode = shareCode
		room.BlindEscalation = req.BlindEscalation
		if req.TurnTimeoutSeconds != nil {
			room.TurnTimeoutSeconds = *req.TurnTimeoutSeconds
		}
	}
	if h.rooms != nil {
		if err := h.rooms.Create(c.Context(), room); err != nil {
			return problem.InternalServer("failed to create room", c, err).Send(c)
		}
		h.broadcastRoomCreated(c, room)
	}
	if room.BlindEscalation != nil && h.manager != nil {
		// Escalation is now re-armed on every actor creation via the manager's
		// roomLoader (T6), so the createRoom hook only needs to warm the actor.
		if _, err := h.manager.GetOrCreateActor(c.Context(), room.ID, func() *hand.Table {
			return table.SeedForRoom(&room)
		}); err != nil {
			observability.Warn(c.Context(), "room actor warmup failed", err, "room_id", room.ID)
		}
	}
	return c.Status(fiber.StatusCreated).JSON(room)
}

func (h *roomHandlers) listStakes(c fiber.Ctx) error {
	mode := c.Query("currency_mode", "sandbox")
	switch mode {
	case "sandbox":
		return c.JSON(sandboxStakeCatalog())
	case "real":
		if h.cfg == nil || !h.cfg.RealMoneyEnabled {
			return problem.NotFound("real-money mode is not available").Send(c)
		}
		return c.JSON(realStakeCatalog())
	default:
		return problem.BadRequest("currency_mode must be sandbox or real").Send(c)
	}
}

func (h *roomHandlers) listPublic(c fiber.Ctx) error {
	cursor := c.Query("cursor")
	rooms, lastKey, err := h.rooms.ListPublic(c.Context(), 50, decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("failed to list rooms", c, err).Send(c)
	}
	return sendPage(c, rooms, lastKey, cursor)
}

// listBuckets is the lobby grid's availability aggregate. Unlike GET /rooms
// (which the client only ever read the first page of) it walks the whole
// public index, so "2 mesas abertas" is true for the bucket, not for page 1.
func (h *roomHandlers) listBuckets(c fiber.Ctx) error {
	mode := c.Query("currency_mode", roomstore.CurrencyModeSandbox)
	if mode != roomstore.CurrencyModeSandbox && mode != roomstore.CurrencyModeReal {
		return problem.BadRequest("currency_mode must be sandbox or real").Send(c)
	}
	rooms, err := h.rooms.ListAllPublic(c.Context())
	if err != nil {
		return problem.InternalServer("failed to list rooms", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"data": aggregateBuckets(rooms, mode)})
}

// aggregateBuckets groups public rooms by (blinds, seats) within one currency
// mode. A room with no recorded currency_mode predates the field and is
// sandbox by construction.
func aggregateBuckets(rooms []roomstore.Room, mode string) []RoomBucket {
	type key struct {
		smallBlind, bigBlind int64
		maxSeats             int
	}
	byKey := map[key]*RoomBucket{}
	for _, room := range rooms {
		roomMode := room.CurrencyMode
		if roomMode == "" {
			roomMode = roomstore.CurrencyModeSandbox
		}
		if roomMode != mode {
			continue
		}
		k := key{room.SmallBlind, room.BigBlind, room.MaxSeats}
		bucket, ok := byKey[k]
		if !ok {
			bucket = &RoomBucket{
				SmallBlind: room.SmallBlind, BigBlind: room.BigBlind,
				MaxSeats: room.MaxSeats, CurrencyMode: mode,
			}
			byKey[k] = bucket
		}
		free := room.MaxSeats - room.SeatsTaken
		if free < 0 {
			free = 0
		}
		bucket.Rooms++
		bucket.SeatsTaken += room.SeatsTaken
		bucket.SeatsAvailable += free
		if free > 0 {
			bucket.OpenRooms++
		}
	}
	out := make([]RoomBucket, 0, len(byKey))
	for _, bucket := range byKey {
		out = append(out, *bucket)
	}
	slices.SortFunc(out, func(a, b RoomBucket) int {
		if a.BigBlind != b.BigBlind {
			return int(a.BigBlind - b.BigBlind)
		}
		return a.MaxSeats - b.MaxSeats
	})
	return out
}

// joinOrCreate seats the caller in the bucket they asked for and answers with
// the table id it used. The client never picks a room id off a stale lobby
// page, so a lost seat race resolves server-side into another table instead
// of surfacing later as a buy-in error on the table page (#76).
func (h *roomHandlers) joinOrCreate(c fiber.Ctx) error {
	var req JoinOrCreateRoomRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.CurrencyMode == "" {
		req.CurrencyMode = roomstore.CurrencyModeSandbox
	}
	if req.CurrencyMode != roomstore.CurrencyModeSandbox && req.CurrencyMode != roomstore.CurrencyModeReal {
		return problem.BadRequest("currency_mode must be sandbox or real").Send(c)
	}
	if req.CurrencyMode == roomstore.CurrencyModeReal && (h.cfg == nil || !h.cfg.RealMoneyEnabled) {
		return problem.BadRequest("unsupported currency mode").Send(c)
	}
	if !slices.Contains(availableSeats, req.MaxSeats) {
		return problem.BadRequest("max_seats must be 2, 6 or 9").Send(c)
	}
	// Only catalog stakes: this endpoint only ever produces PUBLIC rooms, and
	// createRoom already refuses off-catalog public stakes.
	if !isAllowedPublicStake(req.CurrencyMode, req.SmallBlind, req.BigBlind) {
		return problem.BadRequest("unsupported stake for this currency mode").Send(c)
	}
	buyInMin, buyInMax := req.BigBlind*publicBuyInMinBigBlinds, req.BigBlind*publicBuyInMaxBigBlinds
	if req.Amount < buyInMin || req.Amount > buyInMax || req.Amount%req.BigBlind != 0 {
		return problem.BadRequest("amount must be within range and a multiple of big_blind").Send(c)
	}
	userID, ok := c.Locals(localsUserID).(string)
	if !ok || userID == "" {
		return problem.Unauthorized("invalid credentials").Send(c)
	}

	// A retry of this same click (or a second tab) must land on the seat the
	// player already holds, never buy a second one in a sibling table — so
	// an open session inside this bucket short-circuits the whole thing.
	// Best effort: a lookup failure only costs the short-circuit, and the
	// buy-in below is idempotent on its own for an already-seated player.
	if h.sessions != nil {
		if tableID, err := h.sessions.FindLatestOpenSession(c.Context(), userID); err != nil {
			observability.Warn(c.Context(), "join-or-create open session lookup failed", err, "player_id", userID)
		} else if tableID != "" {
			if room, err := h.rooms.Get(c.Context(), tableID); err == nil && room != nil && roomMatchesBucket(*room, req) {
				return c.JSON(JoinOrCreateRoomResponse{RoomID: room.ID})
			}
		}
	}

	rooms, err := h.rooms.ListAllPublic(c.Context())
	if err != nil {
		return problem.InternalServer("failed to list rooms", c, err).Send(c)
	}
	roomID, created, err := seatInBucket(
		openRoomsInBucket(rooms, req),
		func(room roomstore.Room) error {
			return h.buyin.BuyInWithAutoRebuy(c.Context(), room.ID, userID, req.Amount, room.Status == "active", req.AutoRebuy, req.IdempotencyKey)
		},
		func() (string, error) { return h.createAndSeat(c, req, userID) },
	)
	if err != nil {
		if errors.Is(err, buyin.ErrTermsNotAccepted) {
			return problem.Forbidden(err.Error()).Send(c)
		}
		if p, ok := problem.FromWalletError(err); ok {
			return p.Send(c)
		}
		return problem.Conflict(err.Error()).Send(c)
	}
	return c.JSON(JoinOrCreateRoomResponse{RoomID: roomID, Created: created})
}

// roomMatchesBucket reports whether room is one this bucket spec would have
// picked — same stakes, seats and currency, and public.
func roomMatchesBucket(room roomstore.Room, req JoinOrCreateRoomRequest) bool {
	mode := room.CurrencyMode
	if mode == "" {
		mode = roomstore.CurrencyModeSandbox
	}
	return room.Visibility == "public" && mode == req.CurrencyMode &&
		room.SmallBlind == req.SmallBlind && room.BigBlind == req.BigBlind &&
		room.MaxSeats == req.MaxSeats
}

// openRoomsInBucket narrows the public directory to the rooms that could seat
// this request, fullest first so players consolidate onto near-full tables
// instead of scattering one per table. seats_taken is a write-through mirror
// and can be stale, so this is a candidate list, not a guarantee — seatInBucket
// is what actually resolves the race.
func openRoomsInBucket(rooms []roomstore.Room, req JoinOrCreateRoomRequest) []roomstore.Room {
	out := make([]roomstore.Room, 0, len(rooms))
	for _, room := range rooms {
		if !roomMatchesBucket(room, req) || room.SeatsTaken >= room.MaxSeats {
			continue
		}
		if req.Amount < room.BuyInMin || req.Amount > room.BuyInMax {
			continue
		}
		out = append(out, room)
	}
	slices.SortFunc(out, func(a, b roomstore.Room) int {
		if a.SeatsTaken != b.SeatsTaken {
			return b.SeatsTaken - a.SeatsTaken
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// seatInBucket walks the candidates until one seats the player, falling
// through to create for a fresh table. A full table is never an error the
// caller sees: it just means the lobby page (or another player's join that
// landed first) was stale, which is precisely what this endpoint exists to
// absorb. Any other buy-in failure is real and stops the walk — retrying it
// against a sibling table would just repeat it, and on the wallet paths
// could debit twice.
func seatInBucket(candidates []roomstore.Room, seat func(roomstore.Room) error, create func() (string, error)) (roomID string, created bool, err error) {
	for _, room := range candidates {
		switch err := seat(room); {
		case err == nil:
			return room.ID, false, nil
		case errors.Is(err, table.ErrNoSeatsAvailable):
			continue
		default:
			return "", false, err
		}
	}
	roomID, err = create()
	return roomID, true, err
}

// createAndSeat opens a brand-new public table for the bucket and seats the
// caller in it. Buy-in bounds mirror createRoom's public-room rule.
func (h *roomHandlers) createAndSeat(c fiber.Ctx, req JoinOrCreateRoomRequest, userID string) (string, error) {
	room := roomstore.Room{
		ID:                   newRoomID(),
		Visibility:           "public",
		CurrencyMode:         req.CurrencyMode,
		SmallBlind:           req.SmallBlind,
		BigBlind:             req.BigBlind,
		MaxSeats:             req.MaxSeats,
		BuyInMin:             req.BigBlind * publicBuyInMinBigBlinds,
		BuyInMax:             req.BigBlind * publicBuyInMaxBigBlinds,
		EquityDisplayEnabled: true,
		Status:               "waiting",
		CreatedBy:            userID,
		CreatedAt:            dynamo.NowStr(),
	}
	if req.CurrencyMode == roomstore.CurrencyModeReal {
		room.EntryFeeCents, room.Tier, _ = realStakeLookup(req.SmallBlind, req.BigBlind)
	}
	if err := h.rooms.Create(c.Context(), room); err != nil {
		return "", err
	}
	h.broadcastRoomCreated(c, room)
	if err := h.buyin.BuyInWithAutoRebuy(c.Context(), room.ID, userID, req.Amount, false, req.AutoRebuy, req.IdempotencyKey); err != nil {
		return "", err
	}
	return room.ID, nil
}

func (h *roomHandlers) broadcastRoomCreated(c fiber.Ctx, room roomstore.Room) {
	if h.reg == nil {
		return
	}
	data, err := goproto.Marshal(&pokerproto.ServerMessage{Type: "room_created", Room: ConvertRoom(room)})
	if err != nil {
		observability.Warn(c.Context(), "room created event serialization failed", err, "room_id", room.ID)
		return
	}
	h.reg.Broadcast(c.Context(), "lobby", data)
}

func (h *roomHandlers) getRoom(c fiber.Ctx) error {
	room, err := h.rooms.Get(c.Context(), c.Params("id"))
	if err != nil {
		return problem.InternalServer("failed to get room", c, err).Send(c)
	}
	if room == nil {
		return problem.NotFound("room not found").Send(c)
	}
	userID, _ := c.Locals(localsUserID).(string)
	allowed, err := h.privateAccessAllowed(c.Context(), room, userID, "")
	if err != nil {
		return problem.InternalServer("failed to validate private room access", c, err).Send(c)
	}
	if !allowed {
		return problem.Forbidden("private room access required").Send(c)
	}
	return c.JSON(sanitizeRoom(room, userID))
}

// getByShareCode is how an invitee resolves a private room: they were handed
// the code out of band, so echoing it back leaks nothing.
func (h *roomHandlers) getByShareCode(c fiber.Ctx) error {
	room, err := h.rooms.GetByShareCode(c.Context(), c.Params("code"))
	if err != nil {
		return problem.InternalServer("failed to get room", c, err).Send(c)
	}
	if room == nil {
		return problem.NotFound("room not found").Send(c)
	}
	userID, _ := c.Locals(localsUserID).(string)
	return c.JSON(sanitizeRoom(room, userID))
}

// seated is the server-authoritative answer to "does this player already
// hold a live seat at this table?" — used by the client on table-page load
// so a player who closed their tab, or is opening the table from a second
// device, is not asked to repeat the buy-in ceremony for a seat they already
// have (playerID is always claims.Sub, never client-supplied — IDOR-safe).
func (h *roomHandlers) seated(c fiber.Ctx) error {
	room, err := h.rooms.Get(c.Context(), c.Params("id"))
	if err != nil {
		return problem.InternalServer("failed to get room", c, err).Send(c)
	}
	if room == nil {
		return problem.NotFound("room not found").Send(c)
	}
	userID, _ := c.Locals(localsUserID).(string)
	seated, stack, err := h.buyin.Seated(c.Context(), c.Params("id"), userID)
	if err != nil {
		return problem.InternalServer("failed to check seat", c, err).Send(c)
	}
	resp := fiber.Map{"seated": seated, "stack": stack}
	if room.CurrencyMode == roomstore.CurrencyModeReal && room.EntryFeeCents > 0 {
		feeDue, expiresAt, err := h.buyin.FeeStatus(c.Context(), room, userID)
		if err != nil {
			return problem.InternalServer("failed to check entry fee status", c, err).Send(c)
		}
		resp["entry_fee_cents"] = room.EntryFeeCents
		resp["fee_due"] = feeDue
		if !feeDue {
			resp["entitlement_expires_at"] = expiresAt
		} else {
			resp["entitlement_expires_at"] = nil
		}
	}
	return c.JSON(resp)
}

// sanitizeRoom strips the share code from any viewer other than the room's
// creator — knowing a private room's ID must not reveal its invite code.
func sanitizeRoom(room *roomstore.Room, viewerID string) roomstore.Room {
	out := *room
	if room.CreatedBy != viewerID {
		out.ShareCode = ""
	}
	return out
}

// privateRoomAccessAllowed gates joining a private room: the creator is always
// allowed; anyone else must present the share code (constant-time compare).
// Public rooms are always allowed.
func privateRoomAccessAllowed(room *roomstore.Room, viewerID, shareCode string, inviteGranted bool) bool {
	if room.Visibility != "private" || room.CreatedBy == viewerID {
		return true
	}
	if inviteGranted {
		return true
	}
	return room.ShareCode != "" &&
		subtle.ConstantTimeCompare([]byte(room.ShareCode), []byte(shareCode)) == 1
}

func (h *roomHandlers) privateAccessAllowed(ctx context.Context, room *roomstore.Room, viewerID, shareCode string) (bool, error) {
	if privateRoomAccessAllowed(room, viewerID, shareCode, false) {
		return true, nil
	}
	return h.rooms.HasInviteGrant(ctx, room.ID, viewerID)
}

func (h *roomHandlers) join(c fiber.Ctx) error {
	var req JoinRoomRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	room, err := h.rooms.Get(c.Context(), c.Params("id"))
	if err != nil {
		return problem.InternalServer("failed to get room", c, err).Send(c)
	}
	if room == nil {
		return problem.NotFound("room not found").Send(c)
	}
	if room.CurrencyMode != "sandbox" && (h.cfg == nil || !h.cfg.RealMoneyEnabled) {
		return problem.BadRequest("unsupported currency mode").Send(c)
	}
	if req.Amount < room.BuyInMin || req.Amount > room.BuyInMax || req.Amount%room.BigBlind != 0 {
		return problem.BadRequest("amount must be within range and a multiple of big_blind").Send(c)
	}
	userID, _ := c.Locals(localsUserID).(string)
	allowed, err := h.privateAccessAllowed(c.Context(), room, userID, req.ShareCode)
	if err != nil {
		return problem.InternalServer("failed to validate private room access", c, err).Send(c)
	}
	if !allowed {
		return problem.Forbidden("share code required to join a private room").Send(c)
	}
	if err := h.buyin.BuyInWithAutoRebuy(c.Context(), room.ID, userID, req.Amount, room.Status == "active", req.AutoRebuy, req.IdempotencyKey); err != nil {
		if errors.Is(err, buyin.ErrTermsNotAccepted) {
			return problem.Forbidden(err.Error()).Send(c)
		}
		if errors.Is(err, table.ErrNoSeatsAvailable) {
			return problem.TableFull().Send(c)
		}
		if p, ok := problem.FromWalletError(err); ok {
			return p.Send(c)
		}
		return problem.Conflict(err.Error()).Send(c)
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *roomHandlers) leave(c fiber.Ctx) error {
	var req LeaveRoomRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid request body").WithCause(err).Send(c)
	}
	userID, _ := c.Locals(localsUserID).(string)
	stack, err := h.buyin.CashOut(c.Context(), c.Params("id"), userID, req.IdempotencyKey)
	if err != nil {
		if p, ok := problem.FromWalletError(err); ok {
			return p.Send(c)
		}
		return problem.Conflict(err.Error()).Send(c)
	}
	return c.JSON(fiber.Map{"amount": stack})
}

func (h *roomHandlers) ready(c fiber.Ctx) error {
	return problem.NotImplemented("use the table WebSocket's ready message").Send(c)
}

func newRoomID() string { return ulid.MustNew(ulid.Now(), rand.Reader).String() }

// 6 random bytes = 12 hex chars: still typeable, but too sparse to brute-force
// online against GET /rooms/code/:code.
func newShareCode() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%X", b), nil
}
