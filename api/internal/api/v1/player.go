package v1

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/avatar"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/reports"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
)

// Name and WalletMode are pointers so an absent key means "don't touch this
// field" — a wallet-mode-only update must not blank out the display name.
type UpdatePlayerRequest struct {
	Name                 *string   `json:"name"`
	WalletMode           *string   `json:"wallet_mode"`
	DeckVariant          *string   `json:"deck_variant"`
	ShowcasePublic       *bool     `json:"showcase_public"`
	PlaystylePublic      *bool     `json:"playstyle_public"`
	FeaturedAchievements *[]string `json:"featured_achievements"`
	FavoriteReactions    *[]string `json:"favorite_reactions"`
}

type sessionLogReader interface {
	ListSessions(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.SessionItem, map[string]types.AttributeValue, error)
	ListHands(ctx context.Context, playerID, mode string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error)
	ListHandsByTable(ctx context.Context, playerID, mode, tableID string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error)
	GetHand(ctx context.Context, playerID, mode, handID string) (*sessionlog.HandItem, error)
}

type playerAchievementStore interface {
	ListAchievements(ctx context.Context, playerID, mode string, limit int, startKey map[string]types.AttributeValue) ([]achievements.PlayerAchievementProgress, map[string]types.AttributeValue, error)
}

type playerHandlers struct {
	players      *player.Service
	cfg          *config.Config
	sessions     sessionLogReader
	achievements playerAchievementStore
	avatars      *avatar.Service
	stats        pokerStatsReader
	reports      *reports.Service
}

// RegisterPlayers mounts every /players/me/* route: profile, wallet-mode,
// terms acceptance, session/hand history, and achievement progress all live
// under the same resource and share the same auth-derived playerID.
func RegisterPlayers(router fiber.Router, auth fiber.Handler, players *player.Service, sessions sessionLogReader, achievementStore playerAchievementStore, cfg *config.Config, avatars *avatar.Service, avatarLimiter *RateLimiter, stats pokerStatsReader, extras ...any) {
	var reportSvc *reports.Service
	for _, extra := range extras {
		if value, ok := extra.(*reports.Service); ok {
			reportSvc = value
		}
	}
	h := &playerHandlers{players: players, sessions: sessions, achievements: achievementStore, cfg: cfg, avatars: avatars, stats: stats, reports: reportSvc}
	router.Get("/players/:playerId/showcase", h.showcase)
	g := router.Group("/players", auth)
	g.Get("/me", h.me)
	g.Post("/me", h.updateMe)
	g.Post("/me/terms/accept", h.acceptTerms)
	g.Post("/me/avatar/upload-url", rateLimit(avatarLimiter, playerKey("avatar-upload")), h.avatarUploadURL)
	g.Post("/me/avatar/confirm", h.avatarConfirm)
	g.Delete("/me/avatar", h.avatarDelete)
	g.Post("/:playerId/avatar/report", rateLimit(avatarLimiter, playerKey("avatar-report")), h.avatarReport)
	g.Get("/me/sessions", h.sessionHistory)
	g.Get("/me/hands", h.handHistory)
	g.Get("/me/hand/:id", h.handByID)
	g.Get("/me/achievements", h.achievementProgress)
}

type confirmAvatarRequest struct {
	Version int `json:"version"`
}

func (h *playerHandlers) avatarUploadURL(c fiber.Ctx) error {
	if h.avatars == nil || !h.avatars.Enabled() {
		return problem.New(http.StatusServiceUnavailable, "/problems/avatar-disabled", "Avatar unavailable", "avatar uploads are disabled").Send(c)
	}
	userID := c.Locals(localsUserID).(string)
	profile, err := h.players.GetOrCreate(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("failed to load player profile", c, err).Send(c)
	}
	version := profile.AvatarVersion + 1
	upload, err := h.avatars.Presign(c.Context(), fmt.Sprintf("up/%s/%d.jpg", userID, version))
	if err != nil {
		return problem.InternalServer("failed to authorize avatar upload", c, err).Send(c)
	}
	return c.JSON(fiber.Map{"url": upload.URL, "fields": upload.Fields, "version": version})
}

