package v1

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/sessionlog"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

// Register mounts poker's routes under /v1.0. seed builds a brand-new
// hand.Table the first time a given table ID is ever acquired (see
// tablemanager.Manager.GetOrCreateActor) — passed straight through to the WS
// gateway. Any instance may accept any table's connection directly under
// ARCHITECTURE.md §2's revised model — there is no proxy route.
func Register(app *fiber.App, cfg *config.Config, db *dynamodb.Client, verifier *jwtverify.Verifier, manager *tablemanager.Manager, reg ws.Registry, seed func(string) func() *hand.Table, rooms *roomstore.Store, buyinSvc *buyin.Service, players *player.Service, leaderboardSvc *leaderboard.Service, dailyRewardSvc *dailyreward.Service, cacheBackend cache.Backend, tableStore *tablestore.Store, sessionStore *sessionlog.Store) {
	router := app.Group("/v1.0")

	// Health (unauthenticated): /v1.0/health is a dependency-free liveness probe;
	// /v1.0/health-check is the detailed dependency report the ALB target group
	// probes (it accepts 200 and 207).
	RegisterHealth(router, cfg, db)

	RegisterTableWS(router, verifier, manager, reg, cfg.CorsAllowedOrigins, seed, rooms, cfg)
	auth := authMiddleware(verifier)
	if tableStore != nil {
		RegisterHandHistory(router, auth, &tablestoreAdapter{store: tableStore})
	}
	if sessionStore != nil {
		RegisterPlayerHistory(router, auth, sessionStore)
	}

	// Fixed-window rate limits on the mutating endpoints (M6/S2). Keyed per
	// caller IP; Redis (mandatory in prod, T2) makes the counter fleet-wide.
	createLimiter := NewRateLimiter(cacheBackend, 10, time.Minute)
	joinLimiter := NewRateLimiter(cacheBackend, 30, time.Minute)
	spinLimiter := NewRateLimiter(cacheBackend, 60, time.Minute)

	RegisterRooms(router, auth, rooms, buyinSvc, manager, createLimiter, joinLimiter)
	RegisterPlayers(router, auth, players)
	RegisterLeaderboard(router, auth, leaderboardSvc)
	RegisterDailyReward(router, auth, dailyRewardSvc, spinLimiter)
}
