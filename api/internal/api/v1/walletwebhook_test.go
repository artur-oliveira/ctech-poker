package v1

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	goproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/api-commons/ws"
	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/reactionpurchase"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// noopReactionSvc is a reactionpurchase.Service that no test in this file
// ever dispatches to (every purchase_id here lacks the "prdp" prefix) — real
// prdp-prefixed dispatch coverage lives in walletwebhook_reaction_test.go,
// which needs DynamoDB Local (build tag integration) since
// reactionpurchase.Service's stores aren't fakeable interfaces.
func noopReactionSvc() *reactionpurchase.Service {
	return reactionpurchase.NewService(&fakeReactionWallet{}, reactionpurchase.NewEntitlementStore(nil, "test"), reactionpurchase.NewStore(nil, "test"))
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type recordingConn struct{ messages [][]byte }

func (r *recordingConn) WriteMessage(_ int, data []byte) error {
	r.messages = append(r.messages, data)
	return nil
}

func TestValidWalletWebhookSignature(t *testing.T) {
	body := []byte(`{"purchase_id":"sbxp-1"}`)
	valid := sign("secret", body)

	cases := []struct {
		name   string
		secret string
		header string
		want   bool
	}{
		{"valid", "secret", valid, true},
		{"wrong secret", "other", valid, false},
		{"tampered body handled by caller, header malformed here", "secret", "sha256=deadbeef", false},
		{"missing prefix", "secret", hex.EncodeToString([]byte("x")), false},
		{"empty secret", "", valid, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validWalletWebhookSignature(tc.secret, body, tc.header); got != tc.want {
				t.Fatalf("validWalletWebhookSignature(%q, body, %q) = %v, want %v", tc.secret, tc.header, got, tc.want)
			}
		})
	}
}

func TestWalletWebhookBroadcastsOnConfirm(t *testing.T) {
	wallet := &fakeSandboxWallet{purchase: &walletclient.SandboxPurchase{UserID: "player-1", Status: "confirmed"}}
	store := newFakeStoreForWebhookTest()
	store.rows[key("player-1", "sbxp-1")] = sandboxpurchase.Record{PlayerID: "player-1", PurchaseID: "sbxp-1", Status: "pending", TotalCredits: 1000}
	svc := sandboxpurchase.NewService(wallet, store)

	reg := ws.NewMemoryRegistry()
	conn := &recordingConn{}
	reg.Register("user#player-1", "conn-1", conn)

	app := fiber.New()
	RegisterWalletWebhook(app.Group("/v1.0"), "secret", svc, noopReactionSvc(), reg)

	body, _ := json.Marshal(map[string]string{"purchase_id": "sbxp-1"})
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
	if msg.Type != "sandbox_purchase_update" || msg.PlayerId != "player-1" || msg.PurchaseId != "sbxp-1" || msg.Code != "confirmed" || msg.Amount != 1000 {
		t.Fatalf("unexpected message: %+v", &msg)
	}

	// Replay: same webhook again — status already confirmed, no second broadcast.
	req2 := httptest.NewRequest(http.MethodPost, "/v1.0/webhooks/wallet", bytes.NewReader(body))
	req2.Header.Set("X-Wallet-Signature", sign("secret", body))
	if _, err := app.Test(req2); err != nil {
		t.Fatalf("app.Test replay: %v", err)
	}
	if len(conn.messages) != 1 {
		t.Fatalf("expected replay not to broadcast again, got %d messages", len(conn.messages))
	}
}

func TestWalletWebhookRejectsBadSignature(t *testing.T) {
	svc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	app := fiber.New()
	RegisterWalletWebhook(app.Group("/v1.0"), "secret", svc, noopReactionSvc(), ws.NewMemoryRegistry())

	body, _ := json.Marshal(map[string]string{"purchase_id": "sbxp-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1.0/webhooks/wallet", bytes.NewReader(body))
	req.Header.Set("X-Wallet-Signature", "sha256=wrong")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// newFakeStoreForWebhookTest reuses the same in-memory fake shape as
// sandboxpurchase's own service tests — defined locally here (package v1)
// against the store interface's exported surface via Record.
func newFakeStoreForWebhookTest() *webhookFakeStore {
	return &webhookFakeStore{rows: map[string]sandboxpurchase.Record{}}
}

type webhookFakeStore struct {
	rows map[string]sandboxpurchase.Record
}

func (f *webhookFakeStore) Create(_ context.Context, rec sandboxpurchase.Record) (sandboxpurchase.Record, error) {
	f.rows[key(rec.PlayerID, rec.PurchaseID)] = rec
	return rec, nil
}
func (f *webhookFakeStore) Get(_ context.Context, playerID, purchaseID string) (*sandboxpurchase.Record, error) {
	rec, ok := f.rows[key(playerID, purchaseID)]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}
func (f *webhookFakeStore) UpdateStatus(_ context.Context, playerID, purchaseID, status, updatedAt string) (bool, error) {
	k := key(playerID, purchaseID)
	rec, ok := f.rows[k]
	if !ok {
		return false, nil
	}
	rec.Status, rec.UpdatedAt = status, updatedAt
	f.rows[k] = rec
	return true, nil
}
func (f *webhookFakeStore) List(context.Context, string, int, map[string]types.AttributeValue) ([]sandboxpurchase.Record, map[string]types.AttributeValue, error) {
	return nil, nil, nil
}

func key(a, b string) string { return a + "#" + b }
