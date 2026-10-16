package v1

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	goproto "google.golang.org/protobuf/proto"
	"gopkg.aoctech.app/api-commons/ws"

	pokerproto "gopkg.aoctech.app/poker/api/internal/api/v1/proto"
	"gopkg.aoctech.app/poker/api/internal/billingclient"
	"gopkg.aoctech.app/poker/api/internal/sandboxpurchase"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type fakeInvoices struct {
	byID map[string]*billingclient.Invoice
	err  error
}

func (f fakeInvoices) InvoiceByID(_ context.Context, id string) (*billingclient.Invoice, error) {
	if f.err != nil {
		return nil, f.err
	}
	if inv, ok := f.byID[id]; ok {
		return inv, nil
	}
	return nil, billingclient.ErrNotFound
}

func billingDelivery(t *testing.T, app *fiber.App, secret, body string, at time.Time) int {
	t.Helper()
	ts := strconv.FormatInt(at.Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/v1.0"+PathBillingWebhook, bytes.NewBufferString(body))
	req.Header.Set(billingclient.HeaderTimestamp, ts)
	req.Header.Set(billingclient.HeaderSignature, "v1="+billingclient.Sign(secret, ts, []byte(body)))
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode
}

func TestBillingWebhookConfirmsTheSaleAndBroadcastsOnce(t *testing.T) {
	wallet := &fakeSandboxWallet{purchase: &walletclient.SandboxPurchase{UserID: "player-1", Status: "confirmed"}}
	store := newFakeStoreForWebhookTest()
	store.rows[key("player-1", "snd_abc")] = sandboxpurchase.Record{PlayerID: "player-1", PurchaseID: "snd_abc", Status: "pending", TotalCredits: 1000}
	svc := sandboxpurchase.NewService(wallet, store)
	reg := ws.NewMemoryRegistry()
	conn := &recordingConn{}
	reg.Register("user#player-1", "conn-1", conn)

	invoices := fakeInvoices{byID: map[string]*billingclient.Invoice{
		"in_1":    {ID: "in_1", Reference: "snd_abc", Status: "PAID"},
		"in_wlt":  {ID: "in_wlt", Reference: "dep-1", Status: "PAID"}, // another service's invoice
		"in_open": {ID: "in_open", Reference: "snd_abc", Status: "OPEN"},
	}}
	app := fiber.New()
	RegisterBillingWebhook(app.Group("/v1.0"), "secret", invoices, svc, noopReactionSvc(), noopCosmeticSvc(), reg, nil)
	now := time.Now()
	paid := `{"type":"invoice.paid","data":{"object":"invoice","id":"in_1"}}`

	if code := billingDelivery(t, app, "secret", paid, now); code != http.StatusOK {
		t.Fatalf("paid delivery = %d", code)
	}
	if len(conn.messages) != 1 {
		t.Fatalf("broadcasts = %d, want 1", len(conn.messages))
	}
	var msg pokerproto.ServerMessage
	if err := goproto.Unmarshal(conn.messages[0], &msg); err != nil || msg.Type != "sandbox_purchase_update" || msg.PurchaseId != "snd_abc" || msg.Code != "confirmed" {
		t.Fatalf("message = %+v err=%v", &msg, err)
	}
	// A duplicate delivery is a no-op.
	if code := billingDelivery(t, app, "secret", paid, now); code != http.StatusOK || len(conn.messages) != 1 {
		t.Fatalf("duplicate = %d broadcasts=%d", code, len(conn.messages))
	}
}

func TestBillingWebhookRefusesWhatIsNotSignedAndIgnoresWhatIsNotOurs(t *testing.T) {
	svc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	invoices := fakeInvoices{byID: map[string]*billingclient.Invoice{"in_wlt": {ID: "in_wlt", Reference: "dep-1"}}}
	app := fiber.New()
	RegisterBillingWebhook(app.Group("/v1.0"), "secret", invoices, svc, noopReactionSvc(), noopCosmeticSvc(), ws.NewMemoryRegistry(), nil)
	now := time.Now()
	paid := `{"type":"invoice.paid","data":{"object":"invoice","id":"in_wlt"}}`

	if code := billingDelivery(t, app, "wrong", paid, now); code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d", code)
	}
	if code := billingDelivery(t, app, "secret", paid, now.Add(-time.Hour)); code != http.StatusUnauthorized {
		t.Fatalf("stale timestamp = %d", code)
	}
	if code := billingDelivery(t, app, "secret", paid, now); code != http.StatusOK {
		t.Fatalf("another service's invoice = %d, want 200 so Billing does not retry", code)
	}
	if code := billingDelivery(t, app, "secret", `{"type":"invoice.finalized","data":{"object":"invoice","id":"in_wlt"}}`, now); code != http.StatusOK {
		t.Fatalf("another event type = %d", code)
	}
	if code := billingDelivery(t, app, "secret", `not json`, now); code != http.StatusBadRequest {
		t.Fatalf("garbage = %d", code)
	}
}

func TestBillingWebhookAsksBillingToRetryWhenTheInvoiceCannotBeRead(t *testing.T) {
	svc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	app := fiber.New()
	RegisterBillingWebhook(app.Group("/v1.0"), "secret", fakeInvoices{err: context.DeadlineExceeded}, svc, noopReactionSvc(), noopCosmeticSvc(), ws.NewMemoryRegistry(), nil)
	if code := billingDelivery(t, app, "secret", `{"type":"invoice.paid","data":{"object":"invoice","id":"in_1"}}`, time.Now()); code != http.StatusBadGateway {
		t.Fatalf("unreadable invoice = %d, want 502", code)
	}
}

// invoice.paid whose invoice does not read as paid yet (a stale read) must be retried, not answered 200.
func TestBillingWebhookRetriesAnInvoiceThatDoesNotReadAsPaidYet(t *testing.T) {
	svc := sandboxpurchase.NewService(&fakeSandboxWallet{}, newFakeStoreForWebhookTest())
	invoices := fakeInvoices{byID: map[string]*billingclient.Invoice{"in_1": {ID: "in_1", Reference: "snd_abc", Status: "OPEN"}}}
	app := fiber.New()
	RegisterBillingWebhook(app.Group("/v1.0"), "secret", invoices, svc, noopReactionSvc(), noopCosmeticSvc(), ws.NewMemoryRegistry(), nil)
	if code := billingDelivery(t, app, "secret", `{"type":"invoice.paid","data":{"object":"invoice","id":"in_1"}}`, time.Now()); code != http.StatusServiceUnavailable {
		t.Fatalf("a not-yet-paid read = %d, want 503 so Billing retries", code)
	}
}
