package tablestore

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

const (
	tableState        = "poker_table_state"
	tableActionLog    = "poker_action_log"
	tableActionGuards = "poker_action_guards"
	// tableStateHistory is an append-only audit copy of every hand's final
	// state, written just before the table resets for the next hand — never
	// read back for correctness, only for auditing/debugging (unlike
	// poker_table_state, which is the single authoritative item per table).
	tableStateHistory = "poker_table_state_history"

	// gsiActiveLastAction is the sparse index cmd/tablecleanup queries for
	// stale tables — see the gsi_active comment below and
	// cdk/lib/dynamodb-stack.ts's matching addGlobalSecondaryIndex call.
	gsiActiveLastAction = "gsi_active_last_action"

	// guardTTLDays mirrors ctech-wallet's idemTTLDays
	// (ctech-wallet/api/internal/repositories/wallet.go:19) — a guard only
	// needs to outlive plausible client retries, not forever.
	guardTTLDays = 7

	// stateTTLDays bounds how long the authoritative per-table item and its
	// audit-history snapshots linger after the table goes quiet.
	// poker_table_state / poker_table_state_history are ephemeral: a live
	// table refreshes its ttl on every CommitAction (last_action_at moves with
	// it), cmd/tablecleanup archives idle tables long before this fires, and a
	// rejoin re-seeds a fresh item. Without a ttl a table that was only ever
	// seeded — or an old history snapshot — sat in DynamoDB forever (and,
	// pre-2026-09-03, in PITR continuous backups too). 7 days mirrors
	// guardTTLDays: long enough to investigate a fresh incident, short enough
	// that dead tables do not accumulate. See
	// docs/specs/2026-09-03-next-hand-rearm-storm.md.
	stateTTLDays = 7

	// logTTLDays bounds how long an audit-log entry stays in the hot
	// DynamoDB table before TTL reaps it. Nothing is lost when that
	// happens: the archiver Lambda (cdk/lib/archiver-stack.ts) ships every
	// entry to S3 on insert, independent of and well before its eventual
	// TTL expiry — DynamoDB serves the recent window, S3 is the
	// indefinite archive.
	logTTLDays = 90

	// gsiActiveValue is the sparse gsi_active partition key value every live
	// table carries — MarkArchived REMOVEs this attribute so an archived
	// table drops out of gsi_active_last_action entirely, the same
	// sparse-index convention as roomstore's gsi_public (roomstore/dynamo.go).
	gsiActiveValue = "1"
)

// timeNowFunc is overridden in tests that need a deterministic TTL value.
var timeNowFunc = time.Now

// logSK renders an action log row's sort key from its table version. The zero
// padding is what keeps DynamoDB's lexicographic sort equal to numeric order.
func logSK(version int) string { return fmt.Sprintf("%010d", version) }

// sleepFunc is overridden in tests so a retry backoff costs no wall time.
var sleepFunc = time.Sleep

// Store persists the one authoritative item per table, an audit log, and the
// idempotency guards that back CommitAction's duplicate-action_id rejection.
type Store struct {
	db      *dynamodb.Client
	env     string
	state   dynamo.Base
	log     dynamo.Base
	guards  dynamo.Base
	history dynamo.Base
	// breaker is the per-table write guard in front of CommitAction — see
	// breaker.go. It is defence in depth for every caller of this one shared
	// write sink, not a replacement for any caller's own guard.
	breaker *commitBreaker
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{
		db:      db,
		env:     env,
		state:   dynamo.NewBase(db, env, tableState),
		log:     dynamo.NewBase(db, env, tableActionLog),
		guards:  dynamo.NewBase(db, env, tableActionGuards),
		history: dynamo.NewBase(db, env, tableStateHistory),
		breaker: newCommitBreaker(),
	}
}

// SaveTableStateHistory appends tableID's current state to the audit history
// table, keyed by table ID and the unix-second timestamp it was captured at.
// Plain PutItem, not transactional: this is a best-effort audit copy, never
// the authoritative item, so callers should treat a failure here as
// non-fatal (see table.Actor.saveHandHistorySnapshot).
func (s *Store) SaveTableStateHistory(ctx context.Context, tableID string, unixSeconds int64, state hand.State) error {
	item, err := dynamo.Encode(struct {
		PK    string     `dynamodbav:"pk"`
		SK    string     `dynamodbav:"sk"`
		State hand.State `dynamodbav:"state"`
		TTL   int64      `dynamodbav:"ttl"`
	}{
		PK: tableID, SK: fmt.Sprintf("%d", unixSeconds), State: state,
		TTL: timeNowFunc().Add(stateTTLDays * 24 * time.Hour).Unix(),
	})
	if err != nil {
		return fmt.Errorf("tablestore: encode history snapshot: %w", err)
	}
	return s.history.PutItem(ctx, item)
}

