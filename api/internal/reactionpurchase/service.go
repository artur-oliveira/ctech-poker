package reactionpurchase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gopkg.aoctech.app/poker/api/internal/reactions"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var (
	ErrNotFound        = errors.New("reactionpurchase: not found")
	ErrUnknownReaction = errors.New("reactionpurchase: unknown reaction")
	ErrNotPremium      = errors.New("reactionpurchase: reaction is not premium")
	ErrAlreadyUsed     = errors.New("reactionpurchase: reaction already used, cannot refund")
)

type wallet interface {
	ListProductSKUs(ctx context.Context) ([]walletclient.ProductSKU, error)
	PurchaseProduct(ctx context.Context, userID, sku, idempotencyKey string) (*walletclient.ProductPurchase, error)
	GetProductPurchase(ctx context.Context, purchaseID string) (*walletclient.ProductPurchase, error)
	RefundProductPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*walletclient.ProductPurchase, error)
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

// CatalogEntry merges reactions.catalog's premium flag/fichas price with
// wallet's own PriceCents — prices are never cached/hardcoded locally
// (docs/specs/2026-08-12-premium-reactions.md).
type CatalogEntry struct {
	ID          string `json:"id"`
	Premium     bool   `json:"premium"`
	PriceCents  int64  `json:"price_cents,omitempty"`
	PriceFichas int64  `json:"price_fichas,omitempty"`
}

type Service struct {
	wallet       wallet
	entitlements *EntitlementStore
	store        *Store
	now          func() time.Time
}

func NewService(w wallet, entitlements *EntitlementStore, store *Store) *Service {
	return &Service{wallet: w, entitlements: entitlements, store: store, now: time.Now}
}

func (s *Service) ListCatalog(ctx context.Context) ([]CatalogEntry, error) {
	skus, err := s.wallet.ListProductSKUs(ctx)
	if err != nil {
		return nil, err
	}
	priceBySKU := make(map[string]int64, len(skus))
	for _, sku := range skus {
		priceBySKU[sku.ID] = sku.PriceCents
	}
	all := reactions.All()
	out := make([]CatalogEntry, 0, len(all))
	for _, e := range all {
		entry := CatalogEntry{ID: e.ID, Premium: e.Premium}
		if e.Premium {
			entry.PriceFichas = e.PriceFichas
			entry.PriceCents = priceBySKU[e.SKU]
		}
		out = append(out, entry)
	}
	return out, nil
}

