package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/jwtverify"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

// Register mounts poker's routes under /v1.0. seed builds a brand-new
// hand.Table the first time a given table ID is ever acquired (see
// tablemanager.Manager.GetOrCreateActor) — passed straight through to the WS
// gateway. Any instance may accept any table's connection directly under
// ARCHITECTURE.md §2's revised model — there is no proxy route.
func Register(app *fiber.App, cfg *config.Config, verifier *jwtverify.Verifier, manager *tablemanager.Manager, reg ws.Registry, seed func(string) func() *hand.Table) {
	router := app.Group("/v1.0")

	// Health (unauthenticated): /v1.0/health is a dependency-free liveness probe;
	// /v1.0/health-check is the detailed dependency report the ALB target group
	// probes (it accepts 200 and 207).
	RegisterHealth(router, cfg)

	RegisterTableWS(router, verifier, manager, reg, cfg.CorsAllowedOrigins, seed)
}