// SeedTable creates a table's very first state item at version 1. It is a
// conditional create (attribute_not_exists(pk)) so a first-touch race between
// two instances can never clobber an already-seeded table (M4). If the table
// already exists the conditional fails and we treat it as a successful no-op.
func (s *Store) SeedTable(ctx context.Context, tableID string, state hand.State) error {
	item, err := dynamo.Encode(struct {
		PK           string     `dynamodbav:"pk"`
		Version      int        `dynamodbav:"version"`
		State        hand.State `dynamodbav:"state"`
		LastActionAt int64      `dynamodbav:"last_action_at"`
		GSIActive    string     `dynamodbav:"gsi_active"`
		TTL          int64      `dynamodbav:"ttl"`
	}{
		PK: tableID, Version: 1, State: state, LastActionAt: timeNowFunc().Unix(), GSIActive: gsiActiveValue,
		TTL: timeNowFunc().Add(stateTTLDays * 24 * time.Hour).Unix(),
	})
	if err != nil {
		return fmt.Errorf("tablestore: encode seed state: %w", err)
	}
	tx := s.state.BuildPutTxItemIfAbsent(item)
	if err := s.state.TransactWrite(ctx, []types.TransactWriteItem{tx}); err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil // already seeded
		}
		return fmt.Errorf("%w: seed table: %w", ErrUnavailable, err)
	}
	return nil
}

// LoadTable always does a strongly consistent read. A plain
// eventually-consistent GetItem here would let ensureLoaded
// (internal/table/actor.go) observe "no item" for a table this same request
// just seeded/committed a moment earlier on a different replica — any
// instance may serve any table with no proxying to a lease holder
// (ARCHITECTURE.md §2), so that race is routine, not a corner case, and it
// surfaces to players as a wrongly-rejected "no state seeded" action error.
//
// The consistent read runs through BatchGetItem, NOT TransactGetItems. A
// transactional read of the table item conflicts with any CommitAction
// TransactWrite touching that same item and fails the whole read with
// TransactionCanceledException[TransactionConflict] — which every caller
// surfaces as an outright rejection, since a load error aborts the command
// before its own retry/precondition logic ever runs. The post-hand window is
// exactly when that collides most (outcome log entries, the next-hand
// countdown commit and the hand hooks all write the item while players click
// "show cards" / "rabbit hunt"), so those commands failed on essentially
// every attempt. BatchGetItem with ConsistentRead gives the same freshness
// with no transaction semantics to conflict with — and at half the read
// capacity of a transactional get.
func (s *Store) LoadTable(ctx context.Context, tableID string) (*StoredTable, error) {
	key := map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: tableID}}
	// BatchGetItem is the one read path that can come back "fine, but I did
	// not read it" (UnprocessedKeys, under throttling). Treating that as an
	// empty result would report a live table as unseeded, so retry it.
	for attempt := range 3 {
		out, err := s.state.BatchGetItemRaw(ctx, &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				s.state.TableName: {Keys: []map[string]types.AttributeValue{key}, ConsistentRead: aws.Bool(true)},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("%w: get table: %w", ErrUnavailable, err)
		}
		if items := out.Responses[s.state.TableName]; len(items) > 0 {
			return dynamo.Decode[StoredTable](items[0])
		}
		if len(out.UnprocessedKeys) == 0 {
			return nil, nil // genuinely absent
		}
		sleepFunc(time.Duration(20<<attempt) * time.Millisecond)
	}
	return nil, fmt.Errorf("%w: get table %s: still unprocessed after retries", ErrUnavailable, tableID)
}

