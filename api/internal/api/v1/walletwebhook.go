package v1

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// RegisterWalletWebhook mounts POST /v1.0/webhooks/wallet, unauthenticated by
// JWT — HMAC-SHA256 over the raw body against hmacSecret is the auth here,
// matching ctech-wallet's own outbound M2M webhook signing. purchase_id
// prefix routes the callback: "prdp" (generic product purchase, shared by
// both premium reactions and premium cosmetics) tries reactionSvc first,
// falling back to cosmeticSvc when the SKU isn't a reaction one
// (reactionpurchase.ErrCatalogMismatch); anything else goes to the
// sandbox-credits svc (docs/specs/2026-08-12-premium-reactions.md).
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

		if strings.HasPrefix(payload.PurchaseID, "prdp") {
			record, changed, err := reactionSvc.ConfirmFromWebhook(c.Context(), payload.PurchaseID)
			if err != nil {
				if !errors.Is(err, reactionpurchase.ErrCatalogMismatch) {
					slog.Error("wallet webhook: reaction reverify failed", "purchase_id", payload.PurchaseID, "err", err)
					return c.SendStatus(fiber.StatusInternalServerError)
				}
				// Not a reaction SKU: this "prdp" id belongs to a cosmetic
				// (deck/felt) purchase instead.
				cosRecord, cosChanged, cosErr := cosmeticSvc.ConfirmFromWebhook(c.Context(), payload.PurchaseID)
				if cosErr != nil {
					slog.Error("wallet webhook: cosmetic reverify failed", "purchase_id", payload.PurchaseID, "err", cosErr)
					return c.SendStatus(fiber.StatusInternalServerError)
				}
				if cosChanged {
					data, err := goproto.Marshal(&pokerproto.ServerMessage{
						Type:       "cosmetic_purchase_update",
						PlayerId:   cosRecord.PlayerID,
						PurchaseId: cosRecord.PurchaseID,
						Code:       cosRecord.Status,
					})
					if err == nil {
						reg.Broadcast(c.Context(), "user#"+cosRecord.PlayerID, data)
					}
				}
				return c.SendStatus(fiber.StatusOK)
			}
			if changed {
				data, err := goproto.Marshal(&pokerproto.ServerMessage{
					Type:       "reaction_purchase_update",
					PlayerId:   record.PlayerID,
					PurchaseId: record.PurchaseID,
					Code:       record.Status,
				})
				if err == nil {
					reg.Broadcast(c.Context(), "user#"+record.PlayerID, data)
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
