package reconcile

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tablePending = "poker_pending_cashouts"
	pendingSK    = "pending"
)

type PendingCashout struct {
	ID             string   `dynamodbav:"id" json:"id"`
	PlayerID       string   `dynamodbav:"player_id" json:"player_id"`
	Amount         int64    `dynamodbav:"amount" json:"amount"`
	CurrencyMode   string   `dynamodbav:"currency_mode" json:"currency_mode"` // "sandbox" | "real"
	HoldIDs        []string `dynamodbav:"hold_ids" json:"hold_ids"`
	TableRef       string   `dynamodbav:"table_ref" json:"table_ref"`
	IdempotencyKey string   `dynamodbav:"idempotency_key" json:"idempotency_key"`
	RecordedAt     string   `dynamodbav:"recorded_at" json:"recorded_at"`
	Resolved       bool     `dynamodbav:"resolved" json:"resolved"`
}

type PendingStore struct {
	db   *dynamodb.Client
	env  string
	base dynamo.Base
}

func NewPendingStore(db *dynamodb.Client, env string) *PendingStore {
	return &PendingStore{
		db:   db,
		env:  env,
		base: dynamo.NewBase(db, env, tablePending),
	}
}

func (s *PendingStore) Record(ctx context.Context, p PendingCashout) error {
	if p.RecordedAt == "" {
		p.RecordedAt = dynamo.NowStr()
	}
	item, err := dynamo.Encode(struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
		PendingCashout
	}{PK: p.ID, SK: pendingSK, PendingCashout: p})
	if err != nil {
		return fmt.Errorf("reconcile: encode: %w", err)
	}
	return s.base.PutItem(ctx, item)
}

func (s *PendingStore) MarkResolved(ctx context.Context, id string) error {
	sk := pendingSK
	_, err := s.base.UpdateItem(ctx, id, &sk, map[string]any{"resolved": true})
	if err != nil {
		return fmt.Errorf("reconcile: mark resolved: %w", err)
	}
	return nil
}

func (s *PendingStore) ListUnresolved(ctx context.Context, olderThan time.Duration) ([]PendingCashout, error) {
	tableName := dynamo.TableName(s.env, tablePending)
	out, err := s.db.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		return nil, fmt.Errorf("reconcile: scan: %w", err)
	}
	cutoff := time.Now().Add(-olderThan)
	res := make([]PendingCashout, 0, len(out.Items))
	for _, item := range out.Items {
		p, err := dynamo.Decode[PendingCashout](item)
		if err != nil || p == nil {
			continue
		}
		if p.Resolved {
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
	return res, nil
}
