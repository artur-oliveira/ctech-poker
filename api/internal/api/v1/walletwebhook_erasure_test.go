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
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/ws"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// A PIX product purchase confirmed after the buyer was erased must not
// re-create their rows (saga protocol §4.5, plan ruling R15).
func TestWalletWebhookDropsPurchaseOfErasedUser(t *testing.T) {
	db := webhookTestDynamoClient(t)
	env := fmt.Sprintf("webhook_erasure_test_%d", time.Now().UnixNano())
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_entitlements"))
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_purchases"))
	entitlements := reactionpurchase.NewEntitlementStore(db, env)
	wallet := &fakeReactionWebhookWallet{
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-erased-1", UserID: "player-1", SKU: "poker_reaction_cold", Amount: 100, Status: "confirmed"},
	}
	svc := reactionpurchase.NewService(wallet, entitlements, reactionpurchase.NewStore(db, env))
	sandboxSvc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	reg := ws.NewMemoryRegistry()
	conn := &recordingConn{}
	reg.Register("user#player-1", "conn-1", conn)
	blocked := func(_ context.Context, sub string) (bool, error) { return sub == "player-1", nil }

	app := fiber.New()
	RegisterWalletWebhook(app.Group("/v1.0"), "secret", sandboxSvc, svc, noopCosmeticSvc(), reg, blocked)
	body, _ := json.Marshal(map[string]string{"purchase_id": "prdp-erased-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/webhooks/wallet", bytes.NewReader(body))
	req.Header.Set("X-Wallet-Signature", sign("secret", body))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (drop, do not make the wallet retry), got %d", resp.StatusCode)
	}
	if len(conn.messages) != 0 {
		t.Fatalf("an erased user must not be notified, got %d messages", len(conn.messages))
	}
	got, err := entitlements.Get(context.Background(), "player-1", "cold")
	if err != nil || got != nil {
		t.Fatalf("erased user's entitlement re-granted: %+v (err=%v)", got, err)
	}
}
