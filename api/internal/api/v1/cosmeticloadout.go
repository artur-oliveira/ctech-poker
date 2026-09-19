package v1

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/cosmeticloadout"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type cosmeticLoadoutHandlers struct{ svc *cosmeticloadout.Service }

type CosmeticLoadoutCreateRequest struct {
	Name       string            `json:"name"`
	Selections map[string]string `json:"selections"`
}

// RegisterCosmeticLoadouts mounts /players/me/cosmetic-loadouts (issue
// #313): named, saved deck+felt combinations a player can apply in one
// action. GET doubles as the preview endpoint per the issue's own proposal —
// the client already has the public catalog and renders a preview from the
// ids in the response, so no separate preview endpoint exists.
func RegisterCosmeticLoadouts(router fiber.Router, auth fiber.Handler, svc *cosmeticloadout.Service) {
	h := &cosmeticLoadoutHandlers{svc: svc}
	g := router.Group("/players/me/cosmetic-loadouts", auth)
	g.Post("/", h.create)
	g.Get("/", h.list)
	g.Delete("/:id", h.delete)
	g.Post("/:id/apply", h.apply)
}

func (h *cosmeticLoadoutHandlers) create(c fiber.Ctx) error {
	var req CosmeticLoadoutCreateRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	userID := c.Locals(localsUserID).(string)
	loadout, err := h.svc.Create(c.Context(), userID, req.Name, req.Selections)
	if err != nil {
		return cosmeticLoadoutProblem(err).Send(c)
	}
	return c.Status(fiber.StatusCreated).JSON(loadout)
}

func (h *cosmeticLoadoutHandlers) list(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	loadouts, err := h.svc.List(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("list cosmetic loadouts failed", c, err).Send(c)
	}
	// A bounded (max 5) per-player set — same permanently-one-page envelope
	// RegisterCosmeticPurchase's catalog endpoint uses.
	return sendPage(c, loadouts, nil, "")
}

func (h *cosmeticLoadoutHandlers) delete(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	if err := h.svc.Delete(c.Context(), userID, c.Params("id")); err != nil {
		return cosmeticLoadoutProblem(err).Send(c)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *cosmeticLoadoutHandlers) apply(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	loadout, err := h.svc.Apply(c.Context(), userID, c.Params("id"))
	if err != nil {
		return cosmeticLoadoutProblem(err).Send(c)
	}
	return c.JSON(loadout)
}

func cosmeticLoadoutProblem(err error) *problem.Problem {
	switch {
	case errors.Is(err, cosmeticloadout.ErrNotFound):
		return problem.NotFound("loadout not found")
	case errors.Is(err, cosmeticloadout.ErrInvalidName), errors.Is(err, cosmeticloadout.ErrInvalidSelections):
		return problem.BadRequest(err.Error())
	case errors.Is(err, cosmeticloadout.ErrCosmeticNotOwned):
		return problem.BadRequest(err.Error())
	case errors.Is(err, cosmeticloadout.ErrLoadoutLimit):
		return problem.Conflict(err.Error())
	default:
		return problem.InternalServer("cosmetic loadout operation failed", nil, err)
	}
}
