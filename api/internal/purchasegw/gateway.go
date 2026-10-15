// Package purchasegw sells Poker's Sandbox chip packs and premium goods through Billing
// invoices. It implements, method for method, the small wallet interfaces the
// sandboxpurchase, reactionpurchase and cosmeticpurchase services already depend on, so
// those services and the purchase lifecycle stay as they are: only what stands behind
// them changes, from ctech-wallet's PIX sale to a Billing invoice.
//
// A sale is one Billing invoice. Its reference is the purchase id; its metadata says what
// was sold (sku, player, kind). Nothing is stored here: the invoice is the record, and a
// paid one is re-read and checked against the catalogue before anything is granted.
//
//	chip pack: when the invoice is paid, the wallet is credited the pack's Sandbox
//	           credits, idempotent on the invoice id.
//	good:      no ledger effect; "confirmed" makes Poker grant the entitlement.
//
// There is no path here from Real or from Sandbox into anything: this package never calls
// the wallet's real-debit route (a test pins it).
package purchasegw

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	qrcode "github.com/skip2/go-qrcode"

	"gopkg.aoctech.app/poker/api/internal/accountclient"
	"gopkg.aoctech.app/poker/api/internal/billingclient"
	"gopkg.aoctech.app/poker/api/internal/catalog"
	"gopkg.aoctech.app/poker/api/internal/purchaselifecycle"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

const (
	// SandboxPrefix and ProductPrefix start the ids of the two kinds of sale. The
	// product prefix is the one the webhook already routes on.
	SandboxPrefix = "snd_"
	ProductPrefix = "prdp_"

	kindSandbox = "sandbox"
	kindProduct = "product"

	reasonSandboxPack = "sandbox_pack"
	creditDescription = "Compra de fichas (%s)"
	refundReason      = "requested_by_customer"
)

// Billing is the part of billingclient.Client the gateway uses.
type Billing interface {
	EnsureCustomer(ctx context.Context, userID, taxID, name, email string) (string, error)
	CreateInvoice(ctx context.Context, customerID, reference, description string, amountCents int64, metadata map[string]string) (*billingclient.Invoice, error)
	InvoiceByReference(ctx context.Context, reference string) (*billingclient.Invoice, error)
	OpenPix(ctx context.Context, inv *billingclient.Invoice) error
	RefundInvoice(ctx context.Context, invoiceID string, amountCents int64, reason, idemKey string) error
}

// KYC reads the identity a sale is made out to.
type KYC interface {
	Get(ctx context.Context, userID string) (*accountclient.KYC, error)
}

// Moves is the wallet's Sandbox ledger as Poker uses it: grant credits, take them back.
type Moves interface {
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason, description string) error
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason, description string) error
}

// Gateway implements the purchase side of the wallet interfaces over Billing. It embeds
// Moves so Credit and Debit (the Sandbox ledger the same services spend fichas from)
// are the wallet's, untouched.
type Gateway struct {
	Moves
	billing Billing
	kyc     KYC
}

// New builds a Gateway.
func New(moves Moves, billing Billing, kyc KYC) *Gateway {
	return &Gateway{Moves: moves, billing: billing, kyc: kyc}
}

// ── catalogues ───────────────────────────────────────────────────────────────

func (g *Gateway) ListSandboxSKUs(context.Context) ([]walletclient.SandboxSKU, error) {
	packs := catalog.Sandboxes()
	out := make([]walletclient.SandboxSKU, len(packs))
	for i, p := range packs {
		out[i] = walletclient.SandboxSKU{ID: p.ID, PriceCents: p.PriceCents, BaseCredits: p.BaseCredits, BonusPercent: p.BonusPercent, TotalCredits: p.TotalCredits()}
	}
	return out, nil
}

func (g *Gateway) ListProductSKUs(context.Context) ([]walletclient.ProductSKU, error) {
	goods := catalog.Products()
	out := make([]walletclient.ProductSKU, len(goods))
	for i, p := range goods {
		out[i] = walletclient.ProductSKU{ID: p.ID, PriceCents: p.PriceCents}
	}
	return out, nil
}

// ── chip packs ───────────────────────────────────────────────────────────────

