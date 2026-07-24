package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
)

func TestFiberServerUsesConfiguredTimeoutsAndTrustedProxies(t *testing.T) {
	app := newFiberApp(&config.Config{ReadTimeout: 3, WriteTimeout: 4, IdleTimeout: 5, TrustedProxies: []string{"10.0.0.0/16"}})
	cfg := app.Config()
	if cfg.ReadTimeout != 3*time.Second || cfg.WriteTimeout != 4*time.Second || cfg.IdleTimeout != 5*time.Second {
		t.Fatalf("timeouts read=%v write=%v idle=%v", cfg.ReadTimeout, cfg.WriteTimeout, cfg.IdleTimeout)
	}
	if !cfg.TrustProxy || len(cfg.TrustProxyConfig.Proxies) != 1 || cfg.ProxyHeader != fiber.HeaderXForwardedFor {
		t.Fatalf("proxy config=%+v", cfg.TrustProxyConfig)
	}
}

func TestFiberServerAppliesCredentialedCorsForConfiguredOrigin(t *testing.T) {
	app := newFiberApp(&config.Config{ReadTimeout: 10, WriteTimeout: 10, IdleTimeout: 60, CorsAllowedOrigins: []string{"https://poker.aoctech.app"}})
	app.Get("/probe", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	req, _ := http.NewRequest(http.MethodOptions, "/probe", nil)
	req.Header.Set("Origin", "https://poker.aoctech.app")
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "https://poker.aoctech.app" || resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("cors headers=%v", resp.Header)
	}
}
