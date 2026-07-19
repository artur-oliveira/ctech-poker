// Package app wires the poker API using Fx dependency injection.
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/fx"
	"gopkg.aoctech.app/api-commons/awsconfig"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
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
		newTableStore,
		newTableManager,
	),
	fx.Invoke(registerRoutes),
	fx.Invoke(startServer),
)

func newFiberApp() *fiber.App {
	return fiber.New()
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

func newTableStore(cfg *config.Config) (*tablestore.Store, error) {
	awsCfg, err := awsconfig.Load(context.Background(), cfg.AWSRegion)
	if err != nil {
		return nil, err
	}
	db := awsconfig.NewDynamoDBClient(awsCfg, cfg.DynamoDBEndpoint)
	return tablestore.NewStore(db, cfg.Env), nil
}

func newTableManager(leases *tablelease.Service, store *tablestore.Store, reg ws.Registry) *tablemanager.Manager {
	broadcast := func(tableID, viewerID string, snap hand.Snapshot) {
		data, _ := json.Marshal(map[string]any{"type": "state", "snapshot": snap})
		reg.Broadcast(context.Background(), tableID+"#"+viewerID, data)
	}
	return tablemanager.NewManager(leases, store, broadcast)
}

// defaultSeed is a placeholder until Phase 3's room service supplies real
// stakes/seats — a heads-up-capacity table so the WS gateway is testable
// standalone. Phase 3 replaces this provider entirely.
func defaultSeed(tableID string) func() *hand.Table {
	return func() *hand.Table {
		return hand.NewTable(nil, 10, 20)
	}
}

func registerRoutes(app *fiber.App, cfg *config.Config, verifier *jwtverify.Verifier, manager *tablemanager.Manager, reg ws.Registry) {
	v1.Register(app, cfg, verifier, manager, reg, defaultSeed)
}

func startServer(lc fx.Lifecycle, app *fiber.App, cfg *config.Config) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			addr := ":" + strconv.Itoa(cfg.Port)
			go func() {
				if err := app.Listen(addr); err != nil {
					slog.Error("server stopped", "err", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return app.ShutdownWithContext(stopCtx)
		},
	})
}
