package app

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestLivenessEndpointReturnsOK(t *testing.T) {
	app := fiber.New()
	registerRoutes(app, &config.Config{AppVersion: "1.2.3"})

	req, _ := http.NewRequest(http.MethodGet, "/v1.0/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status    string `json:"status"`
		ReleaseID string `json:"releaseId"`
		ServiceID string `json:"serviceId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "pass" {
		t.Fatalf("expected status pass, got %q", body.Status)
	}
	if body.ReleaseID != "1.2.3" {
		t.Fatalf("expected releaseId 1.2.3, got %q", body.ReleaseID)
	}
	if body.ServiceID == "" {
		t.Fatal("expected non-empty serviceId")
	}
}

func TestHealthCheckEndpointReturnsDetailedStatus(t *testing.T) {
	app := fiber.New()
	registerRoutes(app, &config.Config{AppVersion: "1.2.3"})

	req, _ := http.NewRequest(http.MethodGet, "/v1.0/health-check", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	// 200 (pass) unless cpu/memory happen to be over 90% on the test runner,
	// in which case the same check degrades to 207 (warn) — either is a
	// correct response, not a bug.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != 207 {
		t.Fatalf("expected 200 or 207, got %d", resp.StatusCode)
	}

	var body struct {
		Status      string                 `json:"status"`
		Version     string                 `json:"version"`
		ReleaseID   string                 `json:"releaseId"`
		ServiceID   string                 `json:"serviceId"`
		Description string                 `json:"description"`
		Checks      map[string]interface{} `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Version != "/v1.0" {
		t.Fatalf("expected version /v1.0, got %q", body.Version)
	}
	if body.ReleaseID != "1.2.3" {
		t.Fatalf("expected releaseId 1.2.3, got %q", body.ReleaseID)
	}
	for _, key := range []string{"uptime", "cpu", "memory"} {
		if _, ok := body.Checks[key]; !ok {
			t.Fatalf("expected checks to contain %q, got %+v", key, body.Checks)
		}
	}
}
