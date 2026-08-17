package v1

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	"gopkg.aoctech.app/poker/api/internal/avatar"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/handshare"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/oauthresource"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/playernotes"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/presence"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/recentplayers"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/social"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// Register mounts poker's routes under /v1.0. seed builds a brand-new
// hand.Table the first time a given table ID is ever acquired (see
// tablemanager.Manager.GetOrCreateActor) — passed straight through to the WS
// gateway. Any instance may accept any table's connection directly under
// ARCHITECTURE.md §2's revised model — there is no proxy route.
func Register(
	app *fiber.App,
	cfg *config.Config,
	db *dynamodb.Client,
	verifier *jwtverify.Verifier,
	manager *tablemanager.Manager,
	reg ws.Registry,
	seed func(string) func() *hand.Table,
	cacheBackend cache.Backend,
	rooms *roomstore.Store,
	buyinSvc *buyin.Service,
	players *player.Service,
	leaderboardSvc *leaderboard.Service,
	dailyRewardSvc *dailyreward.Service,
	tableStore *tablestore.Store,
	sessionStore *sessionlog.Store,
	achievementStore *achievements.Store,
	playerNoteStore *playernotes.Store,
	handShareStore *handshare.Store,
	pokerStatsStore *pokerstats.Store,
	avatars *avatar.Service,
	sandboxPurchaseSvc *sandboxpurchase.Service,
	reactionPurchaseSvc *reactionpurchase.Service,
	socialSvc *social.Service,
	presenceSvc *presence.Service,
	recentSvc *recentplayers.Service,
) {
	oauthresource.Register(app, cfg.ServiceAudience, cfg.CtechIssuerURL)
	router := app.Group("/v1.0")

	// Health (unauthenticated): /v1.0/health is a dependency-free liveness probe;
	// /v1.0/health-check is the detailed dependency report the ALB target group
	// probes (it accepts 200 and 207).
	RegisterHealth(router, cfg, db)

	RegisterTableWS(router, verifier, manager, reg, cfg.CorsAllowedOrigins, seed, rooms, cfg, players, pokerStatsStore)
	RegisterGeneralWS(router, verifier, reg, cfg.CorsAllowedOrigins, presenceSvc)
	auth := authMiddleware(verifier)
	RegisterHandHistory(router, auth, &tablestoreAdapter{store: tableStore})
	RegisterAchievementCatalog(router)
	RegisterWalletWebhook(router, cfg.WalletWebhookHMACSecret, sandboxPurchaseSvc, reactionPurchaseSvc, reg)

	// Fixed-window rate limits on the mutating endpoints (M6/S2). Keyed per
	// caller IP; Redis (mandatory in prod, T2) makes the counter fleet-wide.
	createLimiter := NewRateLimiter(cacheBackend, 10, time.Minute)
	joinLimiter := NewRateLimiter(cacheBackend, 30, time.Minute)
	spinLimiter := NewRateLimiter(cacheBackend, 60, time.Minute)
	avatarLimiter := NewRateLimiter(cacheBackend, 5, time.Hour)
	purchaseLimiter := NewRateLimiter(cacheBackend, 10, time.Minute)
	socialMutationPlayerLimiter := NewRateLimiter(cacheBackend, 120, time.Minute)
	socialMutationIPLimiter := NewRateLimiter(cacheBackend, 240, time.Minute)
	friendRequestPlayerLimiter := NewRateLimiter(cacheBackend, 30, 24*time.Hour)
	friendRequestIPLimiter := NewRateLimiter(cacheBackend, 100, 24*time.Hour)
	inviteSenderLimiter := NewRateLimiter(cacheBackend, 20, time.Minute)
	inviteRecipientLimiter := NewRateLimiter(cacheBackend, 5, time.Minute)

	RegisterRooms(router, auth, rooms, buyinSvc, manager, reg, cfg, createLimiter, joinLimiter)
	RegisterPlayers(router, auth, players, sessionStore, achievementStore, cfg, avatars, avatarLimiter, pokerStatsStore)
	RegisterPlayerNotes(router, auth, playerNoteStore)
	RegisterHandShares(router, auth, sessionStore, tableStore, handShareStore)
	RegisterPokerStats(router, auth, pokerStatsStore)
	RegisterLeaderboard(router, auth, leaderboardSvc)
	RegisterDailyReward(router, auth, dailyRewardSvc, spinLimiter)
	RegisterSandboxPurchase(router, auth, sandboxPurchaseSvc, purchaseLimiter)
	RegisterReactionPurchase(router, auth, reactionPurchaseSvc, purchaseLimiter)
	RegisterSocial(router, auth, socialSvc, players, cfg, SocialLimiters{
		MutationPlayer:  socialMutationPlayerLimiter,
		MutationIP:      socialMutationIPLimiter,
		RequestPlayer:   friendRequestPlayerLimiter,
		RequestIP:       friendRequestIPLimiter,
		InviteSender:    inviteSenderLimiter,
		InviteRecipient: inviteRecipientLimiter,
	}, presenceSvc, recentSvc)
}
