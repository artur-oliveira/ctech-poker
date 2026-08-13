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

func (s *Service) List(ctx context.Context, playerID string) ([]Record, error) {
	records, err := s.store.List(ctx, playerID)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })
	return records, nil
}
