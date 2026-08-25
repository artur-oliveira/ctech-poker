package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
)

type sandboxPurchaseHandlers struct{ svc *sandboxpurchase.Service }

type SandboxPurchaseCreateRequest struct {
	SKU string `json:"sku"`
	// IdempotencyKey is stable per purchase click and reused across network
	// retries — mirrors JoinRoomRequest.IdempotencyKey's idem_key convention.
	IdempotencyKey string `json:"idem_key,omitempty"`
}

type SandboxPurchaseRefundRequest struct {
	IdempotencyKey string `json:"idem_key,omitempty"`
}

func RegisterSandboxPurchase(router fiber.Router, auth fiber.Handler, svc *sandboxpurchase.Service, purchaseLimiter *RateLimiter) {
	h := &sandboxPurchaseHandlers{svc: svc}
	g := router.Group("/wallet/sandbox-purchase", auth)
	g.Get("/skus", h.listSkus)
	g.Post("/", rateLimit(purchaseLimiter, ipKey("sandboxpurchase:create")), h.create)
	g.Get("/", h.list)
	g.Get("/:id", h.get)
	g.Post("/:id/refund", h.refund)
}

func (h *sandboxPurchaseHandlers) listSkus(c fiber.Ctx) error {
	skus, err := h.svc.ListSKUs(c.Context())
	if err != nil {
		return walletOrInternalProblem(err, "list skus failed", c).Send(c)
	}
	// The SKU catalog comes back whole from wallet; same envelope as every
	// other list endpoint, permanently on its only page.
	return sendPage(c, skus, nil, "")
}

func (h *sandboxPurchaseHandlers) create(c fiber.Ctx) error {
	var req SandboxPurchaseCreateRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.SKU == "" {
		return problem.BadRequest("sku is required").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.NewString()
	}
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Create(c.Context(), userID, req.SKU, idemKey)
	if err != nil {
		return walletOrInternalProblem(err, "purchase failed", c).Send(c)
	}
	return c.Status(fiber.StatusCreated).JSON(rec)
}

func (h *sandboxPurchaseHandlers) list(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	cursor := c.Query("cursor")
	records, lastKey, err := h.svc.List(c.Context(), userID, limitParam(c), decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("list purchases failed", c, err).Send(c)
	}
	return sendPage(c, records, lastKey, cursor)
}

func (h *sandboxPurchaseHandlers) get(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Refresh(c.Context(), userID, c.Params("id"))
	if errors.Is(err, sandboxpurchase.ErrNotFound) {
		return problem.NotFound("purchase not found").Send(c)
	}
	if err != nil {
		return walletOrInternalProblem(err, "refresh purchase failed", c).Send(c)
	}
	return c.JSON(rec)
}

func (h *sandboxPurchaseHandlers) refund(c fiber.Ctx) error {
	var req SandboxPurchaseRefundRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = uuid.NewString()
	}
	userID := c.Locals(localsUserID).(string)
	rec, err := h.svc.Refund(c.Context(), userID, c.Params("id"), idemKey)
	if errors.Is(err, sandboxpurchase.ErrNotFound) {
		return problem.NotFound("purchase not found").Send(c)
	}
	if err != nil {
		return walletOrInternalProblem(err, "refund failed", c).Send(c)
	}
	return c.JSON(rec)
}
