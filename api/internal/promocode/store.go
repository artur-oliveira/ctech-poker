// Package promocode persists redeemable promotional codes and the
// welcome-pack's once-per-player eligibility for the sandbox-credit purchase
// flow (issue #348, desmembrado de #215). Every guarantee here is a
// DynamoDB conditional write — never an optimistic check — per the explicit
// acceptance criterion: this feature charges real PIX money (via
// walletclient), just for sandbox chips, so a double-redemption is a real
// fraud vector, not a cosmetic bug.
package promocode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableCodes       = "poker_promo_codes"
	tableRedemptions = "poker_promo_redemptions"
)

var (
	// ErrNotFound means the code does not exist in poker_promo_codes.
	ErrNotFound = errors.New("promocode: not found")
	// ErrExpired means the code exists but its expires_at has passed.
	ErrExpired = errors.New("promocode: expired")
	// ErrRedemptionLimit means the code exists and is not expired, but its
	// fleet-wide max_redemptions has already been reached.
	ErrRedemptionLimit = errors.New("promocode: redemption limit reached")
	// ErrAlreadyRedeemed means playerID has already redeemed this exact code
	// (or, for ClaimOnce, already purchased that SKU) — the conditional put
	// on the per-player redemption row lost, which is the intended way for
	// this to fail: never a read-then-branch check.
	ErrAlreadyRedeemed = errors.New("promocode: already redeemed by this player")
)

// Code is a promo code's catalog entry. SKU is the wallet catalog SKU the
// code unlocks — never a client-supplied price or bonus. The actual price
// and credits granted always come from walletclient.ListSandboxSKUs's own
// catalog entry for SKU, resolved by sandboxpurchase.Service the same way
// an ordinary (non-promo) purchase already does; a promo code only changes
// *which* SKU gets charged, never what a SKU costs.
type Code struct {
	Code            string
	SKU             string
	MaxRedemptions  int64
	RedemptionCount int64
	ExpiresAt       time.Time
}

type codeItem struct {
	PK              string `dynamodbav:"pk"`
	SKU             string `dynamodbav:"sku"`
	MaxRedemptions  int64  `dynamodbav:"max_redemptions"`
	RedemptionCount int64  `dynamodbav:"redemption_count"`
	ExpiresAt       int64  `dynamodbav:"expires_at"`
}

func (i codeItem) toCode() Code {
	return Code{
		Code: i.PK, SKU: i.SKU, MaxRedemptions: i.MaxRedemptions,
		RedemptionCount: i.RedemptionCount, ExpiresAt: time.Unix(i.ExpiresAt, 0).UTC(),
	}
}

// redemptionItem is a per-player "this key was already spent" guard. key is
// either a promo code (Redeem) or a SKU (ClaimOnce, the welcome pack) — the
// two never collide because a promo code is never allowed to resolve to
// sandboxpurchase.WelcomePackSKU (enforced by whoever seeds poker_promo_codes,
// not by this package).
type redemptionItem struct {
	PK        string `dynamodbav:"pk"`
	SK        string `dynamodbav:"sk"`
	CreatedAt int64  `dynamodbav:"created_at"`
}

type Store struct {
	codes       dynamo.Base
	redemptions dynamo.Base
	now         func() time.Time
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{
		codes:       dynamo.NewBase(db, env, tableCodes),
		redemptions: dynamo.NewBase(db, env, tableRedemptions),
		now:         time.Now,
	}
}

