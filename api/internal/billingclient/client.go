// Package billingclient is Poker's client for ctech-billing: customers, one-off
// invoices (a chip pack or a good), and credit notes that give money back. It speaks the
// routes Billing's M2M surface serves, with Poker's own OAuth client, and nothing else.
//
// A copy of ctech-wallet's client (without payouts): the shared part is a candidate for
// ctech-go-common once a third caller needs it.
package billingclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gopkg.aoctech.app/api-commons/oauth2client"
)

const (
	// Billing's invoice statuses, as it renders them.
	StatusPaid = "PAID"
	StatusOpen = "OPEN"

	// Payer matches Billing computes against the customer's registered CPF.
	MatchYes = "match"
	// MatchPartial: the payer comes from the bank's webhook, the document masked to
	// its middle digits and every shown digit equal to the customer's.
	MatchPartial = "partial"

	// Invoice metadata: what a sale is, so a paid invoice can be checked against the catalogue.
	MetadataSKU    = "sku"
	MetadataUserID = "user_id"
	MetadataKind   = "kind"

	// OwnerKey is the routing key of Poker's invoices, credentials and webhook endpoint.
	OwnerKey = "poker"

	pathToken = "/v1.0/token"
	scopes    = "billing:customers:read billing:customers:write billing:invoices:read billing:invoices:write"

	// HeaderSignature and HeaderTimestamp are Billing's webhook headers.
	HeaderSignature = "X-Billing-Signature"
	HeaderTimestamp = "X-Billing-Timestamp"
)

var (
	// ErrNotFound: Billing has no such invoice or customer.
	ErrNotFound = errors.New("billing: not found")
	// ErrConflict: an idempotency key or reference was reused for something else.
	ErrConflict = errors.New("billing: idempotency or reference conflict")
	// ErrNoPix: the invoice cannot be paid by Pix now (not open, or no checkout link).
	ErrNoPix = errors.New("billing: invoice has no payable Pix")
)

// Invoice is a one-off invoice as Poker needs it.
type Invoice struct {
	ID          string
	Status      string
	AmountCents int64 // the invoice total
	PaidCents   int64
	Reference   string
	Metadata    map[string]string // sku, user_id, kind: what the sale was, as Poker wrote it
	CheckoutURL string

	// Set by OpenPix.
	PixCopiaECola string
	ExpiresAt     time.Time

	Payer      *Payer // nil until Billing has recorded who paid
	PayerMatch string // "match" | "mismatch" | "unknown" | "" (not recorded yet)
}

// Payer is who paid an invoice, from the bank statement.
type Payer struct {
	Name, Document, Institution string
}

// Client talks to Billing.
type Client struct {
	base   string
	http   *http.Client
	tokens *oauth2client.TokenManager
}

// New builds the client. base is Billing's API, ctechURL ctech-account's (the token
// endpoint). It makes no request: the token is fetched on first use.
func New(base, ctechURL, clientID, clientSecret string) *Client {
	h := &http.Client{Timeout: 8 * time.Second}
	ctech := strings.TrimRight(ctechURL, "/")
	return &Client{
		base:   strings.TrimRight(base, "/"),
		http:   h,
		tokens: oauth2client.New(h, nil, ctech+pathToken, clientID, clientSecret, scopes),
	}
}

