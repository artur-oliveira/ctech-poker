package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
)

type reactionPurchaseHandlers struct{ svc *reactionpurchase.Service }

type ReactionPurchaseCreateRequest struct {
	ReactionID     string `json:"reaction_id"`
	Method         string `json:"method"` // "pix" | "fichas"
	IdempotencyKey string `json:"idem_key,omitempty"`
}

type ReactionPurchaseRefundRequest struct {
	IdempotencyKey string `json:"idem_key,omitempty"`
}

func RegisterReactionPurchase(router fiber.Router, auth fiber.Handler, svc *reactionpurchase.Service, purchaseLimiter *RateLimiter) {
	h := &reactionPurchaseHandlers{svc: svc}
	g := router.Group("/wallet/reaction-purchase", auth)
	g.Get("/catalog", h.catalog)
	g.Post("/", rateLimit(purchaseLimiter, ipKey("reactionpurchase:create")), h.create)
	g.Get("/", h.list)
	g.Get("/:id", h.get)
	g.Post("/:id/refund", h.refund)
}

func (h *reactionPurchaseHandlers) catalog(c fiber.Ctx) error {
	entries, err := h.svc.ListCatalog(c.Context())
	if err != nil {
		return walletOrInternalProblem(err, "list catalog failed", c).Send(c)
	}
	return c.JSON(entries)
}

func (h *reactionPurchaseHandlers) create(c fiber.Ctx) error {
	var req ReactionPurchaseCreateRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.ReactionID == "" {
		return problem.BadRequest("reaction_id is required").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.NewString()
	}
	userID := c.Locals(localsUserID).(string)
	switch req.Method {
	case "pix":
		rec, _, err := h.svc.CreateReal(c.Context(), userID, req.ReactionID, idemKey)
		if err != nil {
			return reactionPurchaseProblem(err, c).Send(c)
		}
		return c.Status(fiber.StatusCreated).JSON(rec)
	case "fichas":
		rec, err := h.svc.CreateSandbox(c.Context(), userID, req.ReactionID, idemKey)
		if err != nil {
			return reactionPurchaseProblem(err, c).Send(c)
		}
		return c.Status(fiber.StatusCreated).JSON(rec)
	default:
		return problem.BadRequest("method must be \"pix\" or \"fichas\"").Send(c)
	}
}

func (h *reactionPurchaseHandlers) list(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	records, err := h.svc.List(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("list reaction purchases failed", c, err).Send(c)
	}
	return c.JSON(records)
}

func (h *reactionPurchaseHandlers) get(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Get(c.Context(), userID, c.Params("id"))
	if errors.Is(err, reactionpurchase.ErrNotFound) {
		return problem.NotFound("purchase not found").Send(c)
	}
	if err != nil {
		return walletOrInternalProblem(err, "get purchase failed", c).Send(c)
	}
	return c.JSON(rec)
}

func (h *reactionPurchaseHandlers) refund(c fiber.Ctx) error {
	var req ReactionPurchaseRefundRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.NewString()
	}
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Refund(c.Context(), userID, c.Params("id"), idemKey)
	if err != nil {
		return reactionPurchaseProblem(err, c).Send(c)
	}
	return c.JSON(rec)
}

func reactionPurchaseProblem(err error, c fiber.Ctx) *problem.Problem {
	switch {
	case errors.Is(err, reactionpurchase.ErrNotFound):
		return problem.NotFound("purchase not found")
	case errors.Is(err, reactionpurchase.ErrAlreadyUsed):
		return problem.Conflict("reaction already used, cannot refund")
	case errors.Is(err, reactionpurchase.ErrUnknownReaction), errors.Is(err, reactionpurchase.ErrNotPremium):
		return problem.BadRequest(err.Error())
	default:
		return walletOrInternalProblem(err, "reaction purchase failed", c)
	}
}