// CreateReal opens a real-money (PIX) purchase via wallet's product-purchase
// route. No entitlement row yet — same "confirm before granting" posture as
// sandbox credits; ConfirmFromWebhook is the only place a pix-method
// entitlement is created.
func (s *Service) CreateReal(ctx context.Context, playerID, reactionID, idemKey string) (Record, walletclient.ProductPurchase, error) {
	sku, priceFichas, ok := reactions.SKUFor(reactionID)
	if !ok {
		if !reactions.IsKnown(reactionID) {
			return Record{}, walletclient.ProductPurchase{}, ErrUnknownReaction
		}
		return Record{}, walletclient.ProductPurchase{}, ErrNotPremium
	}
	purchase, err := s.wallet.PurchaseProduct(ctx, playerID, sku, idemKey)
	if err != nil {
		return Record{}, walletclient.ProductPurchase{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	rec := Record{
		PlayerID: playerID, PurchaseID: purchase.PurchaseID, ReactionID: reactionID, Method: "pix",
		PriceCents: purchase.Amount, PriceFichas: priceFichas, Status: purchase.Status,
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.store.Create(ctx, rec)
	if err != nil {
		return Record{}, walletclient.ProductPurchase{}, err
	}
	return created, *purchase, nil
}

// ConfirmFromWebhook re-verifies purchaseID against wallet before ever acting
// on a webhook delivery — mirrors sandboxpurchase.Service.ConfirmFromWebhook.
// On a new "confirmed" status, writes the Entitlement row — the only place a
// pix-method entitlement is created.
func (s *Service) ConfirmFromWebhook(ctx context.Context, purchaseID string) (Record, bool, error) {
	purchase, err := s.wallet.GetProductPurchase(ctx, purchaseID)
	if err != nil {
		return Record{}, false, err
	}
	local, err := s.store.Get(ctx, purchase.UserID, purchaseID)
	if err != nil {
		return Record{}, false, err
	}
	if local == nil {
		return Record{}, false, nil
	}
	if local.Status == purchase.Status {
		return *local, false, nil
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.store.UpdateStatus(ctx, purchase.UserID, purchaseID, purchase.Status, now); err != nil {
		return Record{}, false, fmt.Errorf("reactionpurchase: webhook update status: %w", err)
	}
	local.Status, local.UpdatedAt = purchase.Status, now
	if purchase.Status == "confirmed" {
		if err := s.entitlements.Put(ctx, Entitlement{
			PlayerID: local.PlayerID, ReactionID: local.ReactionID, PurchaseMethod: "pix",
			PurchaseID: purchaseID, CreatedAt: now,
		}); err != nil {
			return Record{}, false, fmt.Errorf("reactionpurchase: grant entitlement: %w", err)
		}
	}
	return *local, true, nil
}

// CreateSandbox debits priceFichas synchronously (no PIX involved) and, on
// success, writes both the history Record (status "confirmed") and the
// Entitlement row in one call — the sandbox debit is itself the
// confirmation, there is nothing async to wait for
// (docs/specs/2026-08-12-premium-reactions.md).
func (s *Service) CreateSandbox(ctx context.Context, playerID, reactionID, idemKey string) (Record, error) {
	_, priceFichas, ok := reactions.SKUFor(reactionID)
	if !ok {
		if !reactions.IsKnown(reactionID) {
			return Record{}, ErrUnknownReaction
		}
		return Record{}, ErrNotPremium
	}
	if err := s.wallet.Debit(ctx, playerID, priceFichas, idemKey, "reaction_purchase:"+reactionID); err != nil {
		return Record{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	purchaseID := "rp-" + idemKey // poker-minted id — no wallet purchase object exists for this leg
	rec, err := s.store.Create(ctx, Record{
		PlayerID: playerID, PurchaseID: purchaseID, ReactionID: reactionID, Method: "fichas",
		PriceFichas: priceFichas, Status: "confirmed", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return Record{}, err
	}
	if err := s.entitlements.Put(ctx, Entitlement{
		PlayerID: playerID, ReactionID: reactionID, PurchaseMethod: "fichas",
		PurchaseID: purchaseID, CreatedAt: now,
	}); err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: grant entitlement: %w", err)
	}
	return rec, nil
}

// Refund loads the Record to get ReactionID/Method, then the Entitlement to
// check UsedAt, and branches on Method: pix reverses via wallet's
// product-purchase refund; fichas credits the price back directly. Either
// branch deletes the Entitlement and marks the Record refunded.
func (s *Service) Refund(ctx context.Context, playerID, purchaseID, idemKey string) (Record, error) {
	rec, err := s.store.Get(ctx, playerID, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if rec == nil {
		return Record{}, ErrNotFound
	}
	entitlement, err := s.entitlements.Get(ctx, playerID, rec.ReactionID)
	if err != nil {
		return Record{}, err
	}
	if entitlement != nil && entitlement.UsedAt != "" {
		return Record{}, ErrAlreadyUsed
	}

	switch rec.Method {
	case "pix":
		if _, err := s.wallet.RefundProductPurchase(ctx, playerID, purchaseID, idemKey); err != nil {
			return Record{}, err
		}
	case "fichas":
		if err := s.wallet.Credit(ctx, playerID, rec.PriceFichas, idemKey, "reaction_refund:"+rec.ReactionID); err != nil {
			return Record{}, err
		}
	default:
		return Record{}, fmt.Errorf("reactionpurchase: unknown purchase method %q", rec.Method)
	}

	if err := s.entitlements.Delete(ctx, playerID, rec.ReactionID); err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: revoke entitlement: %w", err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.store.UpdateStatus(ctx, playerID, purchaseID, "refunded", now); err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: update status: %w", err)
	}
	rec.Status, rec.UpdatedAt = "refunded", now
	return *rec, nil
}

// MarkUsed is a no-op for a free reaction (no entitlement row exists) —
// callers only invoke it when reactions.IsPremium(id) is true; kept
// tolerant here too so a caller mistake never breaks a reaction send.
func (s *Service) MarkUsed(ctx context.Context, playerID, reactionID string) error {
	if !reactions.IsPremium(reactionID) {
		return nil
	}
	return s.entitlements.MarkUsed(ctx, playerID, reactionID)
}

// IsOwned is the uncached ownership check — Actor.handleReaction consults it
// through a Valkey-backed cache wrapper (Task 6), never directly on the hot
// path.
func (s *Service) IsOwned(ctx context.Context, playerID, reactionID string) (bool, error) {
	e, err := s.entitlements.Get(ctx, playerID, reactionID)
	if err != nil {
		return false, err
	}
	return e != nil, nil
}

func (s *Service) List(ctx context.Context, playerID string) ([]Record, error) {
	records, err := s.store.List(ctx, playerID)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })
	return records, nil
}
