package reconcile

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tablePending = "poker_pending_cashouts"
	pendingSK    = "pending"
	pendingGSI   = "gsi_status"
	pendingOpen  = "open"
)

// Kind values for PendingCashout.Kind. Empty string means KindCashout, for
// backward compatibility with entries recorded before this field existed.
const (
	KindCashout  = "cashout"
	KindFeeDebit = "fee_debit"
)

type PendingCashout struct {
	ID           string `dynamodbav:"id" json:"id"`
	PlayerID     string `dynamodbav:"player_id" json:"player_id"`
	Amount       int64  `dynamodbav:"amount" json:"amount"`
	CurrencyMode string `dynamodbav:"currency_mode" json:"currency_mode"` // "sandbox" | "real"
	// Kind distinguishes what this pending entry retries. Empty/KindCashout:
	// credit a player's final stack back to their wallet (the original use of
	// this store). KindFeeDebit: charge the fixed real-money table-entry fee
	// that failed after the player was already seated (buyin.Service.BuyIn)
	// — same retry-until-resolved shape, opposite direction of money movement.
	Kind           string   `dynamodbav:"kind,omitempty" json:"kind,omitempty"`
	HoldIDs        []string `dynamodbav:"hold_ids" json:"hold_ids"`
	TableRef       string   `dynamodbav:"table_ref" json:"table_ref"`
	IdempotencyKey string   `dynamodbav:"idempotency_key" json:"idempotency_key"`
	RecordedAt     string   `dynamodbav:"recorded_at" json:"recorded_at"`
	Resolved       bool     `dynamodbav:"resolved" json:"resolved"`
	GSIStatus      string   `dynamodbav:"gsi_status,omitempty" json:"-"`
}

type PendingStore struct {
	db            *dynamodb.Client
	env           string
	base          dynamo.Base
	scanPageLimit int32
}

func NewPendingStore(db *dynamodb.Client, env string) *PendingStore {
	return &PendingStore{
		db:   db,
		env:  env,
		base: dynamo.NewBase(db, env, tablePending),
	}
}

func (s *PendingStore) BuildRecordTx(p PendingCashout) (types.TransactWriteItem, error) {
	if p.RecordedAt == "" {
		p.RecordedAt = dynamo.NowStr()
	}
	p.GSIStatus = pendingOpen
	item, err := dynamo.Encode(struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
		PendingCashout
	}{PK: p.ID, SK: pendingSK, PendingCashout: p})
	if err != nil {
		return types.TransactWriteItem{}, fmt.Errorf("reconcile: encode: %w", err)
	}
	return s.base.BuildPutTxItemIfAbsent(item), nil
}

// Record persists an immutable recovery obligation. Replays are idempotent;
// unlike the old PutItem path, a later caller can never overwrite its amount,
// wallet mode, hold IDs, or idempotency key.
func (s *PendingStore) Record(ctx context.Context, p PendingCashout) error {
	item, err := s.BuildRecordTx(p)
	if err != nil {
		return err
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{item}); err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil
		}
		return fmt.Errorf("reconcile: record: %w", err)
	}
	return nil
}

// Get reads back a single recovery row by its ID, or (nil, nil) if none
// exists. Used to confirm whether a specific settlement obligation (e.g. a
// real-money entry-fee charge, keyed off its owning entitlement's persisted
// CreatedAt) actually resolved, instead of trusting a related row's mere
// existence as proof the money moved.
func (s *PendingStore) Get(ctx context.Context, id string) (*PendingCashout, error) {
	item, err := s.base.GetItem(ctx, id, pendingSK)
	if err != nil {
		return nil, fmt.Errorf("reconcile: get: %w", err)
	}
	if len(item) == 0 {
		return nil, nil
	}
	p, err := dynamo.Decode[PendingCashout](item)
	if err != nil {
		return nil, fmt.Errorf("reconcile: decode: %w", err)
	}
	return p, nil
}

func (s *PendingStore) MarkResolved(ctx context.Context, id string) error {
	sk := pendingSK
	_, err := s.base.UpdateItem(ctx, id, &sk, map[string]any{
		"resolved":    true,
		"resolved_at": dynamo.NowStr(),
		"ttl":         time.Now().Add(30 * 24 * time.Hour).Unix(),
		"gsi_status":  nil,
	})
	if err != nil {
		return fmt.Errorf("reconcile: mark resolved: %w", err)
	}
	return nil
}

func (s *PendingStore) ListUnresolved(ctx context.Context, olderThan time.Duration) ([]PendingCashout, error) {
	cutoff := time.Now().Add(-olderThan)
	res := make([]PendingCashout, 0)
	var startKey map[string]types.AttributeValue
	for {
		limit := int(s.scanPageLimit)
		if limit <= 0 {
			limit = 100
		}
		out, err := s.base.QueryGSI(ctx, pendingGSI, "gsi_status", pendingOpen, limit, startKey)
		if err != nil {
			return nil, fmt.Errorf("reconcile: query unresolved: %w", err)
		}
		for _, item := range out.Items {
			p, err := dynamo.Decode[PendingCashout](item)
			if err != nil || p == nil || p.Resolved {
				continue
			}
			if olderThan > 0 && p.RecordedAt != "" {
				recordedAt, err := time.Parse(time.RFC3339Nano, p.RecordedAt)
				if err == nil && recordedAt.After(cutoff) {
					continue
				}
			}
			res = append(res, *p)
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		startKey = out.LastEvaluatedKey
	}
	return res, nil
}
