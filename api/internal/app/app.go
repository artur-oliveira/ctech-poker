// Package app wires the poker API using Fx dependency injection.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"go.uber.org/fx"
	"gopkg.aoctech.app/api-commons/awsconfig"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/achievements"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/leaderboard"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/problem"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/roulette"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// Module is the root Fx module for the poker API.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		newFiberApp,
		newCacheBackend,
		newVerifier,
		newWsRegistry,
		newTableLeaseService,
		newDynamoClient,
		newTableStore,
		newRoomStore,
		newPlayerStore,
		newPlayerService,
		newAchievementStore,
		newAchievementService,
		newLeaderboardStore,
		newLeaderboardService,
		newRouletteStore,
		newRouletteService,
		walletclient.New,
		newBuyinService,
		newTableManager,
	),
	fx.Invoke(registerRoutes),
	fx.Invoke(startServer),
)

func newFiberApp(cfg *config.Config) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      fmt.Sprintf("CTech Poker - %s - %s", cfg.Env, cfg.AppVersion),
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
		ProxyHeader:  fiber.HeaderXForwardedFor,
		TrustProxy:   len(cfg.TrustedProxies) > 0,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: cfg.TrustedProxies,
		},
		ErrorHandler: func(c fiber.Ctx, err error) error {
			if p, ok := errors.AsType[*problem.Problem](err); ok {
				return p.Send(c)
			}
			if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
				return problem.FromError(fiberErr).Send(c)
			}
			slog.Error("unhandled HTTP error", "request_id", requestid.FromContext(c), "path", c.Path(), "err", err)
			return problem.InternalServer("an unexpected error occurred").Send(c)
		},
	})

	app.Use(recover.New())
	// AllowCredentials requires explicit origins. Development intentionally
	// leaves origins empty, which means wildcard/no credentials like Wallet.
	corsCfg := cors.Config{
		AllowMethods:  []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:  []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		ExposeHeaders: []string{"X-Request-ID"},
		MaxAge:        3600,
	}
	if len(cfg.CorsAllowedOrigins) > 0 {
		corsCfg.AllowOrigins = cfg.CorsAllowedOrigins
		corsCfg.AllowCredentials = true
	}
	app.Use(cors.New(corsCfg))
	app.Use(requestid.New())
	app.Use(logger.New(logger.Config{
		Format: `{"time":"${time}","status":${status},"latency":"${latency}","method":"${method}","path":"${path}","request-id":"${request-id}"}` + "\n",
	}))
	return app
}

func newCacheBackend(cfg *config.Config) cache.Backend {
	if cfg.RedisURL == "" {
		return cache.NewMemoryBackend(1024)
	}
	rb, err := cache.NewRedisBackend(cfg.RedisURL)
	if err != nil {
		slog.Error("redis backend unavailable, falling back to in-memory (NOT fleet-shared)", "err", err)
		return cache.NewMemoryBackend(1024)
	}
	return rb
}

func newVerifier(c cache.Backend, cfg *config.Config) *jwtverify.Verifier {
	return jwtverify.NewVerifier(cfg.CtechJWKSURL, cfg.ServiceAudience, cfg.CtechURL, c)
}

func newWsRegistry(lc fx.Lifecycle, c cache.Backend) ws.Registry {
	rb, ok := c.(*cache.RedisBackend)
	if !ok {
		return ws.NewMemoryRegistry()
	}
	reg := ws.NewRedisRegistry(rb.Client())
	lc.Append(fx.Hook{OnStart: reg.Start, OnStop: reg.Stop})
	return reg
}

func newTableLeaseService(c cache.Backend) *tablelease.Service {
	return tablelease.NewService(c)
}

func newDynamoClient(cfg *config.Config) (*dynamodb.Client, error) {
	awsCfg, err := awsconfig.Load(context.Background(), cfg.AWSRegion)
	if err != nil {
		return nil, err
	}
	return awsconfig.NewDynamoDBClient(awsCfg, cfg.DynamoDBEndpoint), nil
}