// CommitAction atomically bumps tableID's version (guarded by
// expectedVersion), records entry in the audit log, unconditionally includes
// every item in extra (e.g. buyin's settlement/pending-cashout row — it must
// land with the state change or not at all), and — when actionID is
// non-empty — writes an idempotency guard so a replayed action_id fails the
// transaction instead of being re-applied. extra's inclusion is deliberately
// independent of actionID: LeaveCmd never carries one, so gating extra behind
// the actionID branch silently dropped it from every leave (see
// docs/plans/2026-08-03-leave-settlement-atomicity.md). Mirrors
// ctech-wallet/api/internal/repositories/wallet.go's mutate/resolveTxErr
// shape: on a failed condition, re-read the guard to disambiguate a version
// race from a duplicate submission.
func (s *Store) CommitAction(ctx context.Context, tableID, handID, actionID string, expectedVersion int, newState hand.State, activity TableActivity, turnDeadlineUnixMs, nextHandDeadlineUnixMs int64, entry ActionLogEntry, extra ...types.TransactWriteItem) error {
	entry.Timestamp = timeNowFunc().UnixMilli()
	stateItem, err := dynamo.Encode(struct {
		State hand.State `dynamodbav:"state"`
	}{State: newState})
	if err != nil {
		return fmt.Errorf("tablestore: encode state: %w", err)
	}
	stateAV := stateItem["state"]
	activityItem, err := dynamo.Encode(struct {
		Activity TableActivity `dynamodbav:"activity"`
	}{Activity: activity})
	if err != nil {
		return fmt.Errorf("tablestore: encode activity: %w", err)
	}

	values := map[string]types.AttributeValue{
		":newVersion":   mustN(expectedVersion + 1),
		":expected":     mustN(expectedVersion),
		":handID":       &types.AttributeValueMemberS{Value: handID},
		":state":        stateAV,
		":activity":     activityItem["activity"],
		":turnDeadline": mustN(int(turnDeadlineUnixMs)),
		// Written on every commit, zero included: a table leaving Complete must
		// clear the countdown, not inherit the previous hand's expiry.
		":nextHandDeadline": mustN(int(nextHandDeadlineUnixMs)),
		":lastActionAt":     mustN(int(timeNowFunc().Unix())),
		// Refreshed on every commit so a live table's item never expires; once
		// a table goes quiet the ttl stops moving forward and DynamoDB reaps
		// it after stateTTLDays (see the const's doc). "ttl" is a reserved word
		// in expressions, hence the #ttl alias below (the item encoders that
		// PutItem the same attribute do not need one).
		":ttl": mustN(int(timeNowFunc().Add(stateTTLDays * 24 * time.Hour).Unix())),
	}
	names := map[string]string{
		"#version": "version",
		"#state":   "state",
		"#ttl":     "ttl",
	}
	stateTx := s.state.BuildRawUpdateTxItem(tableID, nil,
		"SET #version = :newVersion, hand_id = :handID, #state = :state, activity = :activity, "+
			"turn_deadline_unix_ms = :turnDeadline, next_hand_deadline_unix_ms = :nextHandDeadline, "+
			"last_action_at = :lastActionAt, #ttl = :ttl",
		"attribute_exists(pk) AND #version = :expected", names, values)

	logItem, err := dynamo.Encode(struct {
		PK  string `dynamodbav:"pk"`
		SK  string `dynamodbav:"sk"`
		TTL int64  `dynamodbav:"ttl"`
		ActionLogEntry
	}{
		PK: tableID + "#" + handID, SK: logSK(entry.Version),
		TTL:            timeNowFunc().Add(logTTLDays * 24 * time.Hour).Unix(),
		ActionLogEntry: entry,
	})
	if err != nil {
		return fmt.Errorf("tablestore: encode log entry: %w", err)
	}
	logTx := s.log.BuildPutTxItem(logItem)

	// extra (e.g. buyin's settlement/pending-cashout row) must land in the
	// same transaction unconditionally — it has nothing to do with the
	// actionID idempotency guard below. A prior version of this function
	// only appended extra inside the `actionID != ""` branch, but LeaveCmd
	// (internal/table/commands.go) never carries an ActionID, so every
	// leave — manual cash-out or system kick/AFK — silently dropped its
	// settlement row from the transaction: the seat was removed but the
	// poker_pending_cashouts safety net never got written, leaving no
	// recovery trail if buyin.Service's own follow-up credit call then
	// failed. See docs/plans/2026-08-03-leave-settlement-atomicity.md.
	items := []types.TransactWriteItem{stateTx, logTx}
	items = append(items, extra...)
	if actionID != "" {
		guardItem, err := dynamo.Encode(struct {
			PK  string `dynamodbav:"pk"`
			TTL int64  `dynamodbav:"ttl"`
			// Version is the action log row's sort key for this same action.
			// Recording it here turns FindActionByID (report evidence lookup)
			// into two GetItems instead of a query over the whole hand
			// partition — see its doc comment.
			Version int `dynamodbav:"version"`
		}{
			PK: tableID + "#" + handID + "#" + actionID, TTL: timeNowFunc().Add(guardTTLDays * 24 * time.Hour).Unix(),
			Version: entry.Version,
		})
		if err != nil {
			return fmt.Errorf("tablestore: encode guard: %w", err)
		}
		items = append(items, s.guards.BuildPutTxItemIfAbsent(guardItem))
	}

	// Last gate before the write itself (issue #207): the per-table budget is
	// checked here, not at the top of the function, so a transaction that
	// never gets built spends no budget and can never leave a half-open
	// probe in flight. On a closed budget nothing reaches DynamoDB and the
	// caller gets an ErrUnavailable-flavoured error, which every actor
	// handler treats as "abort this command" rather than reload-and-retry.
	if err := s.breaker.allow(tableID, entry.Action); err != nil {
		return err
	}
	if err := s.state.TransactWrite(ctx, items); err != nil {
		resolved := s.resolveCommitErr(ctx, tableID, handID, actionID, err)
		s.breaker.record(tableID, entry.Action, resolved)
		return resolved
	}
	s.breaker.record(tableID, entry.Action, nil)
	return nil
}

