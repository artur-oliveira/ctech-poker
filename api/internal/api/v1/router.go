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
	"gopkg.aoctech.app/poker/api/internal/billingclient"
	"gopkg.aoctech.app/poker/api/internal/botcheck"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/chatprefs"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/cosmeticloadout"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/handmeta"
	"gopkg.aoctech.app/poker/api/internal/handreveal"
	"gopkg.aoctech.app/poker/api/internal/handshare"
	"gopkg.aoctech.app/poker/api/internal/highlights"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/matchup"
	"gopkg.aoctech.app/poker/api/internal/oauthresource"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/playernotes"
	"gopkg.aoctech.app/poker/api/internal/pokerstats"
	"gopkg.aoctech.app/poker/api/internal/presence"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/recentplayers"
	"gopkg.aoctech.app/poker/api/internal/reconcile"
	"gopkg.aoctech.app/poker/api/internal/reports"
	"gopkg.aoctech.app/poker/api/internal/responsible"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/social"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/walletalert"
)

// Deps is everything Register wires into the routes (it replaced a 41-parameter
// signature). Seed builds a brand-new hand.Table the first time a table ID is
// acquired (see tablemanager.Manager.GetOrCreateActor). Nil members are the
// narrow construction seams of internal/app's tests.
type Deps struct {
	Cfg                  *config.Config
	DB                   *dynamodb.Client
	Verifier             *jwtverify.Verifier
	Manager              *tablemanager.Manager
	Reg                  ws.Registry
	Seed                 func(string) func() *hand.Table
	CacheBackend         cache.Backend
	Rooms                *roomstore.Store
	BuyinSvc             *buyin.Service
	Players              *player.Service
	LeaderboardSvc       *leaderboard.Service
	DailyRewardSvc       *dailyreward.Service
	TableStore           *tablestore.Store
	SessionStore         *sessionlog.Store
	AchievementStore     *achievements.Store
	PlayerNoteStore      *playernotes.Store
	HandMetaStore        *handmeta.Store
	HandShareStore       *handshare.Store
	HandRevealStore      *handreveal.Store
	HandRevealSvc        *handreveal.Service
	PokerStatsStore      *pokerstats.Store
	MatchupStore         *matchup.Store
	HighlightsStore      *highlights.Store
	Avatars              *avatar.Service
	SandboxPurchaseSvc   *sandboxpurchase.Service
	ReactionPurchaseSvc  *reactionpurchase.Service
	CosmeticPurchaseSvc  *cosmeticpurchase.Service
	SocialSvc            *social.Service
	PresenceSvc          *presence.Service
	RecentSvc            *recentplayers.Service
	ReportSvc            *reports.Service
	Pending              *reconcile.PendingStore
	WalletAlertSvc       *walletalert.Service
	ChatPrefsStore       *chatprefs.Store
	ChatPrefsCache       *chatprefs.ExtraWordsCache
	ResponsibleSvc       *responsible.Service
	Billing              *billingclient.Client
	BotCheckContestStore *botcheck.ContestStore
	CosmeticLoadoutSvc   *cosmeticloadout.Service
	Blocked              BlockedFunc
}

