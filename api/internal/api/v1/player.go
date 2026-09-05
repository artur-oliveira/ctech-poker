package v1

import (
	"context"
	"errors"
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
	TableTheme           *string   `json:"table_theme"`
	ShowcasePublic       *bool     `json:"showcase_public"`
	TablePublic          *bool     `json:"table_public"`
	PlaystylePublic      *bool     `json:"playstyle_public"`
	FeaturedAchievements *[]string `json:"featured_achievements"`
	FavoriteReactions    *[]string `json:"favorite_reactions"`
}

type sessionLogReader interface {
	ListSessions(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.SessionItem, map[string]types.AttributeValue, error)
	ListHands(ctx context.Context, playerID, mode string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error)
	ListHandsByTable(ctx context.Context, playerID, mode, tableID string, limit int, startKey map[string]types.AttributeValue) ([]sessionlog.HandItem, map[string]types.AttributeValue, error)
	GetHand(ctx context.Context, playerID, mode, handID string) (*sessionlog.HandItem, error)
	// BestPublicHand backs the anonymous showcase and therefore reads only
	// public attributes — never the full HandItem, which carries opponents,
	// seeds and fairness maps a visitor may not see (#225).
	BestPublicHand(ctx context.Context, playerID, mode string) (*sessionlog.PublicHandSummary, error)
}

type playerAchievementStore interface {
	ListAchievements(ctx context.Context, playerID, mode string, limit int, startKey map[string]types.AttributeValue) ([]achievements.PlayerAchievementProgress, map[string]types.AttributeValue, error)
	AllAchievements(ctx context.Context, playerID, mode string) ([]achievements.PlayerAchievementProgress, error)
}

type playerHandlers struct {
	players      *player.Service
	cfg          *config.Config
	sessions     sessionLogReader
	achievements playerAchievementStore
	avatars      *avatar.Service
	stats        pokerStatsReader
	reports      *reports.Service
	// identity pushes a renamed display name into the table the player is
	// seated at, so opponents stop seeing the old one without a reconnect
	// (#64). nil wherever the WS/table stack isn't wired (tests).
	identity *tableIdentityPusher
}