// resolveCommitErr disambiguates a failed transaction: an already-present
// guard means a duplicate action_id; otherwise the state item's version
// condition must have failed.
func (s *Store) LoadActionsSince(ctx context.Context, tableID, handID string, afterSeq int) ([]ActionLogEntry, error) {
	pk := tableID + "#" + handID
	// Oldest first: Seq (below) numbers entries by their position in this
	// result when the stored record predates the Seq field, and every caller
	// (hand history, hand-share replay) presents actions in that same order —
	// ScanIndexForward defaults to false (DynamoDB's newest-first), which
	// silently reversed both the sequence numbers and the replay itself.
	result, err := s.log.Query(ctx, dynamo.QueryOpts{PK: pk, ScanIndexForward: true})
	if err != nil {
		return nil, fmt.Errorf("tablestore: load actions: %w", err)
	}
	out := make([]ActionLogEntry, 0, len(result.Items))
	for i, item := range result.Items {
		e, err := dynamo.Decode[ActionLogEntry](item)
		if err != nil || e == nil {
			continue
		}
		if e.Seq == 0 {
			e.Seq = i + 1
		}
		if e.Seq > afterSeq {
			out = append(out, *e)
		}
	}
	return out, nil
}

// FindActionByID resolves evidence only inside the caller-supplied table and
// hand partition. It never scans the global action log, and it returns the
// server-persisted player/message/reaction rather than trusting report input.
//
// The idempotency guard CommitAction already writes for this exact
// (table, hand, action) records the log row's version — which IS that row's
// sort key — so the lookup is two GetItems by key. It used to read and decode
// every action of the hand to answer one report (#221). Guards expire after
// guardTTLDays while the log lives for logTTLDays, so a report filed against
// an older action falls back to the scan-free partition query below.
func (s *Store) FindActionByID(ctx context.Context, tableID, handID, actionID string) (*ActionLogEntry, error) {
	if tableID == "" || handID == "" || actionID == "" {
		return nil, nil
	}
	guard, err := s.guards.GetItem(ctx, tableID+"#"+handID+"#"+actionID)
	if err != nil {
		return nil, fmt.Errorf("tablestore: load action guard: %w", err)
	}
	version, ok := guard["version"].(*types.AttributeValueMemberN)
	if !ok {
		// No guard at all (expired), or one written before guards carried the
		// version.
		return s.findActionByFilter(ctx, tableID, handID, actionID)
	}
	seq, err := strconv.Atoi(version.Value)
	if err != nil {
		return s.findActionByFilter(ctx, tableID, handID, actionID)
	}
	item, err := s.log.GetItem(ctx, tableID+"#"+handID, logSK(seq))
	if err != nil {
		return nil, fmt.Errorf("tablestore: load action by version: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	entry, err := dynamo.Decode[ActionLogEntry](item)
	if err != nil || entry == nil || entry.ActionID != actionID {
		return nil, err
	}
	return entry, nil
}

// findActionByFilter is the pre-guard-version fallback: one query of this
// hand's partition with a server-side action_id filter, so DynamoDB returns
// the single matching row instead of the whole hand.
func (s *Store) findActionByFilter(ctx context.Context, tableID, handID, actionID string) (*ActionLogEntry, error) {
	var startKey map[string]types.AttributeValue
	for {
		result, err := s.log.Query(ctx, dynamo.QueryOpts{
			PK: tableID + "#" + handID, ScanIndexForward: true, ExclusiveStartKey: startKey,
			FilterField: "action_id", FilterValue: actionID,
		})
		if err != nil {
			return nil, fmt.Errorf("tablestore: find action: %w", err)
		}
		for _, item := range result.Items {
			entry, err := dynamo.Decode[ActionLogEntry](item)
			if err != nil || entry == nil {
				continue
			}
			if entry.TableID == tableID && entry.HandID == handID {
				return entry, nil
			}
		}
		if len(result.LastEvaluatedKey) == 0 {
			return nil, nil
		}
		startKey = result.LastEvaluatedKey
	}
}

// QueryStaleActive returns every still-active table (gsi_active present)
// whose last_action_at is older than olderThanUnix, oldest first — the read
// side of cmd/tablecleanup's sweep. Queries gsi_active_last_action; never
// scans (api-commons/dynamo package doc: "get_item > query > scan").
// dynamo.Base.QueryGSI only supports equality conditions, not this
// last_action_at < cutoff range, hence the raw *dynamodb.Client query.
func (s *Store) QueryStaleActive(ctx context.Context, olderThanUnix int64, limit int) ([]StoredTable, error) {
	out, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(dynamo.TableName(s.env, tableState)),
		IndexName:              aws.String(gsiActiveLastAction),
		KeyConditionExpression: aws.String("gsi_active = :active AND last_action_at < :cutoff"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":active": &types.AttributeValueMemberS{Value: gsiActiveValue},
			":cutoff": mustN(int(olderThanUnix)),
		},
		ScanIndexForward: aws.Bool(true), // oldest last_action_at first
		Limit:            aws.Int32(int32(limit)),
	})
	if err != nil {
		return nil, fmt.Errorf("tablestore: query stale active: %w", err)
	}
	result := make([]StoredTable, 0, len(out.Items))
	for _, keyItem := range out.Items {
		id, ok := keyItem["pk"].(*types.AttributeValueMemberS)
		if !ok {
			continue
		}
		full, err := s.LoadTable(ctx, id.Value)
		if err != nil {
			return nil, fmt.Errorf("tablestore: load stale table %s: %w", id.Value, err)
		}
		if full != nil {
			result = append(result, *full)
		}
	}
	return result, nil
}

