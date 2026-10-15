package billingclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// stub stands in for both ctech-account's token endpoint and Billing.
type stub struct {
	t      *testing.T
	mu     sync.Mutex
	calls  []call
	routes map[string]func(w http.ResponseWriter, r *http.Request, body []byte)
}

type call struct {
	method, path, idem, auth string
	body                     []byte
}

func newStub(t *testing.T) (*stub, *Client) {
	t.Helper()
	s := &stub{t: t, routes: map[string]func(http.ResponseWriter, *http.Request, []byte){}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathToken {
			_, _ = w.Write([]byte(`{"access_token":"tok-1","token_type":"Bearer","expires_in":3600}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.calls = append(s.calls, call{r.Method, r.URL.Path, r.Header.Get("Idempotency-Key"), r.Header.Get("Authorization"), body})
		h := s.routes[r.Method+" "+r.URL.Path]
		s.mu.Unlock()
		if h == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h(w, r, body)
	}))
	t.Cleanup(srv.Close)
	return s, New(srv.URL, srv.URL, "poker", "s")
}

func (s *stub) on(route string, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[route] = func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func (s *stub) last() call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[len(s.calls)-1]
}

func TestCreateInvoiceCarriesTheReferenceAsKeyAndOwnerAndWhatWasSold(t *testing.T) {
	s, c := newStub(t)
	s.on("POST /v1.0/invoices", 201, `{"id":"in_1","reference":"snd_1","status":"OPEN","total":500,"amount_paid":0,"checkout_url":"https://x/checkout?token=T","metadata":{"sku":"pack_500"}}`)
	inv, err := c.CreateInvoice(context.Background(), "cus_1", "snd_1", "Compra de fichas", 500, map[string]string{MetadataSKU: "pack_500", MetadataUserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	got := s.last()
	if got.idem != "snd_1" || got.auth != "Bearer tok-1" {
		t.Fatalf("call = %+v", got)
	}
	var sent map[string]any
	_ = json.Unmarshal(got.body, &sent)
	if sent["owner_key"] != "poker" || sent["amount_cents"] != float64(500) || sent["reference"] != "snd_1" {
		t.Fatalf("body = %v", sent)
	}
	if inv.ID != "in_1" || inv.Reference != "snd_1" || inv.Metadata[MetadataSKU] != "pack_500" || inv.AmountCents != 500 {
		t.Fatalf("invoice = %+v", inv)
	}
}

func TestACreatedInvoiceWithAnotherBodyIsAConflict(t *testing.T) {
	s, c := newStub(t)
	s.on("POST /v1.0/invoices", 409, `{"code":"idempotency_conflict","detail":"different body"}`)
	if _, err := c.CreateInvoice(context.Background(), "cus_1", "snd_1", "x", 100, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestEnsureCustomerReusesOneAndGivesItTheCPFOnce(t *testing.T) {
	s, c := newStub(t)
	s.on("GET /v1.0/customers/by-user/u1", 200, `{"id":"cus_9","tax_id_masked":""}`)
	s.on("PUT /v1.0/customers/cus_9/tax-id", 200, `{}`)
	id, err := c.EnsureCustomer(context.Background(), "u1", "12345678909", "Ana", "ana@x.com")
	if err != nil || id != "cus_9" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if got := s.last(); got.method != "PUT" || got.idem != "poker-taxid-cus_9" {
		t.Fatalf("last call = %+v", got)
	}
}

func TestEnsureCustomerCreatesOnFirstUse(t *testing.T) {
	s, c := newStub(t)
	s.on("POST /v1.0/customers", 201, `{"id":"cus_new"}`)
	id, err := c.EnsureCustomer(context.Background(), "u1", "12345678909", "Ana", "ana@x.com")
	if err != nil || id != "cus_new" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	var sent map[string]string
	_ = json.Unmarshal(s.last().body, &sent)
	if sent["external_ref"] != "poker:u1" || sent["tax_id"] != "12345678909" {
		t.Fatalf("body = %v", sent)
	}
}

func TestOpenPixUsesTheCheckoutTokenWithoutABearer(t *testing.T) {
	s, c := newStub(t)
	s.on("POST /v1.0/checkout/T/pay", 200, `{"payment":{"pix_code":"000201...","expires_at":"2026-10-15T20:00:00Z"}}`)
	inv := &Invoice{Status: StatusOpen, CheckoutURL: "https://x/checkout?token=T"}
	if err := c.OpenPix(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	if inv.PixCopiaECola != "000201..." || inv.ExpiresAt.IsZero() || s.last().auth != "" {
		t.Fatalf("inv = %+v call = %+v", inv, s.last())
	}
	if err := c.OpenPix(context.Background(), &Invoice{Status: StatusPaid, CheckoutURL: "https://x/c?token=T"}); !errors.Is(err, ErrNoPix) {
		t.Fatalf("a paid invoice has no Pix to open: %v", err)
	}
}

func TestRefundInvoiceIsACreditNoteRefundedByPix(t *testing.T) {
	s, c := newStub(t)
	s.on("POST /v1.0/invoices/in_1/credit-notes", 201, `{}`)
	if err := c.RefundInvoice(context.Background(), "in_1", 500, "refund", "refund-snd_1"); err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	got := s.last()
	_ = json.Unmarshal(got.body, &sent)
	if got.idem != "refund-snd_1" || sent["refund_via_pix"] != true || sent["amount"] != float64(500) {
		t.Fatalf("call = %+v body = %v", got, sent)
	}
}

func TestVerifySignature(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"type":"invoice.paid"}`)
	sig := "v1=" + Sign("secret", ts, body)
	if !VerifySignature("secret", sig, ts, body, now, 5*time.Minute) {
		t.Fatal("a good signature was refused")
	}
	for name, ok := range map[string]bool{
		"wrong secret":  VerifySignature("other", sig, ts, body, now, 5*time.Minute),
		"tampered body": VerifySignature("secret", sig, ts, []byte(`{"type":"x"}`), now, 5*time.Minute),
		"stale":         VerifySignature("secret", sig, ts, body, now.Add(time.Hour), 5*time.Minute),
		"no secret":     VerifySignature("", sig, ts, body, now, 5*time.Minute),
		"no prefix":     VerifySignature("secret", Sign("secret", ts, body), ts, body, now, 5*time.Minute),
	} {
		if ok {
			t.Errorf("%s was accepted", name)
		}
	}
}
