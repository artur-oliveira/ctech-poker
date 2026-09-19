package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

// WalletAlertPrefsRequest is the body for PUT /players/me/wallet-alerts.
// Either threshold left at zero disables that particular alert (#304).
type WalletAlertPrefsRequest struct {
	MinSandboxBalance int64 `json:"min_sandbox_balance"`
	MaxPurchaseCents  int64 `json:"max_purchase_cents"`
}

func (h *playerHandlers) walletAlertsGet(c fiber.Ctx) error {
	if h.walletAlerts == nil {
		return problem.NotFound("wallet alerts not available").Send(c)
	}
	userID := c.Locals(localsUserID).(string)
	prefs, err := h.walletAlerts.Get(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("load wallet alert prefs failed", c, err).Send(c)
	}
	if prefs == nil {
		return c.JSON(fiber.Map{"min_sandbox_balance": 0, "max_purchase_cents": 0})
	}
	return c.JSON(prefs)
}

func (h *playerHandlers) walletAlertsPut(c fiber.Ctx) error {
	if h.walletAlerts == nil {
		return problem.NotFound("wallet alerts not available").Send(c)
	}
	var req WalletAlertPrefsRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	if req.MinSandboxBalance < 0 || req.MaxPurchaseCents < 0 {
		return problem.BadRequest("thresholds must not be negative").Send(c)
	}
	userID := c.Locals(localsUserID).(string)
	prefs, err := h.walletAlerts.Set(c.Context(), userID, req.MinSandboxBalance, req.MaxPurchaseCents)
	if err != nil {
		return problem.InternalServer("save wallet alert prefs failed", c, err).Send(c)
	}
	return c.JSON(prefs)
}

func (h *playerHandlers) walletAlertsDelete(c fiber.Ctx) error {
	if h.walletAlerts == nil {
		return problem.NotFound("wallet alerts not available").Send(c)
	}
	userID := c.Locals(localsUserID).(string)
	if err := h.walletAlerts.Delete(c.Context(), userID); err != nil {
		return problem.InternalServer("delete wallet alert prefs failed", c, err).Send(c)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
