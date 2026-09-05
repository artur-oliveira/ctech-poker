package v1

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
	goproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
)

const walletWebhookSignatureHeader = "X-Wallet-Signature"

// productPurchasePrefix is wallet's id prefix for a generic product purchase —
// the id space premium reactions and premium cosmetics share.
const productPurchasePrefix = "prdp"

// productPurchaseUpdate is the handful of fields the websocket frame needs,
// so the two product services' distinct Record types converge here without the
// webhook depending on either shape.
type productPurchaseUpdate struct{ playerID, purchaseID, status string }

// RegisterWalletWebhook mounts POST /v1.0/webhooks/wallet, unauthenticated by
// JWT — HMAC-SHA256 over the raw body against hmacSecret is the auth here,
// matching ctech-wallet's own outbound M2M webhook signing.
//
// Routing is two steps, never speculative: the purchase_id prefix picks the id
// space, and for "prdp" (generic product purchase, shared by premium reactions
// and premium cosmetics) the re-verified purchase's SKU picks the service.
// Anything else goes to the sandbox-credits svc
// (docs/specs/2026-08-12-premium-reactions.md).
func RegisterWalletWebhook(router fiber.Router, hmacSecret string, sandboxSvc *sandboxpurchase.Service, reactionSvc *reactionpurchase.Service, cosmeticSvc *cosmeticpurchase.Service, reg ws.Registry) {
	router.Post("/webhooks/wallet", walletWebhookHandler(hmacSecret, sandboxSvc, reactionSvc, cosmeticSvc, reg))
}

func walletWebhookHandler(hmacSecret string, sandboxSvc *sandboxpurchase.Service, reactionSvc *reactionpurchase.Service, cosmeticSvc *cosmeticpurchase.Service, reg ws.Registry) fiber.Handler {
	return func(c fiber.Ctx) error {
		body := c.Body()
		if !validWalletWebhookSignature(hmacSecret, body, c.Get(walletWebhookSignatureHeader)) {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		var payload struct {
			PurchaseID string `json:"purchase_id"`
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.PurchaseID == "" {
			return c.SendStatus(fiber.StatusBadRequest)
		}

		if strings.HasPrefix(payload.PurchaseID, productPurchasePrefix) {
			// One verification per delivery: the purchase is re-read from
			// wallet once and the SKU on it decides which product service
			// applies it. Reactions and cosmetics share the "prdp" id space,
			// and this used to try reactions first and re-verify the same id a
			// second time as a cosmetic on every cosmetic callback (#211).
			purchase, err := reactionSvc.VerifyPurchase(c.Context(), payload.PurchaseID)
			if err != nil {
				slog.Error("wallet webhook: product reverify failed", "purchase_id", payload.PurchaseID, "err", err)
				return c.SendStatus(fiber.StatusInternalServerError)
			}
			if purchase == nil || purchase.PurchaseID != payload.PurchaseID {
				slog.Error("wallet webhook: wallet returned a different product purchase", "purchase_id", payload.PurchaseID)
				return c.SendStatus(fiber.StatusInternalServerError)
			}

			var (
				record    productPurchaseUpdate
				changed   bool
				eventType string
			)
			switch {
			case reactionSvc.HandlesSKU(purchase.SKU):
				eventType = "reaction_purchase_update"
				rec, ch, syncErr := reactionSvc.SyncPurchase(c.Context(), purchase)
				record, changed, err = productPurchaseUpdate{rec.PlayerID, rec.PurchaseID, rec.Status}, ch, syncErr
			case cosmeticSvc.HandlesSKU(purchase.SKU):
				eventType = "cosmetic_purchase_update"
				rec, ch, syncErr := cosmeticSvc.SyncPurchase(c.Context(), purchase)
				record, changed, err = productPurchaseUpdate{rec.PlayerID, rec.PurchaseID, rec.Status}, ch, syncErr
			default:
				// Wallet knows a product SKU this build's catalogs do not.
				// Non-2xx so the delivery is retried after a deploy that adds
				// it, rather than silently dropped.
				slog.Error("wallet webhook: unknown product SKU", "purchase_id", payload.PurchaseID, "sku", purchase.SKU)
				return c.SendStatus(fiber.StatusInternalServerError)
			}
			if err != nil {
				slog.Error("wallet webhook: product sync failed", "purchase_id", payload.PurchaseID, "sku", purchase.SKU, "err", err)
				return c.SendStatus(fiber.StatusInternalServerError)
			}
			if changed {
				data, err := goproto.Marshal(&pokerproto.ServerMessage{
					Type:       eventType,
					PlayerId:   record.playerID,
					PurchaseId: record.purchaseID,
					Code:       record.status,
				})
				if err == nil {
					reg.Broadcast(c.Context(), "user#"+record.playerID, data)
				}
			}
			return c.SendStatus(fiber.StatusOK)
		}

		record, changed, err := sandboxSvc.ConfirmFromWebhook(c.Context(), payload.PurchaseID)
		if err != nil {
			// Non-2xx makes ctech-wallet retry via its own reconcile sweep.
			slog.Error("wallet webhook: reverify failed", "purchase_id", payload.PurchaseID, "err", err)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		if changed {
			data, err := goproto.Marshal(&pokerproto.ServerMessage{
				Type:       "sandbox_purchase_update",
				PlayerId:   record.PlayerID,
				PurchaseId: record.PurchaseID,
				Amount:     record.TotalCredits,
				Code:       record.Status,
			})
			if err == nil {
				reg.Broadcast(c.Context(), "user#"+record.PlayerID, data)
			}
		}
		return c.SendStatus(fiber.StatusOK)
	}
}

func validWalletWebhookSignature(secret string, body []byte, header string) bool {
	const prefix = "sha256="
	if secret == "" || !strings.HasPrefix(header, prefix) {
		return false
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sig, mac.Sum(nil))
}
