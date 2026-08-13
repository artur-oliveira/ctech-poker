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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	goproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

func webhookTestDynamoClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String("http://localhost:8555") })
}

func webhookCreateTestTable(t *testing.T, db *dynamodb.Client, name string) {
	t.Helper()
	attrs := []types.AttributeDefinition{
		{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
	}
	keys := []types.KeySchemaElement{
		{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
	}
	_, _ = db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(name), AttributeDefinitions: attrs, KeySchema: keys, BillingMode: types.BillingModePayPerRequest,
	})
}

// fakeReactionWebhookWallet fixes GetProductPurchase's result — the webhook
// path only ever calls GetProductPurchase, never Purchase/Refund.
type fakeReactionWebhookWallet struct {
	createResult *walletclient.ProductPurchase
	getResult    *walletclient.ProductPurchase
}

func (f *fakeReactionWebhookWallet) ListProductSKUs(context.Context) ([]walletclient.ProductSKU, error) {
	return nil, nil
}
func (f *fakeReactionWebhookWallet) PurchaseProduct(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return f.createResult, nil
}

func TestCreateReactionPurchasePIXReturnsPaymentPayload(t *testing.T) {
	db := webhookTestDynamoClient(t)
	env := fmt.Sprintf("route_reaction_test_%d", time.Now().UnixNano())
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_entitlements"))
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_purchases"))
	wallet := &fakeReactionWebhookWallet{createResult: &walletclient.ProductPurchase{
		PurchaseID: "prdp-route-1", SKU: "poker_reaction_cold", Amount: 100, Status: "pending",
		PixCopiaECola: "000201-route-pix", QRCodeBase64: "route-qr", ExpiresAt: "2026-08-13T00:00:00Z",
	}}
	svc := reactionpurchase.NewService(wallet, reactionpurchase.NewEntitlementStore(db, env), reactionpurchase.NewStore(db, env))
	app := newReactionPurchaseApp(svc)
	body := []byte(`{"reaction_id":"cold","method":"pix","idem_key":"route-idem"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1.0/wallet/reaction-purchase/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d, want 201", resp.StatusCode)
	}
	var rec reactionpurchase.Record
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.PurchaseID != "prdp-route-1" || rec.PixCopiaECola != "000201-route-pix" || rec.QRCodeBase64 != "route-qr" || rec.ExpiresAt == "" {
		t.Fatalf("response omitted PIX payment payload: %+v", rec)
	}
}
func (f *fakeReactionWebhookWallet) GetProductPurchase(context.Context, string) (*walletclient.ProductPurchase, error) {
	return f.getResult, nil
}
func (f *fakeReactionWebhookWallet) RefundProductPurchase(context.Context, string, string, string) (*walletclient.ProductPurchase, error) {
	return nil, nil
}
func (f *fakeReactionWebhookWallet) Debit(context.Context, string, int64, string, string) error {
	return nil
}
func (f *fakeReactionWebhookWallet) Credit(context.Context, string, int64, string, string) error {
	return nil
}

func TestWalletWebhookDispatchesReactionPurchaseByPrefix(t *testing.T) {
	db := webhookTestDynamoClient(t)
	env := fmt.Sprintf("webhook_reaction_test_%d", time.Now().UnixNano())
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_entitlements"))
	webhookCreateTestTable(t, db, dynamo.TableName(env, "poker_reaction_purchases"))
	entitlements := reactionpurchase.NewEntitlementStore(db, env)
	store := reactionpurchase.NewStore(db, env)

	wallet := &fakeReactionWebhookWallet{
		getResult: &walletclient.ProductPurchase{PurchaseID: "prdp-webhook-1", UserID: "player-1", SKU: "poker_reaction_cold", Amount: 100, Status: "confirmed"},
	}
	svc := reactionpurchase.NewService(wallet, entitlements, store)

	// Seed the pending record CreateReal would have written.
	if _, err := store.Create(context.Background(), reactionpurchase.Record{
		PlayerID: "player-1", PurchaseID: "prdp-webhook-1", ReactionID: "cold", Method: "pix",
		PriceCents: 100, PriceFichas: 100_000, Status: "pending",
		CreatedAt: "2026-08-13T00:00:00Z", UpdatedAt: "2026-08-13T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	sandboxSvc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	reg := ws.NewMemoryRegistry()
	conn := &recordingConn{}
	reg.Register("user#player-1", "conn-1", conn)

	app := fiber.New()
	RegisterWalletWebhook(app.Group("/v1.0"), "secret", sandboxSvc, svc, reg)

	body, _ := json.Marshal(map[string]string{"purchase_id": "prdp-webhook-1"})
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
	if msg.Type != "reaction_purchase_update" || msg.PlayerId != "player-1" || msg.PurchaseId != "prdp-webhook-1" || msg.Code != "confirmed" {
		t.Fatalf("unexpected message: %+v", &msg)
	}

	got, err := entitlements.Get(context.Background(), "player-1", "cold")
	if err != nil || got == nil {
		t.Fatalf("expected entitlement granted after webhook confirm, got %+v (err=%v)", got, err)
	}
}
