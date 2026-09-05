package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type reactionWheelRequest struct {
	ReactionIDs []string `json:"reaction_ids"`
}

// RegisterReactionWheel mounts GET/PUT /players/me/reaction-wheel (#338): the
// player's own ordered subset of owned reactions for the quick-react wheel.
// Same auth/validation shape as every other players/me/* endpoint.
func RegisterReactionWheel(router fiber.Router, auth fiber.Handler, players *player.Service) {
	g := router.Group("/players/me/reaction-wheel", auth)
	g.Get("", func(c fiber.Ctx) error {
		profile, err := players.GetOrCreate(c.Context(), c.Locals(localsUserID).(string))
		if err != nil {
			return problem.InternalServer("failed to load reaction wheel", c, err).Send(c)
		}
		return c.JSON(fiber.Map{"reaction_ids": profile.ReactionWheel})
	})
	g.Put("", func(c fiber.Ctx) error {
		var req reactionWheelRequest
		if err := c.Bind().Body(&req); err != nil {
			return problem.BadRequest("invalid body").Send(c)
		}
		userID := c.Locals(localsUserID).(string)
		profile, err := players.SetReactionWheel(c.Context(), userID, req.ReactionIDs)
		if err != nil {
			switch {
			case errors.Is(err, player.ErrInvalidReactionWheel):
				return problem.BadRequest("reaction_ids must be known, unique reaction ids within the wheel limit").Send(c)
			case errors.Is(err, player.ErrReactionNotOwned):
				return problem.BadRequest("reaction_ids includes a premium reaction you do not own").Send(c)
			}
			return problem.InternalServer("failed to update reaction wheel", c, err).Send(c)
		}
		return c.JSON(fiber.Map{"reaction_ids": profile.ReactionWheel})
	})
}