func (h *playerHandlers) avatarConfirm(c fiber.Ctx) error {
	if h.avatars == nil || !h.avatars.Enabled() {
		return problem.New(http.StatusServiceUnavailable, "/problems/avatar-disabled", "Avatar unavailable", "avatar uploads are disabled").Send(c)
	}
	var req confirmAvatarRequest
	if err := c.Bind().Body(&req); err != nil || req.Version < 1 {
		return problem.BadRequest("version is required").Send(c)
	}
	userID := c.Locals(localsUserID).(string)
	profile, err := h.players.GetOrCreate(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("failed to load player profile", c, err).Send(c)
	}
	if req.Version != profile.AvatarVersion+1 {
		return problem.Conflict("avatar version is stale").Send(c)
	}
	uploadKey := fmt.Sprintf("up/%s/%d.jpg", userID, req.Version)
	publishedKey := fmt.Sprintf("av/%s/%d.jpg", userID, req.Version)
	if err := h.avatars.ValidateAndPublish(c.Context(), uploadKey, publishedKey); err != nil {
		switch {
		case errors.Is(err, avatar.ErrNotFound):
			return problem.NotFound("avatar upload not found").Send(c)
		case errors.Is(err, avatar.ErrEXIF):
			return problem.New(http.StatusUnprocessableEntity, "/problems/avatar-exif", "Invalid avatar", "a imagem contém metadados; selecione-a novamente").Send(c)
		case errors.Is(err, avatar.ErrInvalidImage), errors.Is(err, avatar.ErrImageTooLarge):
			return problem.New(http.StatusUnprocessableEntity, "/problems/avatar-invalid", "Invalid avatar", "a imagem não é um JPEG/PNG válido ou excede os limites").Send(c)
		default:
			return problem.InternalServer("failed to validate avatar", c, err).Send(c)
		}
	}
	updated, err := h.players.SetAvatar(c.Context(), userID, publishedKey, req.Version)
	if err != nil {
		return problem.InternalServer("failed to update avatar", c, err).Send(c)
	}
	h.avatars.DeleteBestEffort(c.Context(), uploadKey, profile.AvatarKey)
	return c.JSON(h.responseWithBalance(c, updated))
}

func (h *playerHandlers) avatarDelete(c fiber.Ctx) error {
	if h.avatars == nil || !h.avatars.Enabled() {
		return problem.New(http.StatusServiceUnavailable, "/problems/avatar-disabled", "Avatar unavailable", "avatar uploads are disabled").Send(c)
	}
	userID := c.Locals(localsUserID).(string)
	profile, err := h.players.GetOrCreate(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("failed to load player profile", c, err).Send(c)
	}
	updated, err := h.players.ClearAvatar(c.Context(), userID)
	if err != nil {
		return problem.InternalServer("failed to remove avatar", c, err).Send(c)
	}
	h.avatars.DeleteBestEffort(c.Context(), profile.AvatarKey)
	return c.JSON(h.responseWithBalance(c, updated))
}