type problemBody struct {
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func (c *Client) do(ctx context.Context, method, path string, idemKey string, in, out any, authed bool) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if authed {
		tok, err := c.tokens.Get(ctx)
		if err != nil {
			return fmt.Errorf("billing token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusConflict:
		var p problemBody
		_ = json.Unmarshal(raw, &p)
		return fmt.Errorf("%w: %s %s", ErrConflict, p.Code, p.Detail)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		var p problemBody
		_ = json.Unmarshal(raw, &p)
		return fmt.Errorf("billing %s %s: status %d %s %s", method, path, resp.StatusCode, p.Code, p.Detail)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// ── customers ────────────────────────────────────────────────────────────────

type customerJSON struct {
	ID          string `json:"id"`
	TaxIDMasked string `json:"tax_id_masked"`
}

// EnsureCustomer returns the Billing customer of a Poker player, creating it on first
// use. The player may already be a customer for another reason (a wallet deposit, a DF-e
// plan), in which case that customer is reused.
func (c *Client) EnsureCustomer(ctx context.Context, userID, taxID, name, email string) (string, error) {
	var found customerJSON
	err := c.do(ctx, http.MethodGet, "/v1.0/customers/by-user/"+url.PathEscape(userID), "", nil, &found, true)
	if err == nil {
		// A customer that exists for another reason may have been created without a
		// CPF, and Billing cannot open a Pix charge for one. Poker reads the CPF from
		// the player's KYC, so it gives the customer that one, once; Billing never
		// replaces a CPF that is set.
		if found.TaxIDMasked == "" {
			if err := c.do(ctx, http.MethodPut, "/v1.0/customers/"+url.PathEscape(found.ID)+"/tax-id", "poker-taxid-"+found.ID, map[string]string{"tax_id": taxID}, nil, true); err != nil {
				return "", fmt.Errorf("billing customer %s: setting the CPF: %w", found.ID, err)
			}
		}
		return found.ID, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", err
	}
	var made customerJSON
	err = c.do(ctx, http.MethodPost, "/v1.0/customers", "poker-customer-"+userID, map[string]string{
		"user_id": userID, "external_ref": "poker:" + userID, "name": name, "email": email, "tax_id": taxID,
	}, &made, true)
	if errors.Is(err, ErrConflict) { // created by a concurrent call in between
		err = c.do(ctx, http.MethodGet, "/v1.0/customers/by-user/"+url.PathEscape(userID), "", nil, &made, true)
	}
	if err != nil {
		return "", err
	}
	return made.ID, nil
}

// ── invoices ─────────────────────────────────────────────────────────────────

type invoiceJSON struct {
	ID          string            `json:"id"`
	Reference   string            `json:"reference"`
	Status      string            `json:"status"`
	Total       int64             `json:"total"`
	AmountPaid  int64             `json:"amount_paid"`
	CheckoutURL string            `json:"checkout_url"`
	Metadata    map[string]string `json:"metadata"`
	Payer       *struct {
		Name        string `json:"name"`
		Document    string `json:"document"`
		Institution string `json:"institution"`
	} `json:"payer"`
	PayerMatch string `json:"payer_match"`
}

func (j *invoiceJSON) invoice() *Invoice {
	inv := &Invoice{
		ID: j.ID, Status: j.Status, AmountCents: j.Total, PaidCents: j.AmountPaid,
		Reference: j.Reference, Metadata: j.Metadata, CheckoutURL: j.CheckoutURL, PayerMatch: j.PayerMatch,
	}
	if j.Payer != nil {
		inv.Payer = &Payer{Name: j.Payer.Name, Document: j.Payer.Document, Institution: j.Payer.Institution}
	}
	return inv
}

// CreateInvoice issues a sale's invoice. The reference doubles as the idempotency key, so a
// retry returns the same invoice; metadata says what was sold, for the check on payment.
// payerMessage is what the payer reads on the Pix ("Pacote de 10.000 fichas") and pixLifetime
// how long the Pix stays payable: a sale here is a quick one, and Billing cannot know that.
func (c *Client) CreateInvoice(ctx context.Context, customerID, reference, description, payerMessage string, amountCents int64, pixLifetime time.Duration, metadata map[string]string) (*Invoice, error) {
	var out invoiceJSON
	err := c.do(ctx, http.MethodPost, "/v1.0/invoices", reference, map[string]any{
		"customer_id": customerID, "description": description, "amount_cents": amountCents,
		"payer_message": payerMessage, "pix_expires_in_seconds": int(pixLifetime.Seconds()),
		"reference": reference, "owner_key": OwnerKey, "metadata": metadata,
	}, &out, true)
	if err != nil {
		return nil, err
	}
	return out.invoice(), nil
}

// InvoiceByReference reads the invoice Poker issued under a reference.
func (c *Client) InvoiceByReference(ctx context.Context, reference string) (*Invoice, error) {
	var out invoiceJSON
	if err := c.do(ctx, http.MethodGet, "/v1.0/invoices/by-reference/"+url.PathEscape(reference), "", nil, &out, true); err != nil {
		return nil, err
	}
	return out.invoice(), nil
}

// InvoiceByID reads an invoice by Billing's id (what an event names).
func (c *Client) InvoiceByID(ctx context.Context, id string) (*Invoice, error) {
	var out invoiceJSON
	if err := c.do(ctx, http.MethodGet, "/v1.0/invoices/"+url.PathEscape(id), "", nil, &out, true); err != nil {
		return nil, err
	}
	return out.invoice(), nil
}

// OpenPix opens (or returns the still-live) Pix charge of an open invoice, through
// the public checkout link Billing put on it, and fills the copia-e-cola and its
// expiry. The link is the credential of that route, so no bearer is sent.
func (c *Client) OpenPix(ctx context.Context, inv *Invoice) error {
	if inv.Status != StatusOpen || inv.CheckoutURL == "" {
		return ErrNoPix
	}
	u, err := url.Parse(inv.CheckoutURL)
	if err != nil || u.Query().Get("token") == "" {
		return fmt.Errorf("%w: checkout link without a token", ErrNoPix)
	}
	var out struct {
		Payment *struct {
			PixCode   string    `json:"pix_code"`
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"payment"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1.0/checkout/"+url.PathEscape(u.Query().Get("token"))+"/pay", "", nil, &out, false); err != nil {
		return err
	}
	if out.Payment == nil || out.Payment.PixCode == "" {
		return ErrNoPix
	}
	inv.PixCopiaECola, inv.ExpiresAt = out.Payment.PixCode, out.Payment.ExpiresAt
	return nil
}

// RefundInvoice gives money back to the payer of a paid invoice: a credit note
// with the refund by Pix, which Billing does against the payment it holds.
func (c *Client) RefundInvoice(ctx context.Context, invoiceID string, amountCents int64, reason, idemKey string) error {
	return c.do(ctx, http.MethodPost, "/v1.0/invoices/"+url.PathEscape(invoiceID)+"/credit-notes", idemKey, map[string]any{
		"amount": amountCents, "reason": reason, "refund_via_pix": true,
	}, nil, true)
}

// ── events ───────────────────────────────────────────────────────────────────

// Sign renders Billing's delivery signature: HMAC-SHA256 over "timestamp.body".
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks a delivery: the header is "v1=<hex>", the timestamp is
// unix seconds within skew of now, and the signature covers both.
func VerifySignature(secret, header, timestamp string, body []byte, now time.Time, skew time.Duration) bool {
	sig, ok := strings.CutPrefix(header, "v1=")
	if !ok || secret == "" {
		return false
	}
	sec, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	at := time.Unix(sec, 0)
	if now.Sub(at) > skew || at.Sub(now) > skew {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(Sign(secret, timestamp, body)))
}