// MarkArchived flips tableID to archived and removes it from
// gsi_active_last_action, guarded by expectedVersion — the same
// version-equality discipline as CommitAction. If another instance
// committed an action on this table since the caller's stale-active query
// ran, this fails with ErrVersionConflict and the caller should simply skip
// archiving it this pass (it is no longer stale).
func (s *Store) MarkArchived(ctx context.Context, tableID string, expectedVersion int) error {
	_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(dynamo.TableName(s.env, tableState)),
		Key:                 map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: tableID}},
		UpdateExpression:    aws.String("SET archived = :true REMOVE gsi_active"),
		ConditionExpression: aws.String("attribute_exists(pk) AND #version = :expected"),
		ExpressionAttributeNames: map[string]string{
			"#version": "version",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true":     &types.AttributeValueMemberBOOL{Value: true},
			":expected": mustN(expectedVersion),
		},
	})
	if err != nil {
		if dynamo.IsConditionFailed(err) {
			return ErrVersionConflict
		}
		return fmt.Errorf("tablestore: mark archived: %w", err)
	}
	return nil
}

func (s *Store) resolveCommitErr(ctx context.Context, tableID, handID, actionID string, txErr error) error {
	if !dynamo.IsConditionFailed(txErr) {
		return fmt.Errorf("%w: commit: %w", ErrUnavailable, txErr)
	}
	if actionID != "" {
		item, err := s.guards.GetItem(ctx, tableID+"#"+handID+"#"+actionID)
		if err != nil {
			return fmt.Errorf("%w: check guard: %w", ErrUnavailable, err)
		}
		if item != nil {
			return ErrDuplicateAction
		}
	}
	return ErrVersionConflict
}

func mustN(v int) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v)}
}
