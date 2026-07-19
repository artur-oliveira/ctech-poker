package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// Register mounts poker's routes under /v1.0.
func Register(app *fiber.App, cfg *config.Config) {
	router := app.Group("/v1.0")

	// Health (unauthenticated): /v1.0/health is a dependency-free liveness probe;
	// /v1.0/health-check is the detailed dependency report the ALB target group
	// probes (it accepts 200 and 207).
	RegisterHealth(router, cfg)
}