func newTableStore(db *dynamodb.Client, cfg *config.Config) *tablestore.Store {
	return tablestore.NewStore(db, cfg.Env)
}
func newRoomStore(db *dynamodb.Client, cfg *config.Config) *roomstore.Store {
	return roomstore.NewStore(db, cfg.Env)
}
func newPlayerStore(db *dynamodb.Client, cfg *config.Config) *player.Store {
	return player.NewStore(db, cfg.Env)
}
func newPlayerService(store *player.Store) *player.Service { return player.NewService(store) }
func newAchievementStore(db *dynamodb.Client, cfg *config.Config) *achievements.Store {
	return achievements.NewStore(db, cfg.Env)
}
func newAchievementService(store *achievements.Store) *achievements.Service {
	return achievements.NewService(store)
}
func newLeaderboardStore(db *dynamodb.Client, cfg *config.Config) *leaderboard.Store {
	return leaderboard.NewStore(db, cfg.Env)
}
func newLeaderboardService(store *leaderboard.Store) *leaderboard.Service {
	return leaderboard.NewServiceWithStore(store)
}
func newRouletteStore(db *dynamodb.Client, cfg *config.Config) *roulette.Store {
	return roulette.NewStore(db, cfg.Env)
}
func newRouletteService(wallet *walletclient.Client, store *roulette.Store) *roulette.Service {
	return roulette.NewService(wallet, store)
}
func newBuyinService(wallet *walletclient.Client, manager *tablemanager.Manager, rooms *roomstore.Store, players *player.Service) *buyin.Service {
	return buyin.NewServiceWithPlayers(wallet, manager, rooms, players)
}

func newTableManager(leases *tablelease.Service, store *tablestore.Store, reg ws.Registry, achv *achievements.Service, leaderboardSvc *leaderboard.Service) *tablemanager.Manager {
	broadcast := func(tableID, viewerID string, snap hand.Snapshot) {
		data, _ := json.Marshal(map[string]any{"type": "state", "snapshot": snap})
		reg.Broadcast(context.Background(), tableID+"#"+viewerID, data)
	}
	onHandComplete := func(tableID string, outcome hand.HandOutcome) {
		ctx := context.Background()
		unlocks, err := achv.RecordHand(ctx, tableID, outcome)
		if err != nil {
			slog.Error("achievements record hand failed", "table", tableID, "err", err)
		}
		for _, unlock := range unlocks {
			data, _ := json.Marshal(map[string]any{"type": "achievement_unlocked", "key": unlock.Key, "stars": unlock.Stars})
			reg.Broadcast(ctx, tableID+"#"+unlock.PlayerID, data)
		}
		if err := leaderboardSvc.RecordUnlocks(ctx, unlocks); err != nil {
			slog.Error("leaderboard achievement points failed", "table", tableID, "err", err)
		}
		if err := leaderboardSvc.RecordHand(ctx, outcome); err != nil {
			slog.Error("leaderboard record hand failed", "table", tableID, "err", err)
		}
	}
	return tablemanager.NewManager(leases, store, broadcast, onHandComplete)
}

func roomBackedSeed(rooms *roomstore.Store) func(string) func() *hand.Table {
	return func(tableID string) func() *hand.Table {
		return func() *hand.Table {
			if rooms == nil {
				return hand.NewTable(nil, 10, 20)
			}
			room, err := rooms.Get(context.Background(), tableID)
			if err != nil || room == nil {
				return hand.NewTable(nil, 10, 20)
			}
			table := hand.NewTable(nil, room.SmallBlind, room.BigBlind)
			table.ConfigureRake(room.CurrencyMode)
			return table
		}
	}
}

func registerRoutes(app *fiber.App, cfg *config.Config, db *dynamodb.Client, verifier *jwtverify.Verifier, manager *tablemanager.Manager, reg ws.Registry, rooms *roomstore.Store, buyinSvc *buyin.Service, players *player.Service, leaderboardSvc *leaderboard.Service, rouletteSvc *roulette.Service) {
	v1.Register(app, cfg, db, verifier, manager, reg, roomBackedSeed(rooms), rooms, buyinSvc, players, leaderboardSvc, rouletteSvc)
}

func startServer(lc fx.Lifecycle, app *fiber.App, cfg *config.Config) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			addr := ":" + strconv.Itoa(cfg.Port)
			slog.Info("starting ctech-poker-api", "addr", addr, "env", cfg.Env)
			go func() {
				if err := app.Listen(addr); err != nil {
					slog.Error("server stopped", "err", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.Info("shutting down ctech-poker-api")
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return app.ShutdownWithContext(stopCtx)
		},
	})
}