// PurchaseSandbox opens the invoice for a chip pack and the Pix to pay it. A retry with
// the same key returns the same sale; the same key for another pack is a conflict.
func (g *Gateway) PurchaseSandbox(ctx context.Context, userID, sku, idempotencyKey, description string) (*walletclient.SandboxPurchase, error) {
	pack, ok := catalog.SandboxByID(sku)
	if !ok {
		return nil, rejected(http.StatusBadRequest, "/problems/bad-request", "Bad Request", "sku inválido")
	}
	inv, id, err := g.open(ctx, kindSandbox, SandboxPrefix, userID, sku, pack.PriceCents, idempotencyKey, description)
	if err != nil {
		return nil, err
	}
	return sandboxView(id, userID, sku, pack, inv), nil
}

// GetSandboxPurchase re-reads the sale from Billing. A paid, correctly priced invoice
// credits the Sandbox wallet (idempotent on the invoice id) and reports "confirmed".
func (g *Gateway) GetSandboxPurchase(ctx context.Context, purchaseID string) (*walletclient.SandboxPurchase, error) {
	inv, userID, sku, err := g.read(ctx, kindSandbox, SandboxPrefix, purchaseID)
	if err != nil {
		return nil, err
	}
	pack, ok := catalog.SandboxByID(sku)
	if !ok {
		return nil, rejected(http.StatusConflict, "/problems/unknown-sku", "Unknown SKU", "o item desta compra não está mais no catálogo")
	}
	view := sandboxView(purchaseID, userID, sku, pack, inv)
	if inv.Status == billingclient.StatusPaid {
		if err := g.checkPaid(inv, pack.PriceCents); err != nil {
			return nil, err
		}
		if err := g.Credit(ctx, userID, pack.TotalCredits(), inv.ID, reasonSandboxPack, fmt.Sprintf(creditDescription, sku)); err != nil {
			return nil, fmt.Errorf("purchasegw: crediting the sandbox wallet: %w", err)
		}
	}
	return view, nil
}

// RefundSandboxPurchase takes the credits back and gives the money back by Pix. The credits
// go first: a refused debit (the player has spent them) stops the refund before any money
// moves. Idempotent: both legs reuse keys derived from the purchase.
func (g *Gateway) RefundSandboxPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*walletclient.SandboxPurchase, error) {
	inv, owner, sku, err := g.read(ctx, kindSandbox, SandboxPrefix, purchaseID)
	if err != nil {
		return nil, err
	}
	if owner != userID {
		return nil, rejected(http.StatusNotFound, "/problems/not-found", "Not Found", "compra não encontrada")
	}
	pack, ok := catalog.SandboxByID(sku)
	if !ok {
		return nil, rejected(http.StatusConflict, "/problems/unknown-sku", "Unknown SKU", "o item desta compra não está mais no catálogo")
	}
	if inv.Status != billingclient.StatusPaid {
		return nil, rejected(http.StatusConflict, "/problems/conflict", "Conflict", "compra ainda não confirmada")
	}
	if err := g.checkPaid(inv, pack.PriceCents); err != nil {
		return nil, err
	}
	if err := g.Debit(ctx, userID, pack.TotalCredits(), "refund-credits-"+inv.ID, "sandbox_pack_refund", "Estorno da compra de fichas"); err != nil {
		return nil, fmt.Errorf("purchasegw: taking the credits back: %w", err)
	}
	if err := g.billing.RefundInvoice(ctx, inv.ID, pack.PriceCents, refundReason, "refund-"+inv.ID); err != nil {
		slog.Error("ALARM sandbox sale: credits taken back but the Pix refund failed; a retry resumes it", "purchase_id", purchaseID, "invoice_id", inv.ID, "err", err)
		return nil, fmt.Errorf("purchasegw: refunding the invoice: %w", err)
	}
	view := sandboxView(purchaseID, userID, sku, pack, inv)
	view.Status = purchaselifecycle.StatusRefunded
	return view, nil
}

// ── goods ────────────────────────────────────────────────────────────────────

// PurchaseProduct opens the invoice for a premium good and the Pix to pay it.
func (g *Gateway) PurchaseProduct(ctx context.Context, userID, sku, idempotencyKey, description string) (*walletclient.ProductPurchase, error) {
	product, ok := catalog.ProductByID(sku)
	if !ok {
		return nil, rejected(http.StatusBadRequest, "/problems/bad-request", "Bad Request", "sku inválido")
	}
	inv, id, err := g.open(ctx, kindProduct, ProductPrefix, userID, sku, product.PriceCents, idempotencyKey, description)
	if err != nil {
		return nil, err
	}
	return productView(id, userID, sku, product, inv), nil
}

