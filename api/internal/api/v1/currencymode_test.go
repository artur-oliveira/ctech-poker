package v1

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

func TestCurrencyModeParamDefaultsMissingAndInvalidToSandbox(t *testing.T) {
	for _, query := range []string{"", "?mode=invalid", "?mode=sandbox"} {
		app := fiber.New()
		app.Get("/", func(c fiber.Ctx) error { return c.SendString(currencyModeParam(c)) })
		response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/"+query, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		if string(body) != roomstore.CurrencyModeSandbox {
			t.Fatalf("query=%q mode=%q", query, body)
		}
	}
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error { return c.SendString(currencyModeParam(c)) })
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/?mode=real", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != roomstore.CurrencyModeReal {
		t.Fatalf("mode=%q", body)
	}
}
