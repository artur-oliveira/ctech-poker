package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// capabilitiesSchemaVersion is bumped whenever a field is removed or its
// meaning changes — adding a flag is backwards compatible and does not bump
// it. Same contract oauthresource.Manifest.SchemaVersion already carries.
const capabilitiesSchemaVersion = 1

// capabilities is the runtime feature manifest: what this deployment has
// turned on, so a client can hide a surface instead of discovering it is off
// by attempting the operation and parsing the error (issue #312).
//
// Every field is derived from *config.Config and is identical for every
// caller, which is why the route is unauthenticated and needs no store
// access. Player-dependent capabilities (cohorts, entitlements) deliberately
// do NOT belong here: they would make the response per-player and turn a
// constant into a fan-out. They belong on the profile/entitlement endpoints
// that already authenticate.
type capabilities struct {
	SchemaVersion      int  `json:"schema_version"`
	RealMoneyEnabled   bool `json:"real_money_enabled"`
	SocialGraphEnabled bool `json:"social_graph_enabled"`
	BotCheckEnabled    bool `json:"bot_check_enabled"`
}

// RegisterCapabilities mounts GET /v1.0/capabilities, unauthenticated for the
// same reason RegisterHealth is: it reports deployment configuration, never
// anything about the caller.
func RegisterCapabilities(router fiber.Router, cfg *config.Config) {
	router.Get("/capabilities", func(c fiber.Ctx) error {
		return c.JSON(capabilities{
			SchemaVersion:      capabilitiesSchemaVersion,
			RealMoneyEnabled:   cfg.RealMoneyEnabled,
			SocialGraphEnabled: cfg.SocialGraphEnabled,
			// Turnstile is disabled when no secret is configured
			// (config.Config.TurnstileSecret) — the client must not render a
			// widget whose verification the server will skip.
			BotCheckEnabled: cfg.TurnstileSecret != "",
		})
	})
}