// GetProductPurchase re-reads the sale from Billing; "confirmed" is what makes Poker
// grant the entitlement.
func (g *Gateway) GetProductPurchase(ctx context.Context, purchaseID string) (*walletclient.ProductPurchase, error) {
	inv, userID, sku, err := g.read(ctx, kindProduct, ProductPrefix, purchaseID)
	if err != nil {
		return nil, err
	}
	product, ok := catalog.ProductByID(sku)
	if !ok {
		return nil, rejected(http.StatusConflict, "/problems/unknown-sku", "Unknown SKU", "o item desta compra não está mais no catálogo")
	}
	view := productView(purchaseID, userID, sku, product, inv)
	if inv.Status == billingclient.StatusPaid {
		if err := g.checkPaid(inv, product.PriceCents); err != nil {
			return nil, err
		}
	}
	return view, nil
}

// RefundProductPurchase gives the money back by Pix; no ledger is involved.
func (g *Gateway) RefundProductPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*walletclient.ProductPurchase, error) {
	inv, owner, sku, err := g.read(ctx, kindProduct, ProductPrefix, purchaseID)
	if err != nil {
		return nil, err
	}
	if owner != userID {
		return nil, rejected(http.StatusNotFound, "/problems/not-found", "Not Found", "compra não encontrada")
	}
	product, ok := catalog.ProductByID(sku)
	if !ok {
		return nil, rejected(http.StatusConflict, "/problems/unknown-sku", "Unknown SKU", "o item desta compra não está mais no catálogo")
	}
	if inv.Status != billingclient.StatusPaid {
		return nil, rejected(http.StatusConflict, "/problems/conflict", "Conflict", "compra ainda não confirmada")
	}
	if err := g.checkPaid(inv, product.PriceCents); err != nil {
		return nil, err
	}
	if err := g.billing.RefundInvoice(ctx, inv.ID, product.PriceCents, refundReason, "refund-"+inv.ID); err != nil {
		return nil, fmt.Errorf("purchasegw: refunding the invoice: %w", err)
	}
	view := productView(purchaseID, userID, sku, product, inv)
	view.Status = purchaselifecycle.StatusRefunded
	return view, nil
}

// ── shared ───────────────────────────────────────────────────────────────────

// purchaseID derives the sale's id from the player and the idempotency key, so a retry
// reproduces it: it is the invoice reference and the Billing idempotency key.
func purchaseID(prefix, userID, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(userID + "|" + idempotencyKey))
	return prefix + hex.EncodeToString(sum[:])[:24]
}

// open issues the invoice (or returns the one an earlier try issued) and its Pix.
func (g *Gateway) open(ctx context.Context, kind, prefix, userID, sku string, priceCents int64, idempotencyKey, description string) (*billingclient.Invoice, string, error) {
	if idempotencyKey == "" {
		return nil, "", rejected(http.StatusBadRequest, "/problems/bad-request", "Bad Request", "idempotency key obrigatória")
	}
	id := purchaseID(prefix, userID, idempotencyKey)

	// A replay finds its invoice; it must be the same sale.
	if existing, err := g.billing.InvoiceByReference(ctx, id); err == nil {
		if existing.Metadata[billingclient.MetadataSKU] != sku || existing.Metadata[billingclient.MetadataUserID] != userID || existing.AmountCents != priceCents {
			return nil, "", rejected(http.StatusConflict, "/problems/idempotency-conflict", "Idempotency Conflict", "a chave de idempotência já foi usada para outra compra")
		}
		g.openPix(ctx, existing)
		return existing, id, nil
	} else if !errors.Is(err, billingclient.ErrNotFound) {
		return nil, "", err
	}

	k, err := g.kyc.Get(ctx, userID)
	switch {
	case errors.Is(err, accountclient.ErrNoKYC) || (err == nil && k.CPF == ""):
		return nil, "", rejected(http.StatusUnprocessableEntity, "/problems/kyc-required", "KYC Required", "verifique sua identidade para comprar por Pix")
	case err != nil:
		return nil, "", err
	}
	customerID, err := g.billing.EnsureCustomer(ctx, userID, k.CPF, k.LegalName, k.Email)
	if err != nil {
		return nil, "", err
	}
	inv, err := g.billing.CreateInvoice(ctx, customerID, id, description, priceCents, map[string]string{
		billingclient.MetadataSKU: sku, billingclient.MetadataUserID: userID, billingclient.MetadataKind: kind,
	})
	switch {
	case errors.Is(err, billingclient.ErrConflict):
		return nil, "", rejected(http.StatusConflict, "/problems/idempotency-conflict", "Idempotency Conflict", "a chave de idempotência já foi usada para outra compra")
	case err != nil:
		return nil, "", err
	}
	g.openPix(ctx, inv)
	return inv, id, nil
}