// Register mounts poker's routes under /v1.0. Any instance may accept any table's connection directly under
// ARCHITECTURE.md §2's revised model — there is no proxy route.
func Register(app *fiber.App, d Deps) {
	cfg := d.Cfg
	db := d.DB
	verifier := d.Verifier
	manager := d.Manager
	reg := d.Reg
	seed := d.Seed
	cacheBackend := d.CacheBackend
	rooms := d.Rooms
	buyinSvc := d.BuyinSvc
	players := d.Players
	leaderboardSvc := d.LeaderboardSvc
	dailyRewardSvc := d.DailyRewardSvc
	tableStore := d.TableStore
	sessionStore := d.SessionStore
	achievementStore := d.AchievementStore
	playerNoteStore := d.PlayerNoteStore
	handMetaStore := d.HandMetaStore
	handShareStore := d.HandShareStore
	handRevealStore := d.HandRevealStore
	handRevealSvc := d.HandRevealSvc
	pokerStatsStore := d.PokerStatsStore
	matchupStore := d.MatchupStore
	highlightsStore := d.HighlightsStore
	avatars := d.Avatars
	sandboxPurchaseSvc := d.SandboxPurchaseSvc
	reactionPurchaseSvc := d.ReactionPurchaseSvc
	cosmeticPurchaseSvc := d.CosmeticPurchaseSvc
	socialSvc := d.SocialSvc
	presenceSvc := d.PresenceSvc
	recentSvc := d.RecentSvc
	reportSvc := d.ReportSvc
	pending := d.Pending
	walletAlertSvc := d.WalletAlertSvc
	chatPrefsStore := d.ChatPrefsStore
	chatPrefsCache := d.ChatPrefsCache
	responsibleSvc := d.ResponsibleSvc
	billing := d.Billing
	botCheckContestStore := d.BotCheckContestStore
	cosmeticLoadoutSvc := d.CosmeticLoadoutSvc
	blocked := d.Blocked
	oauthresource.Register(app, cfg.ServiceAudience, cfg.CtechIssuerURL)
	router := app.Group("/v1.0")

	// Health (unauthenticated): /v1.0/health is a dependency-free liveness probe;
	// /v1.0/health-check is the detailed dependency report the ALB target group
	// probes (it accepts 200 and 207).
	RegisterHealth(router, cfg, db)

	// Feature manifest (unauthenticated, deployment-wide — see capabilities.go).
	RegisterCapabilities(router, cfg)

	// The table WebSocket's per-player limits go through the same Redis
	// counter as the HTTP ones — a player spread across instances, or
	// reconnecting, used to get one fresh in-memory budget per connection
	// (#43). 10 actions/sec/player is generous for a human, tight for a
	// script; reactions stay at one per 2 s.
	limiterCounter := newLimiterCounter(cacheBackend)
	wsActionLimiter := newLimiter(limiterCounter, "ws-action", 10, time.Second)
	wsReactionLimiter := newLimiter(limiterCounter, "ws-reaction", 1, 2*time.Second)
	RegisterTableWS(router, verifier, manager, reg, cfg.CorsAllowedOrigins, seed, rooms, cfg, players, pokerStatsStore, wsActionLimiter, wsReactionLimiter, blocked)
	RegisterGeneralWS(router, verifier, reg, cfg.CorsAllowedOrigins, presenceSvc, blocked)
	auth := authMiddleware(verifier, blocked)
	idem := newIdempotency(cfg, db)
	RegisterHandHistory(router, auth, &tablestoreAdapter{store: tableStore})
	RegisterAchievementCatalog(router)
	RegisterWalletWebhook(router, cfg.WalletWebhookHMACSecret, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, reg, blocked)
	if billing != nil {
		RegisterBillingWebhook(router, cfg.BillingEventSecret, billing, sandboxPurchaseSvc, reactionPurchaseSvc, cosmeticPurchaseSvc, reg, blocked)
	}

	// Fixed-window rate limits on the mutating endpoints (M6/S2). Keyed per
	// caller IP; Redis (mandatory in prod, T2) makes the counter fleet-wide.
	createLimiter := newLimiter(limiterCounter, "create", 10, time.Minute)
	joinLimiter := newLimiter(limiterCounter, "join", 30, time.Minute)
	spinLimiter := newLimiter(limiterCounter, "spin", 60, time.Minute)
	avatarLimiter := newLimiter(limiterCounter, "avatar", 5, time.Hour)
	// The public avatar READ path is limited separately and far more loosely:
	// avatarLimiter's 5/hour guards uploads and reports, while a single table
	// view legitimately fetches up to nine images. Keyed per IP, not per
	// player — the route is unauthenticated.
	avatarReadLimiter := newLimiter(limiterCounter, "avatar-read", 600, time.Minute)
	// GET /leaderboard is public (see RegisterLeaderboard), so an IP limiter
	// is the only thing standing between it and an unauthenticated scraper.
	leaderboardReadLimiter := newLimiter(limiterCounter, "leaderboard-read", 120, time.Minute)
	purchaseLimiter := newLimiter(limiterCounter, "purchase", 10, time.Minute)
	socialMutationPlayerLimiter := newLimiter(limiterCounter, "social-mutation-player", 120, time.Minute)
	socialMutationIPLimiter := newLimiter(limiterCounter, "social-mutation-ip", 240, time.Minute)
	friendRequestPlayerLimiter := newLimiter(limiterCounter, "friend-request-player", 30, 24*time.Hour)
	friendRequestIPLimiter := newLimiter(limiterCounter, "friend-request-ip", 100, 24*time.Hour)
	inviteSenderLimiter := newLimiter(limiterCounter, "invite-sender", 20, time.Minute)
	inviteRecipientLimiter := newLimiter(limiterCounter, "invite-recipient", 5, time.Minute)
	reportPlayerLimiter := newLimiter(limiterCounter, "report-player", 10, time.Hour)
	reportIPLimiter := newLimiter(limiterCounter, "report-ip", 50, time.Hour)

	// Unauthenticated, unlike every Register* call below it.
	RegisterAvatars(router, avatars, avatarReadLimiter)
	RegisterRooms(router, auth, idem, rooms, buyinSvc, manager, reg, cfg, sessionStore, createLimiter, joinLimiter)
	identityPusher := &tableIdentityPusher{
		manager: manager, seed: seed, presence: presenceSvc,
		players: players, stats: pokerStatsStore, cfg: cfg,
	}
	RegisterPlayers(router, auth, players, sessionStore, achievementStore, cfg, avatars, avatarLimiter, pokerStatsStore, reportSvc, identityPusher, leaderboardSvc, pending, walletAlertSvc)
	RegisterReactionWheel(router, auth, players)
	RegisterPlayerNotes(router, auth, playerNoteStore)
	RegisterChatPrefs(router, auth, chatPrefsStore, chatPrefsCache)
	RegisterResponsible(router, auth, responsibleSvc)
	RegisterBotCheckContest(router, auth, botCheckContestStore)
	RegisterHandMeta(router, auth, handMetaStore)
	RegisterHandShares(router, auth, sessionStore, tableStore, handShareStore)
	RegisterHandReveal(router, auth, idem, sessionStore, handRevealStore, handRevealSvc, purchaseLimiter)
	RegisterHighlights(router, auth, sessionStore, highlightsStore)
	RegisterPokerStats(router, auth, pokerStatsStore, players)
	RegisterMatchups(router, auth, matchupStore)
	RegisterLeaderboard(router, auth, leaderboardSvc, players, leaderboardReadLimiter)
	RegisterDailyReward(router, auth, idem, dailyRewardSvc, spinLimiter)
	RegisterSandboxPurchase(router, auth, idem, sandboxPurchaseSvc, purchaseLimiter)
	RegisterReactionPurchase(router, auth, idem, reactionPurchaseSvc, purchaseLimiter)
	RegisterCosmeticPurchase(router, auth, idem, cosmeticPurchaseSvc, purchaseLimiter)
	RegisterAvatarCosmeticsOwnership(router, auth, cosmeticPurchaseSvc)
	// nil in the narrower test-only wiring (app.registerRoutes).
	if cosmeticLoadoutSvc != nil {
		RegisterCosmeticLoadouts(router, auth, cosmeticLoadoutSvc)
	}
	RegisterSocial(router, auth, socialSvc, players, cfg, SocialLimiters{
		MutationPlayer:  socialMutationPlayerLimiter,
		MutationIP:      socialMutationIPLimiter,
		RequestPlayer:   friendRequestPlayerLimiter,
		RequestIP:       friendRequestIPLimiter,
		InviteSender:    inviteSenderLimiter,
		InviteRecipient: inviteRecipientLimiter,
	}, presenceSvc, recentSvc, rooms)
	RegisterReports(router, auth, reportSvc, cfg, ReportLimiters{Player: reportPlayerLimiter, IP: reportIPLimiter})
}
