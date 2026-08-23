package handreveal

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tableHandRevealPayments = "poker_hand_reveal_payments"

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
)

type paymentItem struct {
	PK     string `dynamodbav:"pk"` // hand_id#viewer_id
	Status string `dynamodbav:"status"`
}

func paymentKey(handID, viewerID string) string { return handID + "#" + viewerID }

type PaymentStore struct{ base dynamo.Base }

func NewPaymentStore(db *dynamodb.Client, env string) *PaymentStore {
	return &PaymentStore{base: dynamo.NewBase(db, env, tableHandRevealPayments)}
}

// ClaimPayment conditionally creates a pending payment row for (handID,
// viewerID) and returns its status. A retried call (network retry, or a
// caller resuming after a wallet failure partway through Service.PayForReveal)
// finds the existing row instead of creating a second one and returns its
// current status — mirrors dailyreward.Store.Claim's idempotent-claim shape.
func (s *PaymentStore) ClaimPayment(ctx context.Context, handID, viewerID string) (string, error) {
	item, err := dynamo.Encode(paymentItem{PK: paymentKey(handID, viewerID), Status: StatusPending})
	if err != nil {
		return "", fmt.Errorf("handreveal: encode payment: %w", err)
	}
	err = s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(item)})
	if err == nil {
		return StatusPending, nil
	}
	if !dynamo.IsConditionFailed(err) {
		return "", fmt.Errorf("handreveal: claim payment: %w", err)
	}
	existing, err := s.base.GetItem(ctx, paymentKey(handID, viewerID))
	if err != nil {
		return "", fmt.Errorf("handreveal: load claimed payment: %w", err)
	}
	if existing == nil {
		return "", fmt.Errorf("handreveal: payment claim disappeared")
	}
	decoded, err := dynamo.Decode[paymentItem](existing)
	if err != nil {
		return "", fmt.Errorf("handreveal: decode payment: %w", err)
	}
	return decoded.Status, nil
}

func (s *PaymentStore) CompletePayment(ctx context.Context, handID, viewerID string) error {
	ok, err := s.base.UpdateItem(ctx, paymentKey(handID, viewerID), nil, map[string]any{"status": StatusCompleted})
	if err != nil {
		return fmt.Errorf("handreveal: complete payment: %w", err)
	}
	if !ok {
		return fmt.Errorf("handreveal: payment claim not found for %s#%s", handID, viewerID)
	}
	return nil
}

func (s *PaymentStore) HasPaid(ctx context.Context, handID, viewerID string) (bool, error) {
	item, err := s.base.GetItem(ctx, paymentKey(handID, viewerID))
	if err != nil {
		return false, fmt.Errorf("handreveal: has paid: %w", err)
	}
	if item == nil {
		return false, nil
	}
	decoded, err := dynamo.Decode[paymentItem](item)
	if err != nil {
		return false, fmt.Errorf("handreveal: decode payment: %w", err)
	}
	return decoded.Status == StatusCompleted, nil
}
