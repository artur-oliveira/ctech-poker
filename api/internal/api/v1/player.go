package v1

import (
	"context"
	"errors"
	"log/slog"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

// Name and WalletMode are pointers so an absent key means "don't touch this
// field" — a wallet-mode-only update must not blank out the display name.
type UpdatePlayerRequest struct {
	Name        *string `json:"name"`
	WalletMode  *string `json:"wallet_mode"`
	DeckVariant *string `json:"deck_variant"`
}

type sessionLogReader interface {
	ListSessions(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.SessionItem, map[string]types.AttributeValue, error)
	ListHands(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error)
	ListHandsByTable(ctx context.Context, playerID, tableID string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error)
	GetHand(ctx context.Context, playerID, handID string) (*sessionlog.HandItem, error)
}

type playerAchievementStore interface {
	ListAchievements(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]achievements.PlayerAchievementProgress, map[string]types.AttributeValue, error)
}

type playerHandlers struct {
	players      *player.Service
	cfg          *config.Config
	sessions     sessionLogReader
	achievements playerAchievementStore
}

// RegisterPlayers mounts every /players/me/* route: profile, wallet-mode,
// terms acceptance, session/hand history, and achievement progress all live
// under the same resource and share the same auth-derived playerID.
func RegisterPlayers(router fiber.Router, auth fiber.Handler, players *player.Service, sessions sessionLogReader, achievementStore playerAchievementStore, cfg *config.Config) {
	h := &playerHandlers{players: players, sessions: sessions, achievements: achievementStore, cfg: cfg}
	g := router.Group("/players", auth)
	g.Get("/me", h.me)
	g.Post("/me", h.updateMe)
	g.Post("/me/terms/accept", h.acceptTerms)
	g.Get("/me/sessions", h.sessionHistory)
	g.Get("/me/hands", h.handHistory)
	g.Get("/me/hands/:handId", h.handByID)
	g.Get("/me/achievements", h.achievementProgress)
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
		if *req.WalletMode == "real" && (h.cfg == nil || !h.cfg.RealMoneyEnabled) {
			return problem.BadRequest("unsupported wallet mode").Send(c)
		}
		if _, err := h.players.SetWalletMode(c.Context(), userID, *req.WalletMode); err != nil {
			if errors.Is(err, player.ErrInvalidWalletMode) {
				return problem.BadRequest("wallet_mode must be sandbox or real").Send(c)
			}
			return problem.InternalServer("failed to update player profile", c, err).Send(c)
		}
	}
	if req.DeckVariant != nil {
		if _, err := h.players.SetDeckVariant(c.Context(), userID, *req.DeckVariant); err != nil {
			if errors.Is(err, player.ErrInvalidDeckVariant) {
				return problem.BadRequest("deck_variant must not be empty").Send(c)
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

func (h *playerHandlers) sessionHistory(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	cursor := c.Query("cursor")
	sessions, lastKey, err := h.sessions.ListSessions(c.Context(), userID, 50, decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("failed to list sessions", c, err).Send(c)
	}
	return sendPage(c, sessions, lastKey, cursor)
}

func (h *playerHandlers) handHistory(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	cursor := c.Query("cursor")
	limit := limitParam(c)

	if tableID := c.Query("table_id"); tableID != "" {
		hands, lastKey, err := h.sessions.ListHandsByTable(c.Context(), userID, tableID, limit, decodeCursor(cursor))
		if err != nil {
			return problem.InternalServer("failed to list hands", c, err).Send(c)
		}
		return sendPage(c, hands, lastKey, cursor)
	}
	hands, lastKey, err := h.sessions.ListHands(c.Context(), userID, limit, decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("failed to list hands", c, err).Send(c)
	}
	return sendPage(c, hands, lastKey, cursor)
}

func (h *playerHandlers) handByID(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	handID, err := url.PathUnescape(c.Params("handId"))
	if err != nil {
		return problem.BadRequest("hand id is invalid").Send(c)
	}
	if handID == "" {
		return problem.BadRequest("hand id is required").Send(c)
	}
	hand, err := h.sessions.GetHand(c.Context(), userID, handID)
	if err != nil {
		return problem.InternalServer("failed to list hands", c, err).Send(c)
	}
	if hand == nil {
		return problem.NotFound("hand not found").Send(c)
	}
	return c.JSON(hand)
}

func (h *playerHandlers) achievementProgress(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	cursor := c.Query("cursor")
	progress, lastKey, err := h.achievements.ListAchievements(c.Context(), userID, 100, decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("failed to list achievements", c, err).Send(c)
	}
	return sendPage(c, progress, lastKey, cursor)
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
		"deck_variant":            profile.EffectiveDeckVariant(),
		"poker_terms_accepted":    profile.TermsAccepted(),
		"poker_terms_accepted_at": profile.TermsAcceptedAt,
	}
}
