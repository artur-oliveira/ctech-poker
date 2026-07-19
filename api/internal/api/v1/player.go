package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type playerHandlers struct{ players *player.Service }

func RegisterPlayers(router fiber.Router, auth fiber.Handler, players *player.Service) {
	h := &playerHandlers{players: players}
	g := router.Group("/players", auth)
	g.Get("/me", h.me)
	g.Post("/me/terms/accept", h.acceptTerms)
}

func (h *playerHandlers) me(c fiber.Ctx) error {
	profile, err := h.players.GetOrCreate(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("failed to load player profile").Send(c)
	}
	return c.JSON(playerResponse(profile))
}

func (h *playerHandlers) acceptTerms(c fiber.Ctx) error {
	profile, err := h.players.AcceptTerms(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("failed to accept poker terms").Send(c)
	}
	return c.JSON(playerResponse(profile))
}

func playerResponse(profile *player.PlayerProfile) fiber.Map {
	return fiber.Map{
		"user_id":                 profile.UserID,
		"poker_terms_accepted":    profile.TermsAccepted(),
		"poker_terms_accepted_at": profile.TermsAcceptedAt,
	}
}