// GetCode returns code's catalog entry, or (nil, nil) if it does not exist.
func (s *Store) GetCode(ctx context.Context, code string) (*Code, error) {
	item, err := s.codes.GetItem(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("promocode: get code: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	decoded, err := dynamo.Decode[codeItem](item)
	if err != nil {
		return nil, fmt.Errorf("promocode: decode code: %w", err)
	}
	c := decoded.toCode()
	return &c, nil
}

// Redeem atomically claims code for playerID: first a conditional put of a
// per-player redemption row (attribute_not_exists(pk) — blocks a duplicate
// redemption by the same player, ErrAlreadyRedeemed on loss), then a
// conditional counter bump on the code itself (not expired, under its
// redemption cap). If the second write loses, the first is rolled back
// (best-effort) so a legitimate retry is never permanently blocked by a
// code that turned out to be expired or exhausted.
func (s *Store) Redeem(ctx context.Context, playerID, code string) (*Code, error) {
	now := s.now()
	redemption, err := dynamo.Encode(redemptionItem{PK: playerID, SK: code, CreatedAt: now.Unix()})
	if err != nil {
		return nil, fmt.Errorf("promocode: encode redemption: %w", err)
	}
	if _, err := s.redemptions.PutItemRaw(ctx, &dynamodb.PutItemInput{
		Item:                redemption,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	}); err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil, ErrAlreadyRedeemed
		}
		return nil, fmt.Errorf("promocode: claim redemption: %w", err)
	}

	out, err := s.codes.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:                 map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: code}},
		UpdateExpression:    aws.String("ADD redemption_count :one"),
		ConditionExpression: aws.String("attribute_exists(pk) AND expires_at > :now AND redemption_count < max_redemptions"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one": &types.AttributeValueMemberN{Value: "1"},
			":now": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", now.Unix())},
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		s.rollbackRedemption(ctx, playerID, code)
		if !dynamo.IsConditionFailed(err) {
			return nil, fmt.Errorf("promocode: bump redemption count: %w", err)
		}
		// Condition failed for one of three disjoint reasons; a plain Get here
		// is informational only (to report *which* one) — the conditional
		// UpdateItem above, not this read, is what actually enforced it.
		existing, getErr := s.GetCode(ctx, code)
		if getErr != nil || existing == nil {
			return nil, ErrNotFound
		}
		if !existing.ExpiresAt.After(now) {
			return nil, ErrExpired
		}
		return nil, ErrRedemptionLimit
	}
	decoded, err := dynamo.Decode[codeItem](out.Attributes)
	if err != nil {
		return nil, fmt.Errorf("promocode: decode updated code: %w", err)
	}
	c := decoded.toCode()
	return &c, nil
}

// ReleaseRedemption undoes a successful Redeem when the purchase it was
// meant to cover never actually happened (e.g. the wallet PIX charge call
// itself errored) — deletes the player's redemption row and gives the
// redemption slot back to the code's counter. Best-effort: a failure here
// only means the player's next retry sees ErrAlreadyRedeemed/ErrRedemptionLimit
// and has to reach out, never a duplicate discount.
func (s *Store) ReleaseRedemption(ctx context.Context, playerID, code string) {
	if _, err := s.redemptions.DeleteItem(ctx, playerID, code); err != nil {
		slog.Warn("promocode: rollback redemption failed", "player_id", playerID, "code", code, "err", err)
		return
	}
	if _, err := s.codes.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:                 map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: code}},
		UpdateExpression:    aws.String("ADD redemption_count :neg"),
		ConditionExpression: aws.String("attribute_exists(pk) AND redemption_count > :zero"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":neg":  &types.AttributeValueMemberN{Value: "-1"},
			":zero": &types.AttributeValueMemberN{Value: "0"},
		},
	}); err != nil && !dynamo.IsConditionFailed(err) {
		slog.Warn("promocode: rollback redemption count failed", "code", code, "err", err)
	}
}

func (s *Store) rollbackRedemption(ctx context.Context, playerID, code string) {
	if _, err := s.redemptions.DeleteItem(ctx, playerID, code); err != nil {
		slog.Warn("promocode: rollback failed redemption attempt failed", "player_id", playerID, "code", code, "err", err)
	}
}

// ClaimOnce enforces "at most once per player" for sku with no
// code/expiry/redemption-cap semantics — the welcome pack. Reuses the same
// redemption table (sk=sku instead of sk=code) and the identical
// conditional-put guard Redeem uses, so a duplicate claim fails the exact
// same conditional write, never an optimistic check.
func (s *Store) ClaimOnce(ctx context.Context, playerID, sku string) error {
	item, err := dynamo.Encode(redemptionItem{PK: playerID, SK: sku, CreatedAt: s.now().Unix()})
	if err != nil {
		return fmt.Errorf("promocode: encode claim: %w", err)
	}
	if _, err := s.redemptions.PutItemRaw(ctx, &dynamodb.PutItemInput{
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	}); err != nil {
		if dynamo.IsConditionFailed(err) {
			return ErrAlreadyRedeemed
		}
		return fmt.Errorf("promocode: claim: %w", err)
	}
	return nil
}

// ReleaseClaim undoes a successful ClaimOnce when the purchase it was meant
// to cover never happened, mirroring ReleaseRedemption for the code-less case.
func (s *Store) ReleaseClaim(ctx context.Context, playerID, sku string) {
	if _, err := s.redemptions.DeleteItem(ctx, playerID, sku); err != nil {
		slog.Warn("promocode: rollback claim failed", "player_id", playerID, "sku", sku, "err", err)
	}
}