func (h *playerHandlers) avatarReport(c fiber.Ctx) error {
	targetID, err := url.PathUnescape(c.Params("playerId"))
	if err != nil || targetID == "" {
		return problem.BadRequest("player id is invalid").Send(c)
	}
	reporterID := c.Locals(localsUserID).(string)
	if h.reports != nil {
		if _, err := h.reports.Create(c.Context(), reporterID, "legacy-avatar-report-v1", reports.CreateInput{
			TargetPlayerID: targetID, Category: reports.CategoryInappropriateProfile, Surface: reports.SurfaceProfile,
		}); err != nil {
			return reportProblem(err, c).Send(c)
		}
	}
	if err := h.players.ReportAvatar(c.Context(), targetID, reporterID); err != nil {
		return problem.InternalServer("failed to report avatar", c, err).Send(c)
	}
	return c.SendStatus(http.StatusNoContent)
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
	if req.ShowcasePublic != nil || req.PlaystylePublic != nil || req.FeaturedAchievements != nil {
		current, err := h.players.GetOrCreate(c.Context(), userID)
		if err != nil {
			return problem.InternalServer("failed to load player profile", c, err).Send(c)
		}
		public := current.ShowcasePublic
		playstylePublic := current.PlaystylePublic
		featured := current.FeaturedAchievements
		if req.ShowcasePublic != nil {
			public = *req.ShowcasePublic
		}
		if req.PlaystylePublic != nil {
			playstylePublic = *req.PlaystylePublic
		}
		if req.FeaturedAchievements != nil {
			featured = *req.FeaturedAchievements
		}
		if _, err := h.players.SetShowcase(c.Context(), userID, public, playstylePublic, featured); err != nil {
			if errors.Is(err, player.ErrInvalidShowcase) {
				return problem.BadRequest("featured_achievements must contain up to three valid unique keys").Send(c)
			}
			return problem.InternalServer("failed to update profile showcase", c, err).Send(c)
		}
	}
	if req.FavoriteReactions != nil {
		if _, err := h.players.SetFavoriteReactions(c.Context(), userID, *req.FavoriteReactions); err != nil {
			if errors.Is(err, player.ErrInvalidFavoriteReactions) {
				return problem.BadRequest("favorite_reactions must contain up to three valid unique reaction ids").Send(c)
			}
			return problem.InternalServer("failed to update favorite reactions", c, err).Send(c)
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
	return c.JSON(playerResponse(profile, h.avatarBaseURL()))
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
	mode := currencyModeParam(c)

	if tableID := c.Query("table_id"); tableID != "" {
		hands, lastKey, err := h.sessions.ListHandsByTable(c.Context(), userID, mode, tableID, limit, decodeCursor(cursor))
		if err != nil {
			return problem.InternalServer("failed to list hands", c, err).Send(c)
		}
		return sendPage(c, hands, lastKey, cursor)
	}
	hands, lastKey, err := h.sessions.ListHands(c.Context(), userID, mode, limit, decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("failed to list hands", c, err).Send(c)
	}
	return sendPage(c, hands, lastKey, cursor)
}

func (h *playerHandlers) handByID(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	handID, err := url.PathUnescape(c.Params("id"))
	if err != nil {
		return problem.BadRequest("hand id is invalid").Send(c)
	}
	if handID == "" {
		return problem.BadRequest("hand id is required").Send(c)
	}
	hand, err := h.sessions.GetHand(c.Context(), userID, currencyModeParam(c), handID)
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
	progress, lastKey, err := h.achievements.ListAchievements(c.Context(), userID, currencyModeParam(c), 100, decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("failed to list achievements", c, err).Send(c)
	}
	return sendPage(c, progress, lastKey, cursor)
}

func (h *playerHandlers) showcase(c fiber.Ctx) error {
	playerID, err := url.PathUnescape(c.Params("playerId"))
	if err != nil || playerID == "" {
		return problem.BadRequest("player id is invalid").Send(c)
	}
	profile, err := h.players.PublicShowcase(c.Context(), playerID)
	if errors.Is(err, player.ErrShowcasePrivate) {
		return problem.NotFound("profile showcase not found").Send(c)
	}
	if err != nil {
		return problem.InternalServer("failed to load profile showcase", c, err).Send(c)
	}
	progress, _, err := h.achievements.ListAchievements(c.Context(), playerID, roomstore.CurrencyModeSandbox, 100, nil)
	if err != nil {
		return problem.InternalServer("failed to load showcase achievements", c, err).Send(c)
	}
	counts := make(map[string]int, len(progress))
	for _, item := range progress {
		counts[item.Key] = item.Count
	}
	featured := make([]fiber.Map, 0, len(profile.FeaturedAchievements))
	for _, key := range profile.FeaturedAchievements {
		featured = append(featured, fiber.Map{"key": key, "count": counts[key]})
	}

	var bestHand fiber.Map
	var bestNet int64
	if hands, _, listErr := h.sessions.ListHands(c.Context(), playerID, roomstore.CurrencyModeSandbox, 50, nil); listErr == nil {
		for i := range hands {
			if hands[i].NetChange > bestNet {
				bestNet = hands[i].NetChange
				// Opponent identities/cards, storage keys and shuffle secrets
				// are deliberately absent from the public projection.
				bestHand = fiber.Map{
					"hand_id": hands[i].HandID, "table_id": hands[i].TableID,
					"net_change": hands[i].NetChange, "ended_at": hands[i].EndedAt,
					"board": hands[i].Board, "hole_cards": hands[i].HoleCards,
				}
			}
		}
	}
	response := fiber.Map{
		"player_id":             profile.UserID,
		"name":                  profile.Name,
		"avatar_url":            player.AvatarURL(profile, h.avatarBaseURL()),
		"featured_achievements": featured,
		"best_hand":             bestHand,
	}
	if profile.PlaystylePublic && h.stats != nil {
		stats, statsErr := h.stats.Get(c.Context(), playerID, roomstore.CurrencyModeSandbox)
		if statsErr != nil {
			return problem.InternalServer("failed to load profile playstyle", c, statsErr).Send(c)
		}
		if badges := pokerstats.StyleFor(stats, pokerstats.MinHandsPublic); len(badges) > 0 {
			response["playstyle"] = badges
		}
	}
	return c.JSON(response)
}

// responseWithBalance adds the wallet balance to the profile response.
// A wallet lookup failure (e.g. ctech-wallet briefly down) does not fail the
// whole request — the profile itself is still valid without a balance.
func (h *playerHandlers) responseWithBalance(c fiber.Ctx, profile *player.PlayerProfile) fiber.Map {
	resp := playerResponse(profile, h.avatarBaseURL())
	if balances, err := h.players.Balances(c.Context(), profile.UserID); err == nil {
		resp["game_balance"] = balances.GameBalance
		resp["sandbox_balance"] = balances.SandboxBalance
	} else {
		slog.Warn("player: balance lookup failed", "user_id", profile.UserID, "err", err)
	}
	return resp
}

func (h *playerHandlers) avatarBaseURL() string {
	if h.cfg == nil {
		return ""
	}
	return h.cfg.AvatarBaseURL
}

func playerResponse(profile *player.PlayerProfile, avatarBaseURL string) fiber.Map {
	return fiber.Map{
		"user_id":                 profile.UserID,
		"name":                    profile.Name,
		"friend_code":             profile.FriendCode,
		"avatar_url":              player.AvatarURL(profile, avatarBaseURL),
		"wallet_mode":             profile.EffectiveWalletMode(),
		"deck_variant":            profile.EffectiveDeckVariant(),
		"showcase_public":         profile.ShowcasePublic,
		"playstyle_public":        profile.PlaystylePublic,
		"featured_achievements":   profile.FeaturedAchievements,
		"favorite_reactions":      profile.FavoriteReactions,
		"poker_terms_accepted":    profile.TermsAccepted(),
		"poker_terms_accepted_at": profile.TermsAcceptedAt,
	}
}
