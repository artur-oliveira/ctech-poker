package cosmeticpurchase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"net/http"
	"sort"
	"strings"
	"time"

	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/observability"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var (
	ErrNotFound           = errors.New("cosmeticpurchase: not found")
	ErrUnknownItem        = errors.New("cosmeticpurchase: unknown cosmetic")
	ErrNotPremium         = errors.New("cosmeticpurchase: cosmetic is not premium")
	ErrInUse              = errors.New("cosmeticpurchase: cosmetic is the player's current selection, cannot refund")
	ErrAlreadyOwned       = errors.New("cosmeticpurchase: cosmetic already owned")
	ErrPurchasePending    = errors.New("cosmeticpurchase: another purchase is already in progress")
	ErrAlreadyRefunded    = errors.New("cosmeticpurchase: purchase already refunded")
	ErrNotConfirmed       = errors.New("cosmeticpurchase: purchase is not confirmed")
	ErrMissingEntitlement = errors.New("cosmeticpurchase: confirmed purchase has no matching entitlement")
	ErrCatalogMismatch    = errors.New("cosmeticpurchase: wallet catalog is missing a premium cosmetic SKU")
)

const productPurchasePrefix = "prdp"

type wallet interface {
	ListProductSKUs(ctx context.Context) ([]walletclient.ProductSKU, error)
	PurchaseProduct(ctx context.Context, userID, sku, idempotencyKey string) (*walletclient.ProductPurchase, error)
	GetProductPurchase(ctx context.Context, purchaseID string) (*walletclient.ProductPurchase, error)
	RefundProductPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*walletclient.ProductPurchase, error)
	Debit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
	Credit(ctx context.Context, userID string, amount int64, idempotencyKey, reason string) error
}

// CatalogEntry merges cosmetics.catalog's premium flag/fichas price with
// wallet's own PriceCents — prices are never cached/hardcoded locally.
type CatalogEntry struct {
	ID      string `json:"id"`
	Premium bool   `json:"premium"`
	Kind    string `json:"kind"`
	// Owned is read from the entitlement table, never derived from purchase
	// history: a buy/refund/buy/refund cycle leaves history rows the client
	// cannot safely reduce to ownership. Always true for free items, whose
	// ownership is universal.
	Owned       bool  `json:"owned"`
	PriceCents  int64 `json:"price_cents,omitempty"`
	PriceFichas int64 `json:"price_fichas,omitempty"`
}

// currentSelectionFunc reports the player's currently-applied deck/felt id
// for kind. Injected the same way SetOwnershipInvalidator injects a callback,
// to avoid an import cycle with the player package (which itself depends on
// cosmeticpurchase for ownership checks).
type currentSelectionFunc func(ctx context.Context, playerID string, kind cosmetics.Kind) (string, error)

type Service struct {
	wallet              wallet
	entitlements        *EntitlementStore
	store               *Store
	now                 func() time.Time
	invalidateOwnership func(context.Context, string, cosmetics.Kind, string) error
	currentSelection    currentSelectionFunc
}

func NewService(w wallet, entitlements *EntitlementStore, store *Store) *Service {
	return &Service{wallet: w, entitlements: entitlements, store: store, now: time.Now}
}

func (s *Service) SetOwnershipInvalidator(fn func(context.Context, string, cosmetics.Kind, string) error) {
	s.invalidateOwnership = fn
}

// SetCurrentSelectionFunc wires in the "is this item currently applied"
// check Refund needs. Without it, Refund treats every item as never applied
// (fails open on the refund permissiveness axis, not the ownership one —
// ownership itself is unaffected).
func (s *Service) SetCurrentSelectionFunc(fn currentSelectionFunc) {
	s.currentSelection = fn
}

func (s *Service) invalidate(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) {
	if s.invalidateOwnership != nil {
		if err := s.invalidateOwnership(ctx, playerID, kind, itemID); err != nil {
			observability.Warn(ctx, "cosmetic ownership cache invalidation failed", err, "kind", kind, "item_id", itemID)
		}
	}
}

