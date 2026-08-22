package entitlement

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableEntitlements = "poker_table_entitlements"
	skPrefix          = "ent#"
	// ttlSlack keeps an expired-but-not-yet-reaped row around long enough for
	// ActiveFor's in-memory expiry filter to be the source of truth, not
	// DynamoDB's eventually-consistent TTL sweep.
	ttlSlack = 24 * time.Hour
)

type entitlementItem struct {
	PK           string `dynamodbav:"pk"`
	SK           string `dynamodbav:"sk"`
	BoundTableID string `dynamodbav:"bound_table_id"`
	Tier         string `dynamodbav:"tier"`
	FeeCents     int64  `dynamodbav:"fee_cents"`
	ExpiresAt    int64  `dynamodbav:"expires_at"`
	CreatedAt    int64  `dynamodbav:"created_at"`
	TTL          int64  `dynamodbav:"ttl"`
}

func sk(originTableID string) string { return skPrefix + originTableID }

func (i entitlementItem) toEntitlement() Entitlement {
	return Entitlement{
		PlayerID:      i.PK,
		OriginTableID: i.SK[len(skPrefix):],
		BoundTableID:  i.BoundTableID,
		Tier:          i.Tier,
		FeeCents:      i.FeeCents,
		ExpiresAt:     time.Unix(i.ExpiresAt, 0).UTC(),
		CreatedAt:     time.Unix(i.CreatedAt, 0).UTC(),
	}
}

type Store struct {
	base dynamo.Base
	now  func() time.Time
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableEntitlements), now: time.Now}
}

// Claim persists a table-reservation entitlement, keyed by
// (PlayerID, OriginTableID). The condition — absent, OR present but already
// expired — is the mutex that stops two concurrent buy-ins for the same
// player+table from both charging the entry fee, while still letting a fresh
// buy-in at the same table recharge once the previous entitlement's window
// has genuinely passed (an expired row is otherwise indistinguishable from a
// live one until DynamoDB's eventually-consistent TTL sweep reaps it).
// Returns ErrAlreadyClaimed if a still-valid entitlement already exists.
func (s *Store) Claim(ctx context.Context, e Entitlement) error {
	created := e.CreatedAt
	if created.IsZero() {
		created = s.now()
	}
	item, err := dynamo.Encode(entitlementItem{
		PK: e.PlayerID, SK: sk(e.OriginTableID), BoundTableID: e.BoundTableID, Tier: e.Tier,
		FeeCents: e.FeeCents, ExpiresAt: e.ExpiresAt.Unix(), CreatedAt: created.Unix(),
		TTL: e.ExpiresAt.Add(ttlSlack).Unix(),
	})
	if err != nil {
		return fmt.Errorf("entitlement: encode: %w", err)
	}
	_, err = s.base.PutItemRaw(ctx, &dynamodb.PutItemInput{
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk) OR expires_at <= :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", s.now().Unix())},
		},
	})
	if err != nil {
		if dynamo.IsConditionFailed(err) {
			return ErrAlreadyClaimed
		}
		return fmt.Errorf("entitlement: claim: %w", err)
	}
	return nil
}

// ActiveFor returns playerID's not-yet-expired entitlements. A player holds
// only a handful of these at once (one per concurrently reserved table), so
// filtering expiry in memory after a single Query is cheaper than a GSI.
func (s *Store) ActiveFor(ctx context.Context, playerID string) ([]Entitlement, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, SKPrefix: skPrefix, Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("entitlement: query: %w", err)
	}
	now := s.now()
	out := make([]Entitlement, 0, len(result.Items))
	for _, raw := range result.Items {
		item, err := dynamo.Decode[entitlementItem](raw)
		if err != nil {
			return nil, fmt.Errorf("entitlement: decode: %w", err)
		}
		e := item.toEntitlement()
		if e.ExpiresAt.After(now) {
			out = append(out, e)
		}
	}
	return out, nil
}

// Rebind points an existing, still-valid entitlement at a new table — used
// when its currently bound table becomes unavailable (archived or full).
// The condition (row exists AND not expired) is what stops a race from
// resurrecting an entitlement that just expired, or rebinding one a
// concurrent caller already deleted. Returns ErrNotFound in either case.
func (s *Store) Rebind(ctx context.Context, playerID, originTableID, newTableID string) error {
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: playerID},
			"sk": &types.AttributeValueMemberS{Value: sk(originTableID)},
		},
		UpdateExpression:    aws.String("SET bound_table_id = :new"),
		ConditionExpression: aws.String("attribute_exists(pk) AND expires_at > :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":new": &types.AttributeValueMemberS{Value: newTableID},
			":now": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", s.now().Unix())},
		},
	})
	if err != nil {
		if dynamo.IsConditionFailed(err) {
			return ErrNotFound
		}
		return fmt.Errorf("entitlement: rebind: %w", err)
	}
	return nil
}
