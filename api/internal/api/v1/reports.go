package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/reports"
)

type ReportLimiters struct{ Player, IP *RateLimiter }

type reportRequest struct {
	TargetPlayerID string           `json:"target_player_id"`
	Category       reports.Category `json:"category"`
	Surface        reports.Surface  `json:"surface"`
	TableID        string           `json:"table_id"`
	HandID         string           `json:"hand_id"`
	ActionID       string           `json:"action_id"`
	Details        string           `json:"details"`
}

func RegisterReports(router fiber.Router, auth fiber.Handler, svc *reports.Service, cfg *config.Config, limiters ReportLimiters) {
	if svc == nil {
		return
	}
	group := router.Group("/social", auth, firstPartyOnly)
	group.Post("/reports", reportRateLimit(limiters.Player, playerKey("social:reports"), cfg),
		reportRateLimit(limiters.IP, ipKey("social:reports"), cfg), func(c fiber.Ctx) error {
			key, p := socialIdempotencyKey(c)
			if p != nil {
				return p.Send(c)
			}
			var body reportRequest
			if err := c.Bind().Body(&body); err != nil {
				return problem.BadRequest("invalid body").Send(c)
			}
			created, err := svc.Create(c.Context(), actorID(c), key, reports.CreateInput{
				TargetPlayerID: strings.TrimSpace(body.TargetPlayerID), Category: body.Category, Surface: body.Surface,
				TableID: body.TableID, HandID: body.HandID, ActionID: body.ActionID, Details: body.Details,
			})
			if err != nil {
				return reportProblem(err, c).Send(c)
			}
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"report_id": created.ReportID, "status": created.Status})
		})
}

func reportRateLimit(limiter *RateLimiter, keyFn func(fiber.Ctx) string, cfg *config.Config) fiber.Handler {
	return func(c fiber.Ctx) error {
		if limiter == nil {
			return c.Next()
		}
		allowed, err := limiter.Allow(c.Context(), keyFn(c))
		if err != nil {
			slog.Warn("report rate limiter backend error; allowing request", "err", err)
			return c.Next()
		}
		if allowed {
			return c.Next()
		}
		return problem.New(http.StatusTooManyRequests, "/problems/report-rate-limited", "Report Rate Limited", "too many reports; try again later").Send(c)
	}
}

func reportProblem(err error, c fiber.Ctx) *problem.Problem {
	switch {
	case errors.Is(err, reports.ErrInvalidReport), errors.Is(err, reports.ErrEvidenceMissing):
		return problem.BadRequest("report or evidence is invalid")
	case errors.Is(err, reports.ErrTargetNotFound):
		return problem.NotFound("target player not found")
	default:
		return problem.InternalServer("failed to create report", c, err)
	}
}
