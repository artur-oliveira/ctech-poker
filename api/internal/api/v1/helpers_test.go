package v1

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestLimitParamCapsThePageSize(t *testing.T) {
	cases := []struct {
		query string
		want  int
	}{
		{"", 50},
		{"?limit=10", 10},
		{"?limit=0", 50},
		{"?limit=abc", 50},
		{"?limit=100000", maxLimitParam},
	}
	for _, tc := range cases {
		app := fiber.New()
		var got int
		app.Get("/", func(c fiber.Ctx) error {
			got = limitParam(c)
			return c.SendStatus(fiber.StatusOK)
		})
		if _, err := app.Test(httptest.NewRequest("GET", "/"+tc.query, nil)); err != nil {
			t.Fatalf("request %q: %v", tc.query, err)
		}
		if got != tc.want {
			t.Errorf("limitParam(%q) = %d, want %d", tc.query, got, tc.want)
		}
	}
}