func purchaseRequestKey(playerID string, kind cosmetics.Kind, itemID, method, idemKey string) string {
	sum := sha256.Sum256([]byte(playerID + "\x00" + string(kind) + "\x00" + itemID + "\x00" + method + "\x00" + idemKey))
	return hex.EncodeToString(sum[:])
}

func sandboxPurchaseID(requestKey string) string { return "cpf" + requestKey[:29] }

func definitiveWalletRejection(err error) bool {
	var walletErr *walletclient.Error
	return errors.As(err, &walletErr) && walletErr.Status >= http.StatusBadRequest &&
		walletErr.Status < http.StatusInternalServerError && walletErr.Status != http.StatusRequestTimeout &&
		walletErr.Status != http.StatusTooEarly && walletErr.Status != http.StatusTooManyRequests
}

func recordFromProductPurchase(purchase *walletclient.ProductPurchase, kind cosmetics.Kind, itemID string, priceFichas int64, now string) Record {
	return Record{
		PlayerID: purchase.UserID, PurchaseID: purchase.PurchaseID, Kind: string(kind), ItemID: itemID, Method: methodPIX,
		PriceCents: purchase.Amount, PriceFichas: priceFichas, Status: purchase.Status,
		PixCopiaECola: purchase.PixCopiaECola, QRCodeBase64: purchase.QRCodeBase64, ExpiresAt: purchase.ExpiresAt,
		CreatedAt: now, UpdatedAt: now,
	}
}

// ListCatalog merges the game-owned catalog with wallet prices and this
// player's entitlements, so the client never has to infer ownership from
// purchase history.
func (s *Service) ListCatalog(ctx context.Context, playerID string, kind cosmetics.Kind) ([]CatalogEntry, error) {
	skus, err := s.wallet.ListProductSKUs(ctx)
	if err != nil {
		return nil, err
	}
	owned, err := s.entitlements.OwnedIDs(ctx, playerID, kind)
	if err != nil {
		return nil, err
	}
	priceBySKU := make(map[string]int64, len(skus))
	for _, sku := range skus {
		priceBySKU[sku.ID] = sku.PriceCents
	}
	all := cosmetics.All(kind)
	out := make([]CatalogEntry, 0, len(all))
	for _, e := range all {
		entry := CatalogEntry{ID: e.ID, Premium: e.Premium, Kind: string(e.Kind), Owned: !e.Premium || owned[e.ID]}
		if e.Premium {
			entry.PriceFichas = e.PriceFichas
			price, found := priceBySKU[e.SKU]
			if !found || price <= 0 {
				return nil, fmt.Errorf("%w: %s", ErrCatalogMismatch, e.SKU)
			}
			entry.PriceCents = price
		}
		out = append(out, entry)
	}
	return out, nil
}