// openPix fills the Pix of an open invoice. Best effort: a failure leaves the sale pending
// without a code, and the next read of it tries again.
func (g *Gateway) openPix(ctx context.Context, inv *billingclient.Invoice) {
	if inv.Status != billingclient.StatusOpen {
		return
	}
	if err := g.billing.OpenPix(ctx, inv); err != nil {
		slog.Warn("purchasegw: opening the Pix failed, the sale stays pending without a code", "invoice_id", inv.ID, "err", err)
	}
}

// read loads a sale's invoice and the player and SKU written on it. A reference of another
// kind, or an invoice with no player, is not found: only what this gateway issued is served.
func (g *Gateway) read(ctx context.Context, kind, prefix, purchaseID string) (inv *billingclient.Invoice, userID, sku string, err error) {
	if !strings.HasPrefix(purchaseID, prefix) {
		return nil, "", "", rejected(http.StatusNotFound, "/problems/not-found", "Not Found", "compra não encontrada")
	}
	inv, err = g.billing.InvoiceByReference(ctx, purchaseID)
	if errors.Is(err, billingclient.ErrNotFound) {
		return nil, "", "", rejected(http.StatusNotFound, "/problems/not-found", "Not Found", "compra não encontrada")
	}
	if err != nil {
		return nil, "", "", err
	}
	if inv.Metadata[billingclient.MetadataKind] != kind || inv.Metadata[billingclient.MetadataUserID] == "" {
		return nil, "", "", rejected(http.StatusNotFound, "/problems/not-found", "Not Found", "compra não encontrada")
	}
	return inv, inv.Metadata[billingclient.MetadataUserID], inv.Metadata[billingclient.MetadataSKU], nil
}

// checkPaid is the check before anything is granted: the invoice was paid in full for
// exactly the catalogue price. Anything else is never credited, and says so loudly.
func (g *Gateway) checkPaid(inv *billingclient.Invoice, priceCents int64) error {
	if inv.AmountCents != priceCents || inv.PaidCents < priceCents {
		slog.Error("ALARM poker sale: a paid invoice does not match the catalogue price; nothing granted",
			"invoice_id", inv.ID, "invoice_total", inv.AmountCents, "paid", inv.PaidCents, "catalogue_price", priceCents)
		return rejected(http.StatusConflict, "/problems/amount-mismatch", "Amount Mismatch", "o valor pago não corresponde ao preço do item")
	}
	return nil
}

func status(inv *billingclient.Invoice) string {
	switch inv.Status {
	case billingclient.StatusPaid:
		return purchaselifecycle.StatusConfirmed
	case billingclient.StatusOpen:
		return purchaselifecycle.StatusPending
	default: // VOID, UNCOLLECTIBLE, DRAFT: it will never be paid
		return purchaselifecycle.StatusExpired
	}
}

func qr(code string) string {
	if code == "" {
		return ""
	}
	png, err := qrcode.Encode(code, qrcode.Medium, 256)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(png)
}

func expires(inv *billingclient.Invoice) string {
	if inv.ExpiresAt.IsZero() {
		return ""
	}
	return inv.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z")
}

func sandboxView(id, userID, sku string, pack catalog.Sandbox, inv *billingclient.Invoice) *walletclient.SandboxPurchase {
	return &walletclient.SandboxPurchase{
		PurchaseID: id, UserID: userID, SKU: sku, Amount: pack.PriceCents, AmountExpected: pack.PriceCents,
		CreditsGranted: pack.TotalCredits(), Status: status(inv),
		PixCopiaECola: inv.PixCopiaECola, QRCodeBase64: qr(inv.PixCopiaECola), ExpiresAt: expires(inv),
	}
}

func productView(id, userID, sku string, product catalog.Product, inv *billingclient.Invoice) *walletclient.ProductPurchase {
	return &walletclient.ProductPurchase{
		PurchaseID: id, UserID: userID, SKU: sku, Amount: product.PriceCents, AmountExpected: product.PriceCents,
		Status: status(inv), PixCopiaECola: inv.PixCopiaECola, QRCodeBase64: qr(inv.PixCopiaECola), ExpiresAt: expires(inv),
	}
}

// rejected is a definitive refusal in the shape the services already classify
// (purchaselifecycle.DefinitiveWalletRejection): a 4xx walletclient.Error.
func rejected(statusCode int, typ, title, detail string) error {
	return &walletclient.Error{Status: statusCode, Type: typ, Title: title, Detail: detail}
}
