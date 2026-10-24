package v1

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
)

// The detailed probe must 503 without the primary store, and liveness must not
// depend on it (HAProxy/nginx/UI probe /v1.0/health).
func TestHealthPaths(t *testing.T) {
	app := fiber.New()
	RegisterHealth(app.Group("/v1.0"), &config.Config{AppVersion: "1.2.3"}, nil)
	for path, want := range map[string]int{"/v1.0/health": 200, "/v1.0/health-check": 503} {
		resp, err := app.Test(mustGet(path))
		if err != nil || resp.StatusCode != want {
			t.Fatalf("%s: err=%v code=%v want %d", path, err, resp, want)
		}
	}
}

func mustGet(path string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, path, nil)
	return r
}
