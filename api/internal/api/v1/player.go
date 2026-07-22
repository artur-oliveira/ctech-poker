package v1

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/problem"
)

type playerHandlers struct{ players *player.Service }

// Name and WalletMode are pointers so an absent key means "don't touch this
// field" — a wallet-mode-only update must not blank out the display name.
type UpdatePlayerRequest struct {
	Name       *string `json:"name"`
	WalletMode *string `json:"wallet_mode"`
}

func RegisterPlayers(router fiber.Router, auth fiber.Handler, players *player.Service) {
	h := &playerHandlers{players: players}
	g := router.Group("/players", auth)
	g.Get("/me", h.me)
	g.Post("/me", h.updateMe)
	g.Post("/me/terms/accept", h.acceptTerms)
}

func (h *playerHandlers) me(c fiber.Ctx) error {
	profile, err := h.players.GetOrCreate(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("failed to load player profile", c, err).Send(c)
	}
	return c.JSON(h.responseWithBalance(c, profile))
}

// updateMe sets the caller's display name and/or wallet-mode preference. The
// frontend decodes the name from the id_token (poker's backend never sees
// that token) and calls this once after login if GET /players/me came back
// with no name set, or whenever the user edits their profile.
func (h *playerHandlers) updateMe(c fiber.Ctx) error {
	var req UpdatePlayerRequest
	if err := c.Bind().Body(&req); err != nil {
		return problem.BadRequest("invalid body").Send(c)
	}
	userID := c.Locals(localsUserID).(string)

	if req.Name != nil {
		if _, err := h.players.SetName(c.Context(), userID, *req.Name); err != nil {
			if errors.Is(err, player.ErrEmptyName) {
				return problem.BadRequest("name must not be empty").Send(c)
			}
			return problem.InternalServer("failed to update player profile", c, err).Send(c)
		}
	}
	if req.WalletMode != nil {
		if _, err := h.players.SetWalletMode(c.Context(), userID, *req.WalletMode); err != nil {
			if errors.Is(err, player.ErrInvalidWalletMode) {
				return problem.BadRequest("wallet_mode must be sandbox or real").Send(c)
			}
			return problem.InternalServer("failed to update player profile", c, err).Send(c)
		}
	}

	profile, err := h.players.GetOrCreate(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("failed to load player profile", c, err).Send(c)
	}
	return c.JSON(h.responseWithBalance(c, profile))
}

func (h *playerHandlers) acceptTerms(c fiber.Ctx) error {
	profile, err := h.players.AcceptTerms(c.Context(), c.Locals(localsUserID).(string))
	if err != nil {
		return problem.InternalServer("failed to accept poker terms", c, err).Send(c)
	}
	return c.JSON(playerResponse(profile))
}

// responseWithBalance adds the wallet balance to the profile response.
// A wallet lookup failure (e.g. ctech-wallet briefly down) does not fail the
// whole request — the profile itself is still valid without a balance.
func (h *playerHandlers) responseWithBalance(c fiber.Ctx, profile *player.PlayerProfile) fiber.Map {
	resp := playerResponse(profile)
	if balances, err := h.players.Balances(c.Context(), profile.UserID); err == nil {
		resp["game_balance"] = balances.GameBalance
		resp["sandbox_balance"] = balances.SandboxBalance
	} else {
		slog.Warn("player: balance lookup failed", "user_id", profile.UserID, "err", err)
	}
	return resp
}

func playerResponse(profile *player.PlayerProfile) fiber.Map {
	return fiber.Map{
		"user_id":                 profile.UserID,
		"name":                    profile.Name,
		"wallet_mode":             profile.EffectiveWalletMode(),
		"poker_terms_accepted":    profile.TermsAccepted(),
		"poker_terms_accepted_at": profile.TermsAcceptedAt,
	}
}
