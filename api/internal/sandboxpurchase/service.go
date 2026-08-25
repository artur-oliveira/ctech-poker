package sandboxpurchase

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"sort"
	"time"

	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var ErrNotFound = errors.New("sandboxpurchase: not found")

type wallet interface {
	ListSandboxSKUs(ctx context.Context) ([]walletclient.SandboxSKU, error)
	PurchaseSandbox(ctx context.Context, userID, sku, idempotencyKey string) (*walletclient.SandboxPurchase, error)
	GetSandboxPurchase(ctx context.Context, purchaseID string) (*walletclient.SandboxPurchase, error)
	RefundSandboxPurchase(ctx context.Context, userID, purchaseID, idempotencyKey string) (*walletclient.SandboxPurchase, error)
}

type store interface {
	Create(ctx context.Context, rec Record) (Record, error)
	Get(ctx context.Context, playerID, purchaseID string) (*Record, error)
	UpdateStatus(ctx context.Context, playerID, purchaseID, status, updatedAt string) (bool, error)
	List(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]Record, map[string]types.AttributeValue, error)
}

type Service struct {
	wallet wallet
	store  store
	now    func() time.Time
}

func NewService(wallet wallet, store store) *Service {
	return &Service{wallet: wallet, store: store, now: time.Now}
}

func (s *Service) ListSKUs(ctx context.Context) ([]walletclient.SandboxSKU, error) {
	return s.wallet.ListSandboxSKUs(ctx)
}

// Create validates sku against the live catalog (so base_credits/bonus_percent
// can be recorded locally — wallet's purchase response only returns the
// total), opens the PIX charge, and persists the history row. A retry with
// the same idemKey is idempotent end to end: wallet derives the same
// purchase_id, and Store.Create returns the already-persisted row.
func (s *Service) Create(ctx context.Context, playerID, sku, idemKey string) (Record, error) {
	skus, err := s.wallet.ListSandboxSKUs(ctx)
	if err != nil {
		return Record{}, err
	}
	var def *walletclient.SandboxSKU
	for i := range skus {
		if skus[i].ID == sku {
			def = &skus[i]
			break
		}
	}
	if def == nil {
		return Record{}, fmt.Errorf("sandboxpurchase: unknown sku %q", sku)
	}

	purchase, err := s.wallet.PurchaseSandbox(ctx, playerID, sku, idemKey)
	if err != nil {
		return Record{}, err
	}

	now := s.now().UTC().Format(time.RFC3339Nano)
	rec := Record{
		PlayerID: playerID, PurchaseID: purchase.PurchaseID, SKU: purchase.SKU,
		PriceCents: def.PriceCents, BaseCredits: def.BaseCredits, BonusPercent: def.BonusPercent,
		TotalCredits: purchase.CreditsGranted, Status: purchase.Status,
		PixCopiaECola: purchase.PixCopiaECola, QRCodeBase64: purchase.QRCodeBase64,
		ExpiresAt: purchase.ExpiresAt, CreatedAt: now, UpdatedAt: now,
	}
	return s.store.Create(ctx, rec)
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

// Refresh re-fetches purchaseID from wallet (source of truth) and updates the
// local row if its status changed — the frontend's safety-net poll path.
func (s *Service) Refresh(ctx context.Context, playerID, purchaseID string) (Record, error) {
	local, err := s.store.Get(ctx, playerID, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if local == nil {
		return Record{}, ErrNotFound
	}
	purchase, err := s.wallet.GetSandboxPurchase(ctx, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if purchase.Status != local.Status {
		now := s.now().UTC().Format(time.RFC3339Nano)
		if _, err := s.store.UpdateStatus(ctx, playerID, purchaseID, purchase.Status, now); err != nil {
			return Record{}, fmt.Errorf("sandboxpurchase: update status: %w", err)
		}
		local.Status, local.UpdatedAt = purchase.Status, now
	}
	return *local, nil
}

func (s *Service) Refund(ctx context.Context, playerID, purchaseID, idemKey string) (Record, error) {
	local, err := s.store.Get(ctx, playerID, purchaseID)
	if err != nil {
		return Record{}, err
	}
	if local == nil {
		return Record{}, ErrNotFound
	}
	purchase, err := s.wallet.RefundSandboxPurchase(ctx, playerID, purchaseID, idemKey)
	if err != nil {
		return Record{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.store.UpdateStatus(ctx, playerID, purchaseID, purchase.Status, now); err != nil {
		return Record{}, fmt.Errorf("sandboxpurchase: update status: %w", err)
	}
	local.Status, local.UpdatedAt = purchase.Status, now
	return *local, nil
}

// ConfirmFromWebhook re-verifies purchaseID against wallet before ever acting
// on a webhook delivery — the webhook body itself is never trusted (mirrors
// ctech-wallet's own posture for its inbound PIX webhook). changed is false
// on a replay (status already matches) or when poker has no local row for a
// purchase_id wallet knows about.
func (s *Service) ConfirmFromWebhook(ctx context.Context, purchaseID string) (Record, bool, error) {
	purchase, err := s.wallet.GetSandboxPurchase(ctx, purchaseID)
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
		return Record{}, false, fmt.Errorf("sandboxpurchase: webhook update status: %w", err)
	}
	local.Status, local.UpdatedAt = purchase.Status, now
	return *local, true, nil
}
