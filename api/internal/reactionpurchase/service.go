package reactionpurchase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/api-commons/observability"
	"gopkg.aoctech.app/poker/api/internal/reactions"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var (
	ErrNotFound           = errors.New("reactionpurchase: not found")
	ErrUnknownReaction    = errors.New("reactionpurchase: unknown reaction")
	ErrNotPremium         = errors.New("reactionpurchase: reaction is not premium")
	ErrAlreadyUsed        = errors.New("reactionpurchase: reaction already used, cannot refund")
	ErrAlreadyOwned       = errors.New("reactionpurchase: reaction already owned")
	ErrPurchasePending    = errors.New("reactionpurchase: another purchase is already in progress")
	ErrAlreadyRefunded    = errors.New("reactionpurchase: purchase already refunded")
	ErrNotConfirmed       = errors.New("reactionpurchase: purchase is not confirmed")
	ErrMissingEntitlement = errors.New("reactionpurchase: confirmed purchase has no matching entitlement")
	ErrCatalogMismatch    = errors.New("reactionpurchase: wallet catalog is missing a premium reaction SKU")
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

// CatalogEntry merges reactions.catalog's premium flag/fichas price with
// wallet's own PriceCents — prices are never cached/hardcoded locally
// (docs/specs/2026-08-12-premium-reactions.md).
type CatalogEntry struct {
	ID      string `json:"id"`
	Premium bool   `json:"premium"`
	// Owned is read from the entitlement table, never derived from purchase
	// history: a buy/refund/buy/refund cycle leaves history rows the client
	// cannot safely reduce to ownership. Always true for free reactions,
	// whose ownership is universal.
	Owned       bool  `json:"owned"`
	PriceCents  int64 `json:"price_cents,omitempty"`
	PriceFichas int64 `json:"price_fichas,omitempty"`
}

type Service struct {
	wallet              wallet
	entitlements        *EntitlementStore
	store               *Store
	now                 func() time.Time
	invalidateOwnership func(context.Context, string, string) error
}

func NewService(w wallet, entitlements *EntitlementStore, store *Store) *Service {
	return &Service{wallet: w, entitlements: entitlements, store: store, now: time.Now}
}

func (s *Service) SetOwnershipInvalidator(fn func(context.Context, string, string) error) {
	s.invalidateOwnership = fn
}

func (s *Service) invalidate(ctx context.Context, playerID, reactionID string) {
	if s.invalidateOwnership != nil {
		if err := s.invalidateOwnership(ctx, playerID, reactionID); err != nil {
			observability.Warn(ctx, "reaction ownership cache invalidation failed", err, "reaction_id", reactionID)
		}
	}
}

func purchaseRequestKey(playerID, reactionID, method, idemKey string) string {
	sum := sha256.Sum256([]byte(playerID + "\x00" + reactionID + "\x00" + method + "\x00" + idemKey))
	return hex.EncodeToString(sum[:])
}

func sandboxPurchaseID(requestKey string) string { return "rpf" + requestKey[:29] }

func definitiveWalletRejection(err error) bool {
	var walletErr *walletclient.Error
	return errors.As(err, &walletErr) && walletErr.Status >= http.StatusBadRequest &&
		walletErr.Status < http.StatusInternalServerError && walletErr.Status != http.StatusRequestTimeout &&
		walletErr.Status != http.StatusTooEarly && walletErr.Status != http.StatusTooManyRequests
}

func recordFromProductPurchase(purchase *walletclient.ProductPurchase, reactionID string, priceFichas int64, now string) Record {
	return Record{
		PlayerID: purchase.UserID, PurchaseID: purchase.PurchaseID, ReactionID: reactionID, Method: methodPIX,
		PriceCents: purchase.Amount, PriceFichas: priceFichas, Status: purchase.Status,
		PixCopiaECola: purchase.PixCopiaECola, QRCodeBase64: purchase.QRCodeBase64, ExpiresAt: purchase.ExpiresAt,
		CreatedAt: now, UpdatedAt: now,
	}
}

// ListCatalog merges the game-owned catalog with wallet prices and this
// player's entitlements, so the client never has to infer ownership from
// purchase history.
func (s *Service) ListCatalog(ctx context.Context, playerID string) ([]CatalogEntry, error) {
	skus, err := s.wallet.ListProductSKUs(ctx)
	if err != nil {
		return nil, err
	}
	owned, err := s.entitlements.OwnedIDs(ctx, playerID)
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
		entry := CatalogEntry{ID: e.ID, Premium: e.Premium, Owned: !e.Premium || owned[e.ID]}
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
func (s *Service) CreateReal(ctx context.Context, playerID, reactionID, idemKey string) (Record, walletclient.ProductPurchase, error) {
	sku, priceFichas, ok := reactions.SKUFor(reactionID)
	if !ok {
		if !reactions.IsKnown(reactionID) {
			return Record{}, walletclient.ProductPurchase{}, ErrUnknownReaction
		}
		return Record{}, walletclient.ProductPurchase{}, ErrNotPremium
	}
	requestKey := purchaseRequestKey(playerID, reactionID, methodPIX, idemKey)
	reservation, created, err := s.entitlements.Reserve(ctx, Entitlement{
		PlayerID: playerID, ReactionID: reactionID, PurchaseMethod: methodPIX,
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
					details := recordFromProductPurchase(purchase, reactionID, priceFichas, existing.UpdatedAt)
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
			if cancelErr := s.entitlements.CancelReservation(ctx, playerID, reactionID, requestKey); cancelErr != nil {
				return Record{}, walletclient.ProductPurchase{}, errors.Join(err, cancelErr)
			}
		}
		return Record{}, walletclient.ProductPurchase{}, err
	}
	if purchase == nil || purchase.PurchaseID == "" || purchase.SKU != sku || purchase.Status == "" {
		return Record{}, walletclient.ProductPurchase{}, errors.New("reactionpurchase: wallet returned an incomplete product purchase")
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	rec := recordFromProductPurchase(purchase, reactionID, priceFichas, now)
	rec.PlayerID = playerID
	rec.IdemKey = idemKey
	if err := s.store.AttachRealPurchase(ctx, s.entitlements, rec, requestKey); err != nil {
		return Record{}, walletclient.ProductPurchase{}, err
	}
	stored, err := s.store.Get(ctx, playerID, purchase.PurchaseID)
	if err != nil || stored == nil {
		if err == nil {
			err = errors.New("reactionpurchase: attached purchase disappeared")
		}
		return Record{}, walletclient.ProductPurchase{}, err
	}
	if purchase.Status == statusConfirmed {
		confirmed, _, err := s.store.GrantConfirmed(ctx, s.entitlements, *stored)
		if err != nil {
			return Record{}, walletclient.ProductPurchase{}, err
		}
		s.invalidate(ctx, playerID, reactionID)
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
// on a webhook delivery — mirrors sandboxpurchase.Service.ConfirmFromWebhook.
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
		return Record{}, false, errors.New("reactionpurchase: wallet returned an incomplete product purchase")
	}
	reactionID, priceFichas, ok := reactions.ReactionForSKU(purchase.SKU)
	if !ok {
		return Record{}, false, fmt.Errorf("%w: %s", ErrCatalogMismatch, purchase.SKU)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	local, err := s.store.Get(ctx, purchase.UserID, purchase.PurchaseID)
	if err != nil {
		return Record{}, false, err
	}
	if local != nil && local.ReactionID != reactionID {
		return Record{}, false, errors.New("reactionpurchase: wallet SKU does not match local reaction")
	}
	if purchase.Status == statusConfirmed {
		if local != nil && (local.Status == statusRefunding || local.Status == statusRefunded) {
			return *local, false, nil
		}
		rec := recordFromProductPurchase(purchase, reactionID, priceFichas, now)
		if local != nil {
			rec = *local
			rec.Status, rec.UpdatedAt = statusConfirmed, now
		}
		confirmed, changed, err := s.store.GrantConfirmed(ctx, s.entitlements, rec)
		if err == nil {
			s.invalidate(ctx, purchase.UserID, reactionID)
		}
		return confirmed, changed, err
	}
	reconstructed := false
	if local == nil {
		if purchase.Status != statusPending && purchase.Status != statusFailed && purchase.Status != statusExpired && purchase.Status != statusRefunded {
			return Record{}, false, nil
		}
		recovered := recordFromProductPurchase(purchase, reactionID, priceFichas, now)
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
		s.invalidate(ctx, local.PlayerID, local.ReactionID)
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
func (s *Service) CreateSandbox(ctx context.Context, playerID, reactionID, idemKey string) (Record, error) {
	_, priceFichas, ok := reactions.SKUFor(reactionID)
	if !ok {
		if !reactions.IsKnown(reactionID) {
			return Record{}, ErrUnknownReaction
		}
		return Record{}, ErrNotPremium
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	requestKey := purchaseRequestKey(playerID, reactionID, methodFichas, idemKey)
	purchaseID := sandboxPurchaseID(requestKey)
	rec := Record{
		PlayerID: playerID, PurchaseID: purchaseID, ReactionID: reactionID, Method: methodFichas,
		PriceFichas: priceFichas, Status: statusProcessing, IdemKey: idemKey, CreatedAt: now, UpdatedAt: now,
	}
	entitlement := Entitlement{
		PlayerID: playerID, ReactionID: reactionID, PurchaseMethod: methodFichas,
		PurchaseID: purchaseID, Status: statusPending, RequestKey: requestKey, IdemKey: idemKey, CreatedAt: now,
	}
	if err := s.store.CreateSandboxReservation(ctx, s.entitlements, rec, entitlement); err != nil {
		if !dynamo.IsConditionFailed(err) {
			return Record{}, err
		}
		existingEnt, entErr := s.entitlements.Get(ctx, playerID, reactionID)
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
	if err := s.wallet.Debit(ctx, playerID, priceFichas, idemKey, "reaction_purchase:"+reactionID); err != nil {
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
	s.invalidate(ctx, playerID, reactionID)
	return rec, nil
}

// Refund loads the Record to get ReactionID/Method, then the Entitlement to
// check UsedAt, and branches on Method: pix reverses via wallet's
// product-purchase refund; fichas credits the price back directly. Either
// branch deletes the Entitlement and marks the Record refunded.
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
	switch rec.Status {
	case statusRefunded:
		return Record{}, ErrAlreadyRefunded
	case statusProcessing, statusPending, statusFailed, statusExpired:
		return Record{}, ErrNotConfirmed
	case statusConfirmed:
		entitlement, err := s.entitlements.Get(ctx, playerID, rec.ReactionID)
		if err != nil {
			return Record{}, err
		}
		if entitlement == nil || entitlement.PurchaseID != purchaseID || !entitlement.active() {
			return Record{}, ErrMissingEntitlement
		}
		if entitlement.UsedAt != "" {
			return Record{}, ErrAlreadyUsed
		}
		now := s.now().UTC().Format(time.RFC3339Nano)
		if err := s.store.BeginRefund(ctx, s.entitlements, *rec, now); err != nil {
			latestEnt, getErr := s.entitlements.Get(ctx, playerID, rec.ReactionID)
			if getErr == nil && latestEnt != nil && latestEnt.UsedAt != "" {
				return Record{}, ErrAlreadyUsed
			}
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
		return Record{}, fmt.Errorf("reactionpurchase: invalid purchase status %q", rec.Status)
	}
	s.invalidate(ctx, playerID, rec.ReactionID)

	refundKey := "reaction-refund:" + purchaseID
	switch rec.Method {
	case methodPIX:
		refunded, err := s.wallet.RefundProductPurchase(ctx, playerID, purchaseID, refundKey)
		if err != nil {
			return Record{}, err
		}
		if refunded == nil || refunded.Status != statusRefunded {
			return Record{}, errors.New("reactionpurchase: wallet did not confirm the PIX refund")
		}
	case methodFichas:
		if err := s.wallet.Credit(ctx, playerID, rec.PriceFichas, refundKey, "reaction_refund:"+rec.ReactionID); err != nil {
			return Record{}, err
		}
	default:
		return Record{}, fmt.Errorf("reactionpurchase: unknown purchase method %q", rec.Method)
	}

	now := s.now().UTC().Format(time.RFC3339Nano)
	if err := s.store.CompleteRefund(ctx, playerID, purchaseID, now); err != nil {
		return Record{}, err
	}
	rec.Status, rec.UpdatedAt = statusRefunded, now
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

// BuildMarkUsedIntent returns the conditional write the table actor includes
// in its own commit transaction. A concurrent refund therefore either sees
// used_at and fails, or deletes the entitlement and makes the reaction fail.
func (s *Service) BuildMarkUsedIntent(_ context.Context, playerID, reactionID string) (*types.TransactWriteItem, error) {
	if !reactions.IsPremium(reactionID) {
		return nil, nil
	}
	item := s.entitlements.BuildMarkUsedTxItem(playerID, reactionID)
	return &item, nil
}

// IsOwned is the uncached ownership check — Actor.handleReaction consults it
// through a Valkey-backed cache wrapper (Task 6), never directly on the hot
// path.
func (s *Service) IsOwned(ctx context.Context, playerID, reactionID string) (bool, error) {
	e, err := s.entitlements.Get(ctx, playerID, reactionID)
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
			return Record{}, errors.New("reactionpurchase: wallet returned an incomplete product purchase")
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
			return Record{}, errors.New("reactionpurchase: wallet returned an incomplete product purchase")
		}
		if purchase.PurchaseID != purchaseID || purchase.UserID != playerID {
			return Record{}, ErrNotFound
		}
		updated, _, err := s.syncProductPurchase(ctx, purchase)
		return updated, err
	}
	if rec.Method == methodFichas && rec.Status == statusProcessing {
		requestKey := purchaseRequestKey(playerID, rec.ReactionID, methodFichas, rec.IdemKey)
		if err := s.wallet.Debit(ctx, playerID, rec.PriceFichas, rec.IdemKey, "reaction_purchase:"+rec.ReactionID); err != nil {
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
		s.invalidate(ctx, playerID, rec.ReactionID)
	}
	return *rec, nil
}

// List returns one page of purchase history, newest first *within the page*.
//
// ponytail: page-local ordering. The sort key is the purchase id, not a
// timestamp, so DynamoDB hands back id order and the sort below can only
// reorder what this page contains. Fine while a player's history fits in a
// page or two; upgrade path is a created_at GSI queried with
// ScanIndexForward:false, at which point this sort goes away entirely.
func (s *Service) List(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]Record, map[string]types.AttributeValue, error) {
	records, nextKey, err := s.store.List(ctx, playerID, limit, startKey)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })
	return records, nextKey, nil
}
