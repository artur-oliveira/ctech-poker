package v1

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

type roomHandlers struct {
	rooms   *roomstore.Store
	buyin   *buyin.Service
	manager *tablemanager.Manager
}

func RegisterRooms(router fiber.Router, auth fiber.Handler, rooms *roomstore.Store, buyinSvc *buyin.Service, manager *tablemanager.Manager) {
	h := &roomHandlers{rooms: rooms, buyin: buyinSvc, manager: manager}
	g := router.Group("/rooms", auth)
	g.Post("/", h.createRoom)
	g.Get("/", h.listPublic)
	g.Get("/stakes", h.listStakes)
	g.Get("/:id", h.getRoom)
	g.Post("/:id/join", h.join)
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
	if req.SmallBlind <= 0 || req.BigBlind <= req.SmallBlind {
		return problem.BadRequest("blinds must be positive and big_blind greater than small_blind").Send(c)
	}
	if req.MaxSeats < 2 || req.MaxSeats > 9 {
		return problem.BadRequest("max_seats must be between 2 and 9").Send(c)
	}
	if req.BuyInMin <= 0 || req.BuyInMax < req.BuyInMin || req.BuyInMin%req.BigBlind != 0 || req.BuyInMax%req.BigBlind != 0 {
		return problem.BadRequest("buy-in limits must be ordered positive multiples of big_blind").Send(c)
	}
	if req.Visibility == "public" && req.BlindEscalation != nil {
		return problem.BadRequest("blind escalation is only configurable on private rooms").Send(c)
	}
	if req.Visibility == "public" && !isAllowedPublicStake("sandbox", req.SmallBlind, req.BigBlind) {
		return problem.BadRequest("unsupported public sandbox stake").Send(c)
	}
	if cfg := req.BlindEscalation; cfg != nil && (cfg.IntervalMinutes <= 0 || cfg.Multiplier <= 100 || cfg.Max < req.BigBlind) {
		return problem.BadRequest("invalid blind escalation").Send(c)
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
	room := roomstore.Room{
		ID: newRoomID(), Visibility: req.Visibility, CurrencyMode: "sandbox",
		SmallBlind: req.SmallBlind, BigBlind: req.BigBlind, MaxSeats: req.MaxSeats,
		BuyInMin: req.BuyInMin, BuyInMax: req.BuyInMax, EquityDisplayEnabled: equity,
		Status: "waiting", CreatedBy: userID, CreatedAt: dynamo.NowStr(),
	}
	if req.Visibility == "private" {
		room.ShareCode = newShareCode()
		room.BlindEscalation = req.BlindEscalation
	}
	if h.rooms != nil {
		if err := h.rooms.Create(c.Context(), room); err != nil {
			return problem.InternalServer("failed to create room").Send(c)
		}
	}
	if room.BlindEscalation != nil && h.manager != nil {
		cfg := *room.BlindEscalation
		_, _ = h.manager.GetOrCreateActor(c.Context(), room.ID, func() *hand.Table {
			return hand.NewTable(nil, room.SmallBlind, room.BigBlind)
		}, func(actor *tablemanager.Actor) {
			actor.SetEquityEnabledForActor(room.EquityDisplayEnabled)
			actor.StartEscalation(cfg)
		})
	}
	return c.Status(fiber.StatusCreated).JSON(room)
}

func (h *roomHandlers) listStakes(c fiber.Ctx) error {
	return c.JSON(sandboxStakeCatalog())
}

func (h *roomHandlers) listPublic(c fiber.Ctx) error {
	rooms, _, err := h.rooms.ListPublic(c.Context(), 50, c.Query("cursor"))
	if err != nil {
		return problem.InternalServer("failed to list rooms").Send(c)
	}
	return c.JSON(rooms)
}

func (h *roomHandlers) getRoom(c fiber.Ctx) error {
	room, err := h.rooms.Get(c.Context(), c.Params("id"))
	if err != nil {
		return problem.InternalServer("failed to get room").Send(c)
	}
	if room == nil {
		return problem.NotFound("room not found").Send(c)
	}
	return c.JSON(room)
}

func (h *roomHandlers) join(c fiber.Ctx) error {
	var req JoinRoomRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	room, err := h.rooms.Get(c.Context(), c.Params("id"))
	if err != nil {
		return problem.InternalServer("failed to get room").Send(c)
	}
	if room == nil {
		return problem.NotFound("room not found").Send(c)
	}
	if room.CurrencyMode != "sandbox" {
		return problem.BadRequest("unsupported currency mode").Send(c)
	}
	if req.Amount < room.BuyInMin || req.Amount > room.BuyInMax || req.Amount%room.BigBlind != 0 {
		return problem.BadRequest("amount must be within range and a multiple of big_blind").Send(c)
	}
	userID, _ := c.Locals(localsUserID).(string)
	if err := h.buyin.BuyIn(c.Context(), room.ID, userID, req.Amount, room.Status == "active"); err != nil {
		if errors.Is(err, buyin.ErrTermsNotAccepted) {
			return problem.Forbidden(err.Error()).Send(c)
		}
		return problem.Conflict(err.Error()).Send(c)
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *roomHandlers) leave(c fiber.Ctx) error {
	userID, _ := c.Locals(localsUserID).(string)
	stack, err := h.buyin.CashOut(c.Context(), c.Params("id"), userID)
	if err != nil {
		return problem.Conflict(err.Error()).Send(c)
	}
	return c.JSON(fiber.Map{"amount": stack})
}

func (h *roomHandlers) ready(c fiber.Ctx) error {
	return problem.NotImplemented("use the table WebSocket's ready message").Send(c)
}

func newRoomID() string    { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func newShareCode() string { var b [4]byte; _, _ = rand.Read(b[:]); return fmt.Sprintf("%X", b) }
