package v1

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/ws"

	"gopkg.aoctech.app/poker/api/internal/billingclient"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/purchasegw"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
)

// PathBillingWebhook is where Billing delivers Poker's invoice events.
const PathBillingWebhook = "/webhooks/billing"

// billingEventSkew is how far a delivery's timestamp may be from now; the HMAC over
// timestamp and body is the replay protection.
const billingEventSkew = 5 * time.Minute

// invoiceReader reads the invoice an event names.
type invoiceReader interface {
	InvoiceByID(ctx context.Context, id string) (*billingclient.Invoice, error)
}

// RegisterBillingWebhook mounts POST /v1.0/webhooks/billing: Billing's signed invoice.paid
// for the chip packs and goods Poker sells. Unauthenticated by JWT; the HMAC is the
// credential. The event is a wake-up and nothing more: the invoice it names is read back
// from Billing, and the purchase is re-verified from the invoice exactly as a poll would
// (amount against the catalogue included) before any entitlement or credit follows.
func RegisterBillingWebhook(router fiber.Router, secret string, invoices invoiceReader, sandboxSvc *sandboxpurchase.Service, reactionSvc *reactionpurchase.Service, cosmeticSvc *cosmeticpurchase.Service, reg ws.Registry, blocked BlockedFunc) {
	router.Post(PathBillingWebhook, func(c fiber.Ctx) error {
		body := c.Body()
		if !billingclient.VerifySignature(secret, c.Get(billingclient.HeaderSignature), c.Get(billingclient.HeaderTimestamp), body, time.Now(), billingEventSkew) {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		var ev struct {
			Type string `json:"type"`
			Data struct {
				Object string `json:"object"`
				ID     string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &ev); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		// Anything but a paid invoice is not ours to act on; 200 so Billing does not retry it.
		if ev.Type != "invoice.paid" || ev.Data.Object != "invoice" || ev.Data.ID == "" {
			return c.SendStatus(fiber.StatusOK)
		}
		inv, err := invoices.InvoiceByID(c.Context(), ev.Data.ID)
		switch {
		case errors.Is(err, billingclient.ErrNotFound):
			return c.SendStatus(fiber.StatusOK)
		case err != nil:
			slog.Warn("billing webhook: invoice unreadable, Billing will retry", "invoice_id", ev.Data.ID, "err", err)
			return c.SendStatus(fiber.StatusBadGateway)
		}
		if !strings.HasPrefix(inv.Reference, purchasegw.SandboxPrefix) && !strings.HasPrefix(inv.Reference, purchasegw.ProductPrefix) {
			return c.SendStatus(fiber.StatusOK) // an invoice of another service
		}
		return applyPurchaseEvent(c, inv.Reference, sandboxSvc, reactionSvc, cosmeticSvc, reg, blocked)
	})
}
