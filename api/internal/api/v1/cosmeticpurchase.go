package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type cosmeticPurchaseHandlers struct{ svc *cosmeticpurchase.Service }

type CosmeticPurchaseCreateRequest struct {
	ItemID         string `json:"item_id"`
	Method         string `json:"method"` // "pix" | "fichas"
	IdempotencyKey string `json:"idem_key,omitempty"`
}

type CosmeticPurchaseRefundRequest struct {
	IdempotencyKey string `json:"idem_key,omitempty"`
}

// RegisterCosmeticPurchase mounts /wallet/cosmetic-purchase/:kind, mirroring
// RegisterReactionPurchase's catalog/create/list/get/refund route shape. The
// :kind path param is validated against cosmetics.KindDeck/KindFelt before
// any service call.
func RegisterCosmeticPurchase(router fiber.Router, auth fiber.Handler, svc *cosmeticpurchase.Service, purchaseLimiter *RateLimiter) {
	h := &cosmeticPurchaseHandlers{svc: svc}
	g := router.Group("/wallet/cosmetic-purchase/:kind", auth, validCosmeticKind)
	g.Get("/catalog", h.catalog)
	g.Post("/", rateLimit(purchaseLimiter, ipKey("cosmeticpurchase:create")), h.create)
	g.Get("/", h.list)
	g.Get("/:id", h.get)
	g.Post("/:id/refund", h.refund)
}

func validCosmeticKind(c fiber.Ctx) error {
	kind := cosmetics.Kind(c.Params("kind"))
	if kind != cosmetics.KindDeck && kind != cosmetics.KindFelt {
		return problem.BadRequest("kind must be \"deck\" or \"felt\"").Send(c)
	}
	return c.Next()
}

func (h *cosmeticPurchaseHandlers) catalog(c fiber.Ctx) error {
	kind := cosmetics.Kind(c.Params("kind"))
	entries, err := h.svc.ListCatalog(c.Context(), c.Locals(localsUserID).(string), kind)
	if err != nil {
		return walletOrInternalProblem(err, "list catalog failed", c).Send(c)
	}
	// A fixed in-memory catalog has nothing to page through, but it is still a
	// list endpoint: same envelope, permanently on its only page.
	return sendPage(c, entries, nil, "")
}

func (h *cosmeticPurchaseHandlers) create(c fiber.Ctx) error {
	kind := cosmetics.Kind(c.Params("kind"))
	var req CosmeticPurchaseCreateRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.ItemID == "" {
		return problem.BadRequest("item_id is required").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.NewString()
	}
	userID := c.Locals(localsUserID).(string)
	switch req.Method {
	case "pix":
		rec, _, err := h.svc.CreateReal(c.Context(), userID, kind, req.ItemID, idemKey)
		if err != nil {
			return cosmeticPurchaseProblem(err, c).Send(c)
		}
		return c.Status(fiber.StatusCreated).JSON(rec)
	case "fichas":
		rec, err := h.svc.CreateSandbox(c.Context(), userID, kind, req.ItemID, idemKey)
		if err != nil {
			return cosmeticPurchaseProblem(err, c).Send(c)
		}
		return c.Status(fiber.StatusCreated).JSON(rec)
	default:
		return problem.BadRequest("method must be \"pix\" or \"fichas\"").Send(c)
	}
}

func (h *cosmeticPurchaseHandlers) list(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	cursor := c.Query("cursor")
	records, lastKey, err := h.svc.List(c.Context(), userID, cosmetics.Kind(c.Params("kind")), limitParam(c), decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("list cosmetic purchases failed", c, err).Send(c)
	}
	return sendPage(c, records, lastKey, cursor)
}

func (h *cosmeticPurchaseHandlers) get(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Refresh(c.Context(), userID, c.Params("id"))
	if errors.Is(err, cosmeticpurchase.ErrNotFound) {
		return problem.NotFound("purchase not found").Send(c)
	}
	if err != nil {
		return walletOrInternalProblem(err, "get purchase failed", c).Send(c)
	}
	return c.JSON(rec)
}

func (h *cosmeticPurchaseHandlers) refund(c fiber.Ctx) error {
	var req CosmeticPurchaseRefundRequest
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
		return cosmeticPurchaseProblem(err, c).Send(c)
	}
	return c.JSON(rec)
}

func cosmeticPurchaseProblem(err error, c fiber.Ctx) *problem.Problem {
	switch {
	case errors.Is(err, cosmeticpurchase.ErrNotFound):
		return problem.NotFound("purchase not found")
	case errors.Is(err, cosmeticpurchase.ErrInUse):
		return problem.Conflict("cosmetic is the player's current selection, cannot refund")
	case errors.Is(err, cosmeticpurchase.ErrAlreadyOwned):
		return problem.Conflict("cosmetic already owned")
	case errors.Is(err, cosmeticpurchase.ErrPurchasePending):
		return problem.Conflict("another purchase for this cosmetic is in progress")
	case errors.Is(err, cosmeticpurchase.ErrAlreadyRefunded):
		return problem.Conflict("purchase already refunded")
	case errors.Is(err, cosmeticpurchase.ErrNotConfirmed), errors.Is(err, cosmeticpurchase.ErrMissingEntitlement):
		return problem.Conflict(err.Error())
	case errors.Is(err, cosmeticpurchase.ErrUnknownItem), errors.Is(err, cosmeticpurchase.ErrNotPremium):
		return problem.BadRequest(err.Error())
	default:
		return walletOrInternalProblem(err, "cosmetic purchase failed", c)
	}
}
