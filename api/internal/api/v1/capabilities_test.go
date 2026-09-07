package v1

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestCapabilitiesReportsConfiguredFlags(t *testing.T) {
	app := fiber.New()
	RegisterCapabilities(app.Group("/v1.0"), &config.Config{
		RealMoneyEnabled: true,
		LegalSignoffRef:  "ref",
		TurnstileSecret:  "secret",
	})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/capabilities", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got capabilities
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SchemaVersion != capabilitiesSchemaVersion {
		t.Fatalf("schema_version=%d", got.SchemaVersion)
	}
	if !got.RealMoneyEnabled || got.SocialGraphEnabled || !got.BotCheckEnabled {
		t.Fatalf("flags=%+v", got)
	}
}

// An unconfigured Turnstile secret must report bot checks as off: the server
// skips verification in that case, so a client rendering the widget anyway
// would block players on a check nobody enforces.
func TestCapabilitiesBotCheckOffWithoutTurnstileSecret(t *testing.T) {
	app := fiber.New()
	RegisterCapabilities(app.Group("/v1.0"), &config.Config{})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/v1.0/capabilities", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var got capabilities
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.BotCheckEnabled {
		t.Fatal("bot_check_enabled must be false with no Turnstile secret")
	}
}
