package reconcile

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
	tablePending         = "poker_pending_cashouts"
	pendingSK            = "pending"
	pendingGSI           = "gsi_status"
	pendingOpen          = "open"
	pendingManualReview  = "manual_review"
	playerSettlementsGSI = "gsi_player_settlements"
)

// MaxAttempts is the number of failed reconcile passes a single pending entry
// tolerates before it is quarantined (gsi_status -> "manual_review") and lifted
// out of the normal sweep. cmd/reconcile escalates a quarantined entry to the
// Lambda DLQ so it pages instead of retrying silently every 5 minutes forever.
const MaxAttempts = 5

// maxLastErrorLen caps the persisted LastError string so a verbose wrapped
// error can never bloat the row.
const maxLastErrorLen = 500

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
	// ResolvedAt is set by MarkResolved (dynamo.NowStr()). It backs the
	// player-facing settlement timeline's resolved_at field (#333) — never
	// set on an unresolved entry.
	ResolvedAt string `dynamodbav:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	GSIStatus  string `dynamodbav:"gsi_status,omitempty" json:"-"`
	// Attempts counts how many reconcile passes have tried and failed to
	// resolve this entry. LastAttemptAt/LastError carry the context of the
	// most recent failure. Set by RecordFailedAttempt; once Attempts reaches
	// MaxAttempts the entry is quarantined out of the sweep.
	Attempts      int    `dynamodbav:"attempts,omitempty" json:"attempts,omitempty"`
	LastAttemptAt string `dynamodbav:"last_attempt_at,omitempty" json:"last_attempt_at,omitempty"`
	LastError     string `dynamodbav:"last_error,omitempty" json:"last_error,omitempty"`
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

// Get reads one recovery obligation by its ID, or (nil, nil) when no such
// row exists. It is the "has this money movement actually completed?" read:
// buyin.Service.confirmFeeCharged uses it so the mere existence of a
// table-entry entitlement can never stand in for a settled fee debit.
func (s *PendingStore) Get(ctx context.Context, id string) (*PendingCashout, error) {
	item, err := s.base.GetItem(ctx, id, pendingSK)
	if err != nil {
		return nil, fmt.Errorf("reconcile: get pending: %w", err)
	}
	if len(item) == 0 {
		return nil, nil
	}
	p, err := dynamo.Decode[PendingCashout](item)
	if err != nil {
		return nil, fmt.Errorf("reconcile: decode pending: %w", err)
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

// RecordFailedAttempt increments the attempt counter on a pending entry and
// records the failure context. It returns the new attempt count and whether the
// entry was quarantined by this call: once the count reaches MaxAttempts the
// row's gsi_status flips to "manual_review", which removes it from ListUnresolved
// (that query is keyed on gsi_status = "open") so the sweep stops retrying a
// poison entry — cmd/reconcile then escalates it to the Lambda DLQ.
func (s *PendingStore) RecordFailedAttempt(ctx context.Context, e PendingCashout, cause error) (attempts int, quarantined bool, err error) {
	attempts = e.Attempts + 1
	quarantined = attempts >= MaxAttempts

	lastErr := ""
	if cause != nil {
		lastErr = cause.Error()
		if len(lastErr) > maxLastErrorLen {
			lastErr = lastErr[:maxLastErrorLen]
		}
	}
	updates := map[string]any{
		"attempts":        attempts,
		"last_attempt_at": dynamo.NowStr(),
		"last_error":      lastErr,
	}
	if quarantined {
		updates["gsi_status"] = pendingManualReview
	}

	sk := pendingSK
	ok, uErr := s.base.UpdateItem(ctx, e.ID, &sk, updates)
	if uErr != nil {
		return attempts, false, fmt.Errorf("reconcile: record failed attempt for %s: %w", e.ID, uErr)
	}
	if !ok {
		return attempts, false, fmt.Errorf("reconcile: record failed attempt for %s: entry not found", e.ID)
	}
	return attempts, quarantined, nil
}

// Settlement status values shown to the player — never the raw internal
// gsi_status/Attempts machinery. manual_review (Attempts >= MaxAttempts) is
// deliberately folded into the same generic vocabulary a player can act on,
// never LastError's raw failure text.
const (
	SettlementStatusPending      = "pending"
	SettlementStatusResolved     = "resolved"
	SettlementStatusManualReview = "manual_review"
)

// SettlementView is the only shape ever returned to a player for their own
// financial-adjustment timeline (#333). It deliberately omits HoldIDs,
// IdempotencyKey, LastError and GSIStatus/Attempts — none of that is safe or
// useful to show a player, and the fee itself is always a fixed amount, never
// a percentage, so nothing here should ever be phrased as a rake share.
type SettlementView struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Amount       int64  `json:"amount"`
	CurrencyMode string `json:"currency_mode"`
	TableRef     string `json:"table_ref,omitempty"`
	Status       string `json:"status"`
	RecordedAt   string `json:"recorded_at"`
	ResolvedAt   string `json:"resolved_at,omitempty"`
}

func (p PendingCashout) SettlementView() SettlementView {
	status := SettlementStatusPending
	switch {
	case p.Resolved:
		status = SettlementStatusResolved
	case p.GSIStatus == pendingManualReview:
		status = SettlementStatusManualReview
	}
	kind := p.Kind
	if kind == "" {
		kind = KindCashout
	}
	return SettlementView{ID: p.ID, Kind: kind, Amount: p.Amount, CurrencyMode: p.CurrencyMode,
		TableRef: p.TableRef, Status: status, RecordedAt: p.RecordedAt, ResolvedAt: p.ResolvedAt}
}

// ListForPlayer answers a player's own settlement timeline — pending
// cash-outs, fee-debit retries, resolutions — as a single Query against the
// gsi_player_settlements GSI, never a Scan. Newest first. Populated by
// Record/BuildRecordTx, which already write player_id/recorded_at on every
// entry; pre-existing rows created before this GSI shipped are invisible to
// it until their own 30-day TTL reaps them (no backfill).
func (s *PendingStore) ListForPlayer(ctx context.Context, playerID string, limit int, startKey map[string]types.AttributeValue) ([]PendingCashout, map[string]types.AttributeValue, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	out, err := s.base.QueryRaw(ctx, &dynamodb.QueryInput{
		IndexName:              aws.String(playerSettlementsGSI),
		KeyConditionExpression: aws.String("player_id = :pid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pid": &types.AttributeValueMemberS{Value: playerID},
		},
		ScanIndexForward: aws.Bool(false), Limit: aws.Int32(int32(limit)), ExclusiveStartKey: startKey,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("reconcile: list for player: %w", err)
	}
	items := make([]PendingCashout, 0, len(out.Items))
	for _, item := range out.Items {
		p, decodeErr := dynamo.Decode[PendingCashout](item)
		if decodeErr != nil {
			return nil, nil, fmt.Errorf("reconcile: decode: %w", decodeErr)
		}
		items = append(items, *p)
	}
	return items, out.LastEvaluatedKey, nil
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
