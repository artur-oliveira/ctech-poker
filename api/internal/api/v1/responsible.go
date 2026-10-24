package v1

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"gopkg.aoctech.app/api-commons/problem"
	"gopkg.aoctech.app/poker/api/internal/responsible"
)

// responsibleService is the part of responsible.Service the routes use.
type responsibleService interface {
	Status(ctx context.Context, userID string) (*responsible.Status, error)
	AcceptGambling(ctx context.Context, userID string) error
	SetLimits(ctx context.Context, userID string, daily, weekly, monthly int64) (*responsible.GameLimits, error)
	CancelPendingLimits(ctx context.Context, userID string) (*responsible.GameLimits, error)
	SelfExclude(ctx context.Context, userID, period string) (*responsible.SelfExclusion, error)
	RevokeSelfExclusion(ctx context.Context, userID string) error
}

type responsibleHandlers struct{ svc responsibleService }

type setLimitsRequest struct {
	Daily   int64 `json:"daily"`
	Weekly  int64 `json:"weekly"`
	Monthly int64 `json:"monthly"`
}

type selfExcludeRequest struct {
	Period string `json:"period"` // "30d" | "90d" | "indefinite"
}

// RegisterResponsible mounts /players/me/responsible: the player's consent, personal
// limits and self-exclusion for real-money play. They reduce exposure, so they are
// never behind the real-money switch.
func RegisterResponsible(router fiber.Router, auth fiber.Handler, svc responsibleService) {
	h := &responsibleHandlers{svc: svc}
	g := router.Group("/players/me/responsible", auth)
	g.Get("/", h.status)
	g.Post("/accept", h.accept)
	g.Put("/limits", h.setLimits)
	g.Delete("/limits/pending", h.cancelPending)
	g.Post("/self-exclusion", h.selfExclude)
	g.Delete("/self-exclusion", h.revokeExclusion)
}

func (h *responsibleHandlers) fail(c fiber.Ctx, err error, msg string) error {
	if p, ok := responsibleProblem(err); ok {
		return p.Send(c)
	}
	return internalProblem(msg, err).Send(c)
}

func userOf(c fiber.Ctx) string { return c.Locals(localsUserID).(string) }

func (h *responsibleHandlers) status(c fiber.Ctx) error {
	st, err := h.svc.Status(c.Context(), userOf(c))
	if err != nil {
		return h.fail(c, err, "failed to load responsible-gambling state")
	}
	return c.JSON(statusBody(st))
}

func statusBody(st *responsible.Status) fiber.Map {
	return fiber.Map{
		"addendum_version": responsible.CurrentGamblingAddendumVersion,
		"accepted":         st.Accepted,
		"limits":           st.Limits,
		"usage": fiber.Map{
			"daily": st.Usage.Daily, "weekly": st.Usage.Weekly, "monthly": st.Usage.Monthly,
			"day_resets_at": st.Usage.DayResetsAt, "week_resets_at": st.Usage.WeekResetsAt, "month_resets_at": st.Usage.MonthResetsAt,
		},
		"excluded": st.Excluded,
	}
}

func (h *responsibleHandlers) accept(c fiber.Ctx) error {
	if err := h.svc.AcceptGambling(c.Context(), userOf(c)); err != nil {
		return h.fail(c, err, "failed to record acceptance")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *responsibleHandlers) setLimits(c fiber.Ctx) error {
	var req setLimitsRequest
	if p := bindBody(c, &req); p != nil {
		return p.Send(c)
	}
	lim, err := h.svc.SetLimits(c.Context(), userOf(c), req.Daily, req.Weekly, req.Monthly)
	if err != nil {
		var rule *responsible.RuleError
		if errors.As(err, &rule) {
			return problem.BadRequest(rule.Message).Send(c)
		}
		return h.fail(c, err, "failed to save limits")
	}
	return c.JSON(lim)
}

func (h *responsibleHandlers) cancelPending(c fiber.Ctx) error {
	lim, err := h.svc.CancelPendingLimits(c.Context(), userOf(c))
	if err != nil {
		return h.fail(c, err, "failed to cancel the pending limits")
	}
	return c.JSON(lim)
}

func (h *responsibleHandlers) selfExclude(c fiber.Ctx) error {
	var req selfExcludeRequest
	if p := bindBody(c, &req); p != nil {
		return p.Send(c)
	}
	if _, err := responsible.ExclusionUntil(req.Period, time.Now()); err != nil {
		return problem.BadRequest(err.Error()).Send(c)
	}
	ex, err := h.svc.SelfExclude(c.Context(), userOf(c), req.Period)
	if err != nil {
		return h.fail(c, err, "failed to record the self-exclusion")
	}
	return c.Status(fiber.StatusCreated).JSON(ex)
}

func (h *responsibleHandlers) revokeExclusion(c fiber.Ctx) error {
	if err := h.svc.RevokeSelfExclusion(c.Context(), userOf(c)); err != nil {
		return h.fail(c, err, "failed to revoke the self-exclusion")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
