//go:build integration

package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	goproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/cosmeticpurchase"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// TestWalletWebhookDispatchesCosmeticPurchaseByPrefix covers the "prdp" ->
// reactionSvc-then-cosmeticSvc fallback: a cosmetic (deck/felt) PIX purchase
// shares the same "prdp" wallet id prefix as premium reactions, so it must
// fall through reactionSvc's ErrCatalogMismatch into cosmeticSvc rather than
// being silently dropped (#69).
func TestWalletWebhookDispatchesCosmeticPurchaseByPrefix(t *testing.T) {
	db := webhookTestDynamoClient(t)
	env := fmt.Sprintf("webhook_cosmetic_test_%d", time.Now().UnixNano())
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_entitlements"))
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_purchases"))
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_cosmetic_entitlements"))
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_cosmetic_purchases"))

	// Both services independently re-verify the same purchase_id against
	// wallet (mirroring production, where both point at the same
	// ctech-wallet). reactionSvc's ReactionForSKU lookup on "poker_deck_golden"
	// fails with ErrCatalogMismatch, which is what the webhook handler uses to
	// fall through to cosmeticSvc.
	wallet := &fakeReactionWebhookWallet{
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-webhook-cos-1", UserID: "player-1", SKU: "poker_deck_golden", Amount: 500, Status: "confirmed"},
	}
	reactionSvc := reactionpurchase.NewService(wallet, reactionpurchase.NewEntitlementStore(db, env), reactionpurchase.NewStore(db, env))

	entitlements := cosmeticpurchase.NewEntitlementStore(db, env)
	store := cosmeticpurchase.NewStore(db, env)
	cosmeticSvc := cosmeticpurchase.NewService(wallet, entitlements, store)

	// Seed the pending record CreateReal would have written.
	if _, err := store.Create(context.Background(), cosmeticpurchase.Record{
		PlayerID: "player-1", PurchaseID: "prdp-webhook-cos-1", Kind: "deck", ItemID: "golden", Method: "pix",
		PriceCents: 500, PriceFichas: 500_000, Status: "pending",
		CreatedAt: "2026-08-13T00:00:00Z", UpdatedAt: "2026-08-13T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	sandboxSvc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	reg := ws.NewMemoryRegistry()
	conn := &recordingConn{}
	reg.Register("user#player-1", "conn-1", conn)

	app := fiber.New()
	RegisterWalletWebhook(app.Group("/v1.0"), "secret", sandboxSvc, reactionSvc, cosmeticSvc, reg)

	body, _ := json.Marshal(map[string]string{"purchase_id": "prdp-webhook-cos-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/webhooks/wallet", bytes.NewReader(body))
	req.Header.Set("X-Wallet-Signature", sign("secret", body))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if len(conn.messages) != 1 {
		t.Fatalf("expected exactly one broadcast message, got %d", len(conn.messages))
	}
	var msg pokerproto.ServerMessage
	if err := goproto.Unmarshal(conn.messages[0], &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != "cosmetic_purchase_update" || msg.PlayerId != "player-1" || msg.PurchaseId != "prdp-webhook-cos-1" || msg.Code != "confirmed" {
		t.Fatalf("unexpected message: %+v", &msg)
	}

	owned, err := cosmeticSvc.IsOwned(context.Background(), "player-1", cosmetics.KindDeck, "golden")
	if err != nil || !owned {
		t.Fatalf("expected entitlement granted after webhook confirm, owned=%v (err=%v)", owned, err)
	}
}