// CreateReal first reserves the single purchasable slot, then opens a PIX
// purchase and atomically links its local history. The reservation is not
// ownership: only a wallet-confirmed status activates it.
func (s *Service) CreateReal(ctx context.Context, playerID string, kind cosmetics.Kind, itemID, idemKey string) (Record, walletclient.ProductPurchase, error) {
	sku, priceFichas, ok := cosmetics.SKUFor(kind, itemID)
	if !ok {
		if !cosmetics.IsKnown(kind, itemID) {
			return Record{}, walletclient.ProductPurchase{}, ErrUnknownItem
		}
		return Record{}, walletclient.ProductPurchase{}, ErrNotPremium
	}
	requestKey := purchaseRequestKey(playerID, kind, itemID, methodPIX, idemKey)
	reservation, created, err := s.entitlements.Reserve(ctx, Entitlement{
		PlayerID: playerID, Kind: string(kind), ItemID: itemID, PurchaseMethod: methodPIX,
		Status: statusPending, RequestKey: requestKey, IdemKey: idemKey, CreatedAt: s.now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return Record{}, walletclient.ProductPurchase{}, err
	}
	if !created {
		if reservation != nil && reservation.active() {
			if reservation.RequestKey == requestKey && reservation.PurchaseID != "" {
				existing, getErr := s.store.Get(ctx, playerID, reservation.PurchaseID)
				purchase, walletErr := s.wallet.GetProductPurchase(ctx, reservation.PurchaseID)
				if getErr != nil {
					return Record{}, walletclient.ProductPurchase{}, getErr
				}
				if walletErr != nil {
					return Record{}, walletclient.ProductPurchase{}, walletErr
				}
				if existing != nil && purchase != nil && purchase.UserID == playerID && purchase.SKU == sku {
					updated, _, syncErr := s.syncProductPurchase(ctx, purchase)
					if syncErr != nil {
						return Record{}, walletclient.ProductPurchase{}, syncErr
					}
					if purchase.Status != statusConfirmed {
						return updated, *purchase, nil
					}
					details := recordFromProductPurchase(purchase, kind, itemID, priceFichas, existing.UpdatedAt)
					details.IdemKey = idemKey
					if err := s.store.HydratePIXDetails(ctx, details); err != nil {
						return Record{}, walletclient.ProductPurchase{}, err
					}
					refreshed, err := s.store.Get(ctx, playerID, reservation.PurchaseID)
					if err != nil || refreshed == nil {
						return Record{}, walletclient.ProductPurchase{}, errors.Join(err, ErrNotFound)
					}
					return *refreshed, *purchase, nil
				}
			}
			return Record{}, walletclient.ProductPurchase{}, ErrAlreadyOwned
		}
		if reservation == nil || reservation.PurchaseMethod != methodPIX {
			return Record{}, walletclient.ProductPurchase{}, ErrPurchasePending
		}
		if reservation.PurchaseID != "" {
			existing, getErr := s.store.Get(ctx, playerID, reservation.PurchaseID)
			purchase, walletErr := s.wallet.GetProductPurchase(ctx, reservation.PurchaseID)
			if getErr != nil || walletErr != nil {
				return Record{}, walletclient.ProductPurchase{}, errors.Join(getErr, walletErr)
			}
			if existing == nil || purchase == nil || purchase.UserID != playerID || purchase.SKU != sku {
				return Record{}, walletclient.ProductPurchase{}, ErrPurchasePending
			}
			updated, _, syncErr := s.syncProductPurchase(ctx, purchase)
			if syncErr != nil {
				return Record{}, walletclient.ProductPurchase{}, syncErr
			}
			return updated, *purchase, nil
		}
		if reservation.IdemKey == "" || reservation.RequestKey == "" {
			return Record{}, walletclient.ProductPurchase{}, ErrPurchasePending
		}
		idemKey, requestKey = reservation.IdemKey, reservation.RequestKey
	}

	purchase, err := s.wallet.PurchaseProduct(ctx, playerID, sku, idemKey)
	if err != nil {
		if definitiveWalletRejection(err) {
			if cancelErr := s.entitlements.CancelReservation(ctx, playerID, kind, itemID, requestKey); cancelErr != nil {
				return Record{}, walletclient.ProductPurchase{}, errors.Join(err, cancelErr)
			}
		}
		return Record{}, walletclient.ProductPurchase{}, err
	}
	if purchase == nil || purchase.PurchaseID == "" || purchase.SKU != sku || purchase.Status == "" {
		return Record{}, walletclient.ProductPurchase{}, errors.New("cosmeticpurchase: wallet returned an incomplete product purchase")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	rec := recordFromProductPurchase(purchase, kind, itemID, priceFichas, now)
	rec.PlayerID = playerID
	rec.IdemKey = idemKey
	if err := s.store.AttachRealPurchase(ctx, s.entitlements, rec, requestKey); err != nil {
		return Record{}, walletclient.ProductPurchase{}, err
	}
	stored, err := s.store.Get(ctx, playerID, purchase.PurchaseID)
	if err != nil || stored == nil {
		if err == nil {
			err = errors.New("cosmeticpurchase: attached purchase disappeared")
		}
		return Record{}, walletclient.ProductPurchase{}, err
	}
	if purchase.Status == statusConfirmed {
		confirmed, _, err := s.store.GrantConfirmed(ctx, s.entitlements, *stored)
		if err != nil {
			return Record{}, walletclient.ProductPurchase{}, err
		}
		s.invalidate(ctx, playerID, kind, itemID)
		return confirmed, *purchase, nil
	}
	if purchase.Status == statusFailed || purchase.Status == statusExpired || purchase.Status == statusRefunded {
		now = s.now().UTC().Format(time.RFC3339Nano)
		if err := s.store.ClosePIXTerminal(ctx, s.entitlements, *stored, purchase.Status, now); err != nil {
			return Record{}, walletclient.ProductPurchase{}, err
		}
		stored.Status, stored.UpdatedAt = purchase.Status, now
	}
	return *stored, *purchase, nil
}

// ConfirmFromWebhook re-verifies purchaseID against wallet before ever acting
// on a webhook delivery — mirrors reactionpurchase.Service.ConfirmFromWebhook.
// A confirmed status atomically activates entitlement and history even if the
// webhook arrived before CreateReal finished its local write.
func (s *Service) ConfirmFromWebhook(ctx context.Context, purchaseID string) (Record, bool, error) {
	purchase, err := s.wallet.GetProductPurchase(ctx, purchaseID)
	if err != nil {
		return Record{}, false, err
	}
	return s.syncProductPurchase(ctx, purchase)
}

func (s *Service) syncProductPurchase(ctx context.Context, purchase *walletclient.ProductPurchase) (Record, bool, error) {
	if purchase == nil || purchase.PurchaseID == "" || purchase.UserID == "" {
		return Record{}, false, errors.New("cosmeticpurchase: wallet returned an incomplete product purchase")
	}
	kind, itemID, priceFichas, ok := cosmetics.ItemForSKU(purchase.SKU)
	if !ok {
		return Record{}, false, fmt.Errorf("%w: %s", ErrCatalogMismatch, purchase.SKU)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	local, err := s.store.Get(ctx, purchase.UserID, purchase.PurchaseID)
	if err != nil {
		return Record{}, false, err
	}
	if local != nil && (local.Kind != string(kind) || local.ItemID != itemID) {
		return Record{}, false, errors.New("cosmeticpurchase: wallet SKU does not match local cosmetic")
	}
	if purchase.Status == statusConfirmed {
		if local != nil && (local.Status == statusRefunding || local.Status == statusRefunded) {
			return *local, false, nil
		}
		rec := recordFromProductPurchase(purchase, kind, itemID, priceFichas, now)
		if local != nil {
			rec = *local
			rec.Status, rec.UpdatedAt = statusConfirmed, now
		}
		confirmed, changed, err := s.store.GrantConfirmed(ctx, s.entitlements, rec)
		if err == nil {
			s.invalidate(ctx, purchase.UserID, kind, itemID)
		}
		return confirmed, changed, err
	}
	reconstructed := false
	if local == nil {
		if purchase.Status != statusPending && purchase.Status != statusFailed && purchase.Status != statusExpired && purchase.Status != statusRefunded {
			return Record{}, false, nil
		}
		recovered := recordFromProductPurchase(purchase, kind, itemID, priceFichas, now)
		if err := s.store.RecoverPendingPIX(ctx, s.entitlements, recovered); err != nil {
			return Record{}, false, err
		}
		local = &recovered
		reconstructed = true
	}
	if purchase.Status == statusRefunded && local.Status == statusRefunding {
		if err := s.store.CompleteRefund(ctx, local.PlayerID, local.PurchaseID, now); err != nil {
			return Record{}, false, err
		}
		local.Status, local.UpdatedAt = statusRefunded, now
		return *local, true, nil
	}
	if purchase.Status == statusFailed || purchase.Status == statusExpired || purchase.Status == statusRefunded {
		if err := s.store.ClosePIXTerminal(ctx, s.entitlements, *local, purchase.Status, now); err != nil {
			return Record{}, false, err
		}
		local.Status, local.UpdatedAt = purchase.Status, now
		s.invalidate(ctx, local.PlayerID, kind, itemID)
		return *local, true, nil
	}
	if local.Status == statusRefunding || local.Status == statusRefunded || local.Status == statusFailed || local.Status == statusExpired {
		return *local, false, nil
	}
	if local.Status == statusConfirmed {
		// Wallet purchase states are monotonic. Do not regress a locally
		// confirmed grant on a stale/non-terminal read.
		return *local, false, nil
	}
	if local.Status == purchase.Status {
		return *local, reconstructed, nil
	}
	if _, err := s.store.UpdateStatus(ctx, local.PlayerID, local.PurchaseID, purchase.Status, now); err != nil {
		return Record{}, false, err
	}
	local.Status, local.UpdatedAt = purchase.Status, now
	return *local, true, nil
}

// CreateSandbox persists a processing intent and pending reservation before
// its idempotent wallet debit. A successful debit atomically confirms history
// and ownership; an ambiguous call can be resumed without charging twice.
func (s *Service) CreateSandbox(ctx context.Context, playerID string, kind cosmetics.Kind, itemID, idemKey string) (Record, error) {
	_, priceFichas, ok := cosmetics.SKUFor(kind, itemID)
	if !ok {
		if !cosmetics.IsKnown(kind, itemID) {
			return Record{}, ErrUnknownItem
		}
		return Record{}, ErrNotPremium
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	requestKey := purchaseRequestKey(playerID, kind, itemID, methodFichas, idemKey)
	purchaseID := sandboxPurchaseID(requestKey)
	rec := Record{
		PlayerID: playerID, PurchaseID: purchaseID, Kind: string(kind), ItemID: itemID, Method: methodFichas,
		PriceFichas: priceFichas, Status: statusProcessing, IdemKey: idemKey, CreatedAt: now, UpdatedAt: now,
	}
	entitlement := Entitlement{
		PlayerID: playerID, Kind: string(kind), ItemID: itemID, PurchaseMethod: methodFichas,
		PurchaseID: purchaseID, Status: statusPending, RequestKey: requestKey, IdemKey: idemKey, CreatedAt: now,
	}
	if err := s.store.CreateSandboxReservation(ctx, s.entitlements, rec, entitlement); err != nil {
		if !dynamo.IsConditionFailed(err) {
			return Record{}, err
		}
		existingEnt, entErr := s.entitlements.Get(ctx, playerID, kind, itemID)
		if entErr != nil {
			return Record{}, entErr
		}
		if existingEnt == nil || existingEnt.PurchaseMethod != methodFichas || existingEnt.PurchaseID == "" {
			return Record{}, ErrPurchasePending
		}
		existingRec, recErr := s.store.Get(ctx, playerID, existingEnt.PurchaseID)
		if recErr != nil {
			return Record{}, recErr
		}
		if existingEnt.active() {
			if existingRec != nil && existingRec.Status == statusConfirmed && existingRec.IdemKey == idemKey {
				return *existingRec, nil
			}
			return Record{}, ErrAlreadyOwned
		}
		if existingRec == nil || existingRec.Status != statusProcessing || existingRec.IdemKey == "" || existingEnt.RequestKey == "" {
			return Record{}, ErrPurchasePending
		}
		rec = *existingRec
		idemKey, requestKey, _ = rec.IdemKey, existingEnt.RequestKey, rec.PurchaseID
		priceFichas = rec.PriceFichas
	}
	if err := s.wallet.Debit(ctx, playerID, priceFichas, idemKey, "cosmetic_purchase:"+string(kind)+":"+itemID); err != nil {
		if definitiveWalletRejection(err) {
			if cancelErr := s.store.CancelSandboxReservation(ctx, s.entitlements, rec, requestKey); cancelErr != nil {
				return Record{}, errors.Join(err, cancelErr)
			}
		}
		return Record{}, err
	}
	if err := s.store.ConfirmSandbox(ctx, s.entitlements, rec, requestKey, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return Record{}, err
	}
	rec.Status = statusConfirmed
	rec.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
	s.invalidate(ctx, playerID, kind, itemID)
	return rec, nil
}

// isCurrentSelection reports whether itemID is the player's currently-applied
// deck/felt — the cosmetic equivalent of a reaction's "used" flag. Refund is
// allowed only if the item was never applied, mirroring reactions' "never
// sent" rule generalized to "never selected."
func (s *Service) isCurrentSelection(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) (bool, error) {
	if s.currentSelection == nil {
		return false, nil
	}
	current, err := s.currentSelection(ctx, playerID, kind)
	if err != nil {
		return false, err
	}
	return current == itemID, nil
}

// Refund loads the Record to get Kind/ItemID/Method, then checks the item
// isn't the player's current selection, then branches on Method: pix reverses
// via wallet's product-purchase refund; fichas credits the price back
// directly. Either branch deletes the Entitlement and marks the Record
// refunded.
func (s *Service) Refund(ctx context.Context, playerID, purchaseID, _ string) (Record, error) {
	rec, err := s.store.Get(ctx, playerID, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if rec == nil {
		if !strings.HasPrefix(purchaseID, productPurchasePrefix) {
			return Record{}, ErrNotFound
		}
		purchase, walletErr := s.wallet.GetProductPurchase(ctx, purchaseID)
		if walletErr != nil {
			return Record{}, walletErr
		}
		if purchase == nil || purchase.UserID != playerID {
			return Record{}, ErrNotFound
		}
		updated, _, syncErr := s.syncProductPurchase(ctx, purchase)
		if syncErr != nil {
			return Record{}, syncErr
		}
		if updated.PurchaseID == "" {
			return Record{}, ErrNotFound
		}
		return updated, nil
	}
	kind := cosmetics.Kind(rec.Kind)
	switch rec.Status {
	case statusRefunded:
		return Record{}, ErrAlreadyRefunded
	case statusProcessing, statusPending, statusFailed, statusExpired:
		return Record{}, ErrNotConfirmed
	case statusConfirmed:
		entitlement, err := s.entitlements.Get(ctx, playerID, kind, rec.ItemID)
		if err != nil {
			return Record{}, err
		}
		if entitlement == nil || entitlement.PurchaseID != purchaseID || !entitlement.active() {
			return Record{}, ErrMissingEntitlement
		}
		inUse, err := s.isCurrentSelection(ctx, playerID, kind, rec.ItemID)
		if err != nil {
			return Record{}, err
		}
		if inUse {
			return Record{}, ErrInUse
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		if err := s.store.BeginRefund(ctx, s.entitlements, *rec, now); err != nil {
			latestRec, recordErr := s.store.Get(ctx, playerID, purchaseID)
			if recordErr != nil || latestRec == nil {
				return Record{}, errors.Join(err, recordErr)
			}
			switch latestRec.Status {
			case statusRefunding:
				rec = latestRec
			case statusRefunded:
				return Record{}, ErrAlreadyRefunded
			default:
				return Record{}, err
			}
		} else {
			rec.Status, rec.UpdatedAt = statusRefunding, now
		}
	case statusRefunding:
		// Resume an external operation whose previous response or local completion
		// was interrupted. The deterministic wallet key below makes this safe.
	default:
		return Record{}, fmt.Errorf("cosmeticpurchase: invalid purchase status %q", rec.Status)
	}
	s.invalidate(ctx, playerID, kind, rec.ItemID)

	refundKey := "cosmetic-refund:" + purchaseID
	switch rec.Method {
	case methodPIX:
		refunded, err := s.wallet.RefundProductPurchase(ctx, playerID, purchaseID, refundKey)
		if err != nil {
			return Record{}, err
		}
		if refunded == nil || refunded.Status != statusRefunded {
			return Record{}, errors.New("cosmeticpurchase: wallet did not confirm the PIX refund")
		}
	case methodFichas:
		if err := s.wallet.Credit(ctx, playerID, rec.PriceFichas, refundKey, "cosmetic_refund:"+string(kind)+":"+rec.ItemID); err != nil {
			return Record{}, err
		}
	default:
		return Record{}, fmt.Errorf("cosmeticpurchase: unknown purchase method %q", rec.Method)
	}

	now := s.now().UTC().Format(time.RFC3339Nano)
	if err := s.store.CompleteRefund(ctx, playerID, purchaseID, now); err != nil {
		return Record{}, err
	}
	rec.Status, rec.UpdatedAt = statusRefunded, now
	return *rec, nil
}

// IsOwned is the uncached ownership check — player.Service.requireCosmetic
// calls it directly before persisting a premium deck/felt selection. Unlike
// reactionpurchase, there is no hot-path Valkey cache wrapper here: deck/felt
// writes are low-frequency profile updates, not a per-action hot path (see
// docs/specs/2026-08-21-premium-cosmetics-overhaul.md's scope cuts).
func (s *Service) IsOwned(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) (bool, error) {
	e, err := s.entitlements.Get(ctx, playerID, kind, itemID)
	if err != nil {
		return false, err
	}
	return e.active(), nil
}

// Refresh is the polling safety net. PIX purchases are always re-verified with
// wallet; an interrupted fichas debit resumes from its persisted processing
// record using the original idempotency key.
func (s *Service) Refresh(ctx context.Context, playerID, purchaseID string) (Record, error) {
	rec, err := s.store.Get(ctx, playerID, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if rec == nil {
		purchase, err := s.wallet.GetProductPurchase(ctx, purchaseID)
		if err != nil {
			return Record{}, err
		}
		if purchase == nil || purchase.PurchaseID == "" {
			return Record{}, errors.New("cosmeticpurchase: wallet returned an incomplete product purchase")
		}
		if purchase.PurchaseID != purchaseID || purchase.UserID != playerID {
			return Record{}, ErrNotFound
		}
		updated, _, err := s.syncProductPurchase(ctx, purchase)
		return updated, err
	}
	if rec.Method == methodPIX {
		purchase, err := s.wallet.GetProductPurchase(ctx, purchaseID)
		if err != nil {
			return Record{}, err
		}
		if purchase == nil || purchase.PurchaseID == "" {
			return Record{}, errors.New("cosmeticpurchase: wallet returned an incomplete product purchase")
		}
		if purchase.PurchaseID != purchaseID || purchase.UserID != playerID {
			return Record{}, ErrNotFound
		}
		updated, _, err := s.syncProductPurchase(ctx, purchase)
		return updated, err
	}
	if rec.Method == methodFichas && rec.Status == statusProcessing {
		requestKey := purchaseRequestKey(playerID, cosmetics.Kind(rec.Kind), rec.ItemID, methodFichas, rec.IdemKey)
		if err := s.wallet.Debit(ctx, playerID, rec.PriceFichas, rec.IdemKey, "cosmetic_purchase:"+rec.Kind+":"+rec.ItemID); err != nil {
			if definitiveWalletRejection(err) {
				if cancelErr := s.store.CancelSandboxReservation(ctx, s.entitlements, *rec, requestKey); cancelErr != nil {
					return Record{}, errors.Join(err, cancelErr)
				}
			}
			return Record{}, err
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		if err := s.store.ConfirmSandbox(ctx, s.entitlements, *rec, requestKey, now); err != nil {
			return Record{}, err
		}
		rec.Status, rec.UpdatedAt = statusConfirmed, now
		s.invalidate(ctx, playerID, cosmetics.Kind(rec.Kind), rec.ItemID)
	}
	return *rec, nil
}

// List returns one page of purchase history, newest first *within the page*.
//
// ponytail: page-local ordering. Neither the table's sort key (the purchase
// id) nor gsi_player_kind's (the kind itself) is a timestamp, so the sort
// below can only reorder what this page contains. Fine while a player's
// history fits in a page or two; upgrade path is a created_at sort key on
// gsi_player_kind, queried with ScanIndexForward:false — which needs the
// backfill that the (pk, kind) index deliberately avoided, so it is worth
// doing only once a player's history actually spans pages.
func (s *Service) List(ctx context.Context, playerID string, kind cosmetics.Kind, limit int, startKey map[string]types.AttributeValue) ([]Record, map[string]types.AttributeValue, error) {
	records, nextKey, err := s.store.List(ctx, playerID, kind, limit, startKey)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })
	return records, nextKey, nil
}
