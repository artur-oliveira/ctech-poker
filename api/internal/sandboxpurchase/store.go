package sandboxpurchase

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tablePurchases = "poker_sandbox_purchases"

// Record is poker's own copy of a sandbox-credit purchase — a full history
// row, never TTL'd (unlike ctech-wallet's own pending-purchase row).
type Record struct {
	PlayerID      string `dynamodbav:"pk" json:"player_id"`
	PurchaseID    string `dynamodbav:"sk" json:"purchase_id"`
	SKU           string `dynamodbav:"sku" json:"sku"`
	PriceCents    int64  `dynamodbav:"price_cents" json:"price_cents"`
	BaseCredits   int64  `dynamodbav:"base_credits" json:"base_credits"`
	BonusPercent  int64  `dynamodbav:"bonus_percent" json:"bonus_percent"`
	TotalCredits  int64  `dynamodbav:"total_credits" json:"total_credits"`
	Status        string `dynamodbav:"status" json:"status"`
	PixCopiaECola string `dynamodbav:"pix_copia_e_cola,omitempty" json:"pix_copia_e_cola,omitempty"`
	QRCodeBase64  string `dynamodbav:"qr_code_base64,omitempty" json:"qr_code_base64,omitempty"`
	ExpiresAt     string `dynamodbav:"expires_at,omitempty" json:"expires_at,omitempty"`
	CreatedAt     string `dynamodbav:"created_at" json:"created_at"`
	UpdatedAt     string `dynamodbav:"updated_at" json:"updated_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePurchases)}
}

// Create persists rec, or — if a retried request already created it (same
// deterministic wallet purchase_id) — returns the existing row unchanged.
// Mirrors dailyreward.Store.Claim's conditional-put-then-reget idiom.
func (s *Store) Create(ctx context.Context, rec Record) (Record, error) {
	encoded, err := dynamo.Encode(rec)
	if err != nil {
		return Record{}, fmt.Errorf("sandboxpurchase: encode: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err == nil {
		return rec, nil
	} else if !dynamo.IsConditionFailed(err) {
		return Record{}, fmt.Errorf("sandboxpurchase: persist: %w", err)
	}
	existing, err := s.base.GetItem(ctx, rec.PlayerID, rec.PurchaseID)
	if err != nil {
		return Record{}, fmt.Errorf("sandboxpurchase: load existing: %w", err)
	}
	if existing == nil {
		return Record{}, fmt.Errorf("sandboxpurchase: record disappeared")
	}
	decoded, err := dynamo.Decode[Record](existing)
	if err != nil {
		return Record{}, fmt.Errorf("sandboxpurchase: decode existing: %w", err)
	}
	return *decoded, nil
}

func (s *Store) Get(ctx context.Context, playerID, purchaseID string) (*Record, error) {
	item, err := s.base.GetItem(ctx, playerID, purchaseID)
	if err != nil {
		return nil, fmt.Errorf("sandboxpurchase: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Record](item)
}

func (s *Store) UpdateStatus(ctx context.Context, playerID, purchaseID, status, updatedAt string) (bool, error) {
	sk := purchaseID
	return s.base.UpdateItem(ctx, playerID, &sk, map[string]any{"status": status, "updated_at": updatedAt})
}

// List returns one page of this player's purchase history plus the DynamoDB
// key to resume from.
func (s *Store) List(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]Record, map[string]types.AttributeValue, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: limit, ExclusiveStartKey: startKey})
	if err != nil {
		return nil, nil, fmt.Errorf("sandboxpurchase: list: %w", err)
	}
	out := make([]Record, 0, len(result.Items))
	for _, item := range result.Items {
		rec, err := dynamo.Decode[Record](item)
		if err != nil {
			return nil, nil, fmt.Errorf("sandboxpurchase: decode: %w", err)
		}
		out = append(out, *rec)
	}
	return out, result.LastEvaluatedKey, nil
}
