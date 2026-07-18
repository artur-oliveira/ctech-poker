// Package app wires the poker API using Fx dependency injection.
package app

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/fx"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// Module is the root Fx module for the poker API.
var Module = fx.Options(
	fx.Provide(
		config.Load,
		newFiberApp,
	),
	fx.Invoke(registerRoutes),
	fx.Invoke(startServer),
)

func newFiberApp() *fiber.App {
	return fiber.New()
}

func registerRoutes(app *fiber.App) {
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
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
