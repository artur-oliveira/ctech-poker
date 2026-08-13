package reactionpurchase

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableEntitlements = "poker_reaction_entitlements"
	tablePurchases    = "poker_reaction_purchases"
)

// Entitlement is one row per **owned** premium reaction — free reactions
// never get a row; ownership of a free reaction is universal
// (docs/specs/2026-08-12-premium-reactions.md).
type Entitlement struct {
	PlayerID       string `dynamodbav:"pk" json:"player_id"`
	ReactionID     string `dynamodbav:"sk" json:"reaction_id"`
	PurchaseMethod string `dynamodbav:"purchase_method" json:"purchase_method"` // "pix" | "fichas"
	PurchaseID     string `dynamodbav:"purchase_id" json:"purchase_id"`
	UsedAt         string `dynamodbav:"used_at,omitempty" json:"used_at,omitempty"`
	CreatedAt      string `dynamodbav:"created_at" json:"created_at"`
}

// Record is purchase history — never TTL'd, mirrors poker_sandbox_purchases's shape.
type Record struct {
	PlayerID    string `dynamodbav:"pk" json:"player_id"`
	PurchaseID  string `dynamodbav:"sk" json:"purchase_id"`
	ReactionID  string `dynamodbav:"reaction_id" json:"reaction_id"`
	Method      string `dynamodbav:"method" json:"method"` // "pix" | "fichas"
	PriceCents  int64  `dynamodbav:"price_cents,omitempty" json:"price_cents,omitempty"`
	PriceFichas int64  `dynamodbav:"price_fichas,omitempty" json:"price_fichas,omitempty"`
	Status      string `dynamodbav:"status" json:"status"` // pending | confirmed | refunded
	CreatedAt   string `dynamodbav:"created_at" json:"created_at"`
	UpdatedAt   string `dynamodbav:"updated_at" json:"updated_at"`
}

type EntitlementStore struct{ base dynamo.Base }

func NewEntitlementStore(db *dynamodb.Client, env string) *EntitlementStore {
	return &EntitlementStore{base: dynamo.NewBase(db, env, tableEntitlements)}
}

func (s *EntitlementStore) Put(ctx context.Context, e Entitlement) error {
	encoded, err := dynamo.Encode(e)
	if err != nil {
		return fmt.Errorf("reactionpurchase: encode entitlement: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItem(encoded)}); err != nil {
		return fmt.Errorf("reactionpurchase: put entitlement: %w", err)
	}
	return nil
}

func (s *EntitlementStore) Get(ctx context.Context, playerID, reactionID string) (*Entitlement, error) {
	item, err := s.base.GetItem(ctx, playerID, reactionID)
	if err != nil {
		return nil, fmt.Errorf("reactionpurchase: get entitlement: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Entitlement](item)
}

// MarkUsed is a conditional update setting used_at only if empty —
// first-use-wins, idempotent on replay.
func (s *EntitlementStore) MarkUsed(ctx context.Context, playerID, reactionID string) error {
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: playerID},
			"sk": &types.AttributeValueMemberS{Value: reactionID},
		},
		UpdateExpression:    aws.String("SET used_at = :now"),
		ConditionExpression: aws.String("attribute_not_exists(used_at) OR used_at = :empty"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now":   &types.AttributeValueMemberS{Value: dynamo.NowStr()},
			":empty": &types.AttributeValueMemberS{Value: ""},
		},
	})
	if err != nil && !dynamo.IsConditionFailed(err) {
		return fmt.Errorf("reactionpurchase: mark used: %w", err)
	}
	return nil
}

func (s *EntitlementStore) Delete(ctx context.Context, playerID, reactionID string) error {
	if _, err := s.base.DeleteItem(ctx, playerID, reactionID); err != nil {
		return fmt.Errorf("reactionpurchase: delete entitlement: %w", err)
	}
	return nil
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePurchases)}
}

// Create persists rec, or returns the existing row unchanged on a retried
// request — mirrors sandboxpurchase.Store.Create's conditional-put-then-reget idiom.
func (s *Store) Create(ctx context.Context, rec Record) (Record, error) {
	encoded, err := dynamo.Encode(rec)
	if err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: encode record: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err == nil {
		return rec, nil
	} else if !dynamo.IsConditionFailed(err) {
		return Record{}, fmt.Errorf("reactionpurchase: persist record: %w", err)
	}
	existing, err := s.base.GetItem(ctx, rec.PlayerID, rec.PurchaseID)
	if err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: load existing record: %w", err)
	}
	if existing == nil {
		return Record{}, fmt.Errorf("reactionpurchase: record disappeared")
	}
	decoded, err := dynamo.Decode[Record](existing)
	if err != nil {
		return Record{}, fmt.Errorf("reactionpurchase: decode existing record: %w", err)
	}
	return *decoded, nil
}

func (s *Store) Get(ctx context.Context, playerID, purchaseID string) (*Record, error) {
	item, err := s.base.GetItem(ctx, playerID, purchaseID)
	if err != nil {
		return nil, fmt.Errorf("reactionpurchase: get record: %w", err)
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

func (s *Store) List(ctx context.Context, playerID string) ([]Record, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("reactionpurchase: list records: %w", err)
	}
	out := make([]Record, 0, len(result.Items))
	for _, item := range result.Items {
		rec, err := dynamo.Decode[Record](item)
		if err != nil {
			return nil, fmt.Errorf("reactionpurchase: decode record: %w", err)
		}
		out = append(out, *rec)
	}
	return out, nil
}
