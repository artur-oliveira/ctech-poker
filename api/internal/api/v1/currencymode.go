package v1

import (
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

// currencyModeParam keeps older clients on the historical sandbox dataset.
// Invalid values intentionally follow the same fallback contract.
func currencyModeParam(c fiber.Ctx) string {
	if c.Query("mode") == roomstore.CurrencyModeReal {
		return roomstore.CurrencyModeReal
	}
	return roomstore.CurrencyModeSandbox
}