// RegisterPlayers mounts every /players/me/* route: profile, wallet-mode,
// terms acceptance, session/hand history, and achievement progress all live
// under the same resource and share the same auth-derived playerID.
func RegisterPlayers(router fiber.Router, auth fiber.Handler, players *player.Service, sessions sessionLogReader, achievementStore playerAchievementStore, cfg *config.Config, avatars *avatar.Service, avatarLimiter *RateLimiter, stats pokerStatsReader, extras ...any) {
	var reportSvc *reports.Service
	var identity *tableIdentityPusher
	for _, extra := range extras {
		switch value := extra.(type) {
		case *reports.Service:
			reportSvc = value
		case *tableIdentityPusher:
			identity = value
		}
	}
	h := &playerHandlers{players: players, sessions: sessions, achievements: achievementStore, cfg: cfg, avatars: avatars, stats: stats, reports: reportSvc, identity: identity}
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
	g.Get("/me/achievements/summary", h.achievementsSummary)
	g.Get("/me/reports", h.myReports)
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
	uploadKey, err := avatar.UploadKey(userID, version)
	if err != nil {
		return problem.InternalServer("failed to authorize avatar upload", c, err).Send(c)
	}
	upload, err := h.avatars.Presign(c.Context(), uploadKey)
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
	uploadKey, uploadErr := avatar.UploadKey(userID, req.Version)
	publishedKey, publishErr := avatar.PublishedKey(userID, req.Version)
	if uploadErr != nil || publishErr != nil {
		return problem.InternalServer("failed to derive avatar keys", c, errors.Join(uploadErr, publishErr)).Send(c)
	}
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
		// The seat's name is a cache the table actor filled at join time and
		// never re-reads, so a rename would otherwise stay invisible to
		// everyone at the table until this player reconnected (#64).
		h.identity.push(c.Context(), userID)
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
			if errors.Is(err, player.ErrCosmeticNotOwned) {
				return problem.BadRequest("deck_variant is premium and not owned").Send(c)
			}
			return problem.InternalServer("failed to update player profile", c, err).Send(c)
		}
	}
	if req.TableTheme != nil {
		if _, err := h.players.SetTableTheme(c.Context(), userID, *req.TableTheme); err != nil {
			if errors.Is(err, player.ErrInvalidTableTheme) {
				return problem.BadRequest("table_theme must be a known felt id").Send(c)
			}
			if errors.Is(err, player.ErrCosmeticNotOwned) {
				return problem.BadRequest("table_theme is premium and not owned").Send(c)
			}
			return problem.InternalServer("failed to update player profile", c, err).Send(c)
		}
	}
	if req.ShowcasePublic != nil || req.PlaystylePublic != nil || req.TablePublic != nil || req.FeaturedAchievements != nil {
		current, err := h.players.GetOrCreate(c.Context(), userID)
		if err != nil {
			return problem.InternalServer("failed to load player profile", c, err).Send(c)
		}
		public := current.ShowcasePublic
		playstylePublic := current.PlaystylePublic
		tablePublic := current.TablePublic
		featured := current.FeaturedAchievements
		if req.ShowcasePublic != nil {
			public = *req.ShowcasePublic
		}
		if req.PlaystylePublic != nil {
			playstylePublic = *req.PlaystylePublic
		}
		if req.TablePublic != nil {
			tablePublic = *req.TablePublic
		}
		if req.FeaturedAchievements != nil {
			featured = *req.FeaturedAchievements
		}
		if _, err := h.players.SetShowcase(c.Context(), userID, public, playstylePublic, tablePublic, featured); err != nil {
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

// myReports lets a player track the status of reports they themselves filed
// — never reports filed against them (ListByReporter keys off the reporter,
// not the target). Only the sanitized PlayerReportView shape is ever sent:
// no Details, EvidenceMessage, ReviewedBy/ResolvedBy, and Resolution is
// translated to a generic status message, not the raw internal enum.
func (h *playerHandlers) myReports(c fiber.Ctx) error {
	if h.reports == nil {
		return c.JSON(fiber.Map{"reports": []reports.PlayerReportView{}})
	}
	userID := c.Locals(localsUserID).(string)
	cursor := c.Query("cursor")
	page, err := h.reports.ListByReporter(c.Context(), userID, cursor, limitParam(c))
	if err != nil {
		return problem.InternalServer("failed to list reports", c, err).Send(c)
	}
	views := make([]reports.PlayerReportView, 0, len(page.Reports))
	for _, r := range page.Reports {
		views = append(views, r.Summary().ForReporter())
	}
	return c.JSON(fiber.Map{"reports": views, "next_cursor": page.NextCursor})
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
		if err := h.resolveOpponentProfiles(c.Context(), hands); err != nil {
			return problem.InternalServer("failed to resolve opponent profiles", c, err).Send(c)
		}
		return sendPage(c, hands, lastKey, cursor)
	}
	hands, lastKey, err := h.sessions.ListHands(c.Context(), userID, mode, limit, decodeCursor(cursor))
	if err != nil {
		return problem.InternalServer("failed to list hands", c, err).Send(c)
	}
	if err := h.resolveOpponentProfiles(c.Context(), hands); err != nil {
		return problem.InternalServer("failed to resolve opponent profiles", c, err).Send(c)
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
	if err := h.resolveOpponentProfiles(c.Context(), []sessionlog.HandItem{*hand}); err != nil {
		return problem.InternalServer("failed to resolve opponent profiles", c, err).Send(c)
	}
	return c.JSON(hand)
}

// resolveOpponentProfiles overwrites each opponent's denormalized Name and
// AvatarURL with what their live player profile resolves to right now, in
// place. Both are captured once at hand-complete
// (`sessionlog.OpponentSummary`, no TTL) and never updated again, so without
// this a hand recorded before a later `ClearAvatar` (or an avatar-report
// block) keeps serving a URL to a since-deleted object — a 404 in the
// opponent list (#68) — and a hand recorded before a rename keeps showing the
// old display name forever (#64). This is the read-time resolution #64
// proposes: nothing is backfilled and no rename fans out, the stored copies
// simply stop being what anyone reads, and a row whose player no longer has
// a profile falls back to whatever the hand recorded. `player.Service.GetMany`
// chunks the lookup at `player.MaxBatchProfileIDs` — a single hand-history
// page full of distinct opponents can exceed BatchGetItem's key ceiling even
// though any one hand has at most a table's worth of seats.
func (h *playerHandlers) resolveOpponentProfiles(ctx context.Context, hands []sessionlog.HandItem) error {
	seen := make(map[string]struct{})
	ids := make([]string, 0)
	for i := range hands {
		for _, opp := range hands[i].Opponents {
			if opp.PlayerID == "" {
				continue
			}
			if _, ok := seen[opp.PlayerID]; ok {
				continue
			}
			seen[opp.PlayerID] = struct{}{}
			ids = append(ids, opp.PlayerID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	profiles, err := h.players.GetMany(ctx, ids)
	if err != nil {
		return err
	}
	baseURL := h.avatarBaseURL()
	for i := range hands {
		for j := range hands[i].Opponents {
			opp := &hands[i].Opponents[j]
			profile, ok := profiles[opp.PlayerID]
			if !ok {
				opp.AvatarURL = ""
				continue
			}
			opp.AvatarURL = player.AvatarURL(&profile, baseURL)
			if profile.Name != "" {
				opp.Name = profile.Name
			}
		}
	}
	return nil
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

// achievementsSummary returns the caller's COMPLETE achievement state for one
// mode in a single response: every catalog entry (secret ones only once the
// player is past their first tier) with its progress, earned stars, next
// target and completion flag, plus catalog-wide roll-ups. Unlike
// achievementProgress it is not paginated — the catalog is bounded — so the
// client never derives stars, completion % or the secret-unlock gate from a
// truncated page. Backs the achievements page and the showcase picker (#71).
func (h *playerHandlers) achievementsSummary(c fiber.Ctx) error {
	userID := c.Locals(localsUserID).(string)
	mode := currencyModeParam(c)
	progress, err := h.achievements.AllAchievements(c.Context(), userID, mode)
	if err != nil {
		return problem.InternalServer("failed to load achievement summary", c, err).Send(c)
	}
	return c.JSON(achievements.BuildSummary(mode, progress))
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

	// Opponent identities/cards, storage keys and shuffle secrets are never
	// read in the first place: BestPublicHand projects the six public
	// attributes out of DynamoDB and returns nothing else (#225).
	var bestHand any
	if best, bestErr := h.sessions.BestPublicHand(c.Context(), playerID, roomstore.CurrencyModeSandbox); bestErr == nil && best != nil {
		bestHand = best
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
		"table_theme":             profile.EffectiveTableTheme(),
		"showcase_public":         profile.ShowcasePublic,
		"playstyle_public":        profile.PlaystylePublic,
		"table_public":            profile.TablePublic,
		"featured_achievements":   profile.FeaturedAchievements,
		"favorite_reactions":      profile.FavoriteReactions,
		"poker_terms_accepted":    profile.TermsAccepted(),
		"poker_terms_accepted_at": profile.TermsAcceptedAt,
	}
}
