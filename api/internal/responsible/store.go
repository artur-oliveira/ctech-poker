package responsible

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	// TableResponsible: pk = player id (or SELFEXCL#<cpf hmac> for a carried
	// exclusion), sk = STATE | AUDIT#<ts>#<id> | INFLOW#<key>.
	TableResponsible = "poker_responsible"

	carriedPrefix = "SELFEXCL#"
	auditPrefix   = "AUDIT#"
	inflowPrefix  = "INFLOW#"

	// inflowMarkerTTL outlives any retry of a buy-in by a wide margin.
	inflowMarkerTTL = 30 * 24 * time.Hour
)

// ErrConflict means a conditional write lost to a concurrent writer; the caller
// re-reads and retries.
var ErrConflict = errors.New("responsible: concurrent update")

// Store persists the state, its append-only audit trail and the carried exclusions.
type Store interface {
	// Get returns the player's state, or nil when there is none yet.
	Get(ctx context.Context, userID string) (*State, error)
	// Put writes st conditioned on the stored version being prevVersion (0 = the
	// item must not exist). It bumps st.Version on success.
	Put(ctx context.Context, st *State, prevVersion int64) error
	// CommitInflow does what Put does and, in the same transaction, claims the
	// idempotency marker for key. When the marker already exists the inflow was
	// counted before: it returns counted=false, err=nil and writes nothing.
	CommitInflow(ctx context.Context, st *State, prevVersion int64, key string, now time.Time) (counted bool, err error)
	// Counted reports whether the inflow idempotency marker for key exists.
	Counted(ctx context.Context, userID, key string) (bool, error)
	// Audit appends one immutable event.
	Audit(ctx context.Context, userID, event, detail string, now time.Time) error
	// PutCarried keeps ex until expiresAt; the one that lasts longer wins.
	PutCarried(ctx context.Context, cpfHMAC string, ex SelfExclusion, expiresAt time.Time) error
	// GetCarried returns the live exclusion carried for cpfHMAC, or nil.
	GetCarried(ctx context.Context, cpfHMAC string, now time.Time) (*SelfExclusion, error)
}

// ---- DynamoDB ----

type DynamoStore struct{ base dynamo.Base }

func NewDynamoStore(db *dynamodb.Client, env string) *DynamoStore {
	return &DynamoStore{base: dynamo.NewBase(db, env, TableResponsible)}
}

func (s *DynamoStore) Get(ctx context.Context, userID string) (*State, error) {
	item, err := s.base.GetItem(ctx, userID, StateSK)
	if err != nil || item == nil {
		return nil, err
	}
	var st State
	if err := attributevalue.UnmarshalMap(item, &st); err != nil {
		return nil, fmt.Errorf("responsible: decode state: %w", err)
	}
	return &st, nil
}

func (s *DynamoStore) stateTx(st *State, prevVersion int64) (types.TransactWriteItem, error) {
	next := *st
	next.SK = StateSK
	next.Version = prevVersion + 1
	next.UpdatedAt = dynamo.NowStr()
	av, err := attributevalue.MarshalMap(next)
	if err != nil {
		return types.TransactWriteItem{}, err
	}
	put := &types.Put{TableName: aws.String(s.base.TableName), Item: av}
	if prevVersion == 0 {
		put.ConditionExpression = aws.String("attribute_not_exists(pk)")
	} else {
		put.ConditionExpression = aws.String("#v = :prev")
		put.ExpressionAttributeNames = map[string]string{"#v": "version"}
		put.ExpressionAttributeValues = map[string]types.AttributeValue{":prev": &types.AttributeValueMemberN{Value: strconv.FormatInt(prevVersion, 10)}}
	}
	return types.TransactWriteItem{Put: put}, nil
}

func (s *DynamoStore) Put(ctx context.Context, st *State, prevVersion int64) error {
	tx, err := s.stateTx(st, prevVersion)
	if err != nil {
		return err
	}
	if _, err := s.base.PutItemRaw(ctx, &dynamodb.PutItemInput{
		Item: tx.Put.Item, ConditionExpression: tx.Put.ConditionExpression,
		ExpressionAttributeNames: tx.Put.ExpressionAttributeNames, ExpressionAttributeValues: tx.Put.ExpressionAttributeValues,
	}); err != nil {
		if dynamo.IsConditionFailed(err) {
			return ErrConflict
		}
		return err
	}
	st.Version = prevVersion + 1
	return nil
}

func (s *DynamoStore) CommitInflow(ctx context.Context, st *State, prevVersion int64, key string, now time.Time) (bool, error) {
	stateTx, err := s.stateTx(st, prevVersion)
	if err != nil {
		return false, err
	}
	marker, err := attributevalue.MarshalMap(map[string]any{
		"pk": st.UserID, "sk": inflowPrefix + key, "ttl": now.Add(inflowMarkerTTL).Unix(),
	})
	if err != nil {
		return false, err
	}
	err = s.base.TransactWrite(ctx, []types.TransactWriteItem{stateTx, s.base.BuildPutTxItemIfAbsent(marker)})
	switch {
	case err == nil:
		st.Version = prevVersion + 1
		return true, nil
	case dynamo.IsConditionFailed(err) || dynamo.IsTransactionConflict(err):
		// Either the state moved under us, or this key was counted before.
		item, gerr := s.base.GetItem(ctx, st.UserID, inflowPrefix+key)
		if gerr != nil {
			return false, gerr
		}
		if item != nil {
			return false, nil
		}
		return false, ErrConflict
	}
	return false, err
}

func (s *DynamoStore) Counted(ctx context.Context, userID, key string) (bool, error) {
	item, err := s.base.GetItem(ctx, userID, inflowPrefix+key)
	return item != nil, err
}

func (s *DynamoStore) Audit(ctx context.Context, userID, event, detail string, now time.Time) error {
	av, err := attributevalue.MarshalMap(map[string]any{
		"pk": userID, "sk": auditPrefix + now.UTC().Format(time.RFC3339Nano) + "#" + uuid.NewString(),
		"event": event, "detail": detail, "at": now.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	return s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(av)})
}

func (s *DynamoStore) PutCarried(ctx context.Context, cpfHMAC string, ex SelfExclusion, expiresAt time.Time) error {
	av, err := attributevalue.MarshalMap(map[string]any{
		"pk": carriedPrefix + cpfHMAC, "sk": StateSK, "self_exclusion": ex, "ttl": expiresAt.Unix(),
	})
	if err != nil {
		return err
	}
	_, err = s.base.PutItemRaw(ctx, &dynamodb.PutItemInput{
		Item:                      av,
		ConditionExpression:       aws.String("attribute_not_exists(pk) OR #ttl < :ttl"),
		ExpressionAttributeNames:  map[string]string{"#ttl": "ttl"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":ttl": &types.AttributeValueMemberN{Value: strconv.FormatInt(expiresAt.Unix(), 10)}},
	})
	if dynamo.IsConditionFailed(err) {
		return nil // one that lasts longer is already stored
	}
	return err
}

func (s *DynamoStore) GetCarried(ctx context.Context, cpfHMAC string, now time.Time) (*SelfExclusion, error) {
	item, err := s.base.GetItem(ctx, carriedPrefix+cpfHMAC, StateSK)
	if err != nil || item == nil {
		return nil, err
	}
	var c struct {
		SelfExclusion SelfExclusion `dynamodbav:"self_exclusion"`
		TTL           int64         `dynamodbav:"ttl"`
	}
	if err := attributevalue.UnmarshalMap(item, &c); err != nil {
		return nil, err
	}
	if c.TTL <= now.Unix() {
		return nil, nil // TTL deletion lags by up to days
	}
	return &c.SelfExclusion, nil
}

// ---- in memory (tests, and the importer's dry run) ----

type MemoryStore struct {
	mu      sync.Mutex
	states  map[string]State
	markers map[string]bool
	carried map[string]carriedRow
	Audits  []string // "user|event|detail"
}

type carriedRow struct {
	ex  SelfExclusion
	ttl int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{states: map[string]State{}, markers: map[string]bool{}, carried: map[string]carriedRow{}}
}

func (m *MemoryStore) Get(_ context.Context, userID string) (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.states[userID]
	if !ok {
		return nil, nil
	}
	cp := deepCopy(st)
	return &cp, nil
}

func deepCopy(st State) State {
	if st.SelfExclusion != nil {
		c := *st.SelfExclusion
		st.SelfExclusion = &c
	}
	if st.GameLimits != nil {
		c := *st.GameLimits
		if c.Pending != nil {
			p := *c.Pending
			c.Pending = &p
		}
		st.GameLimits = &c
	}
	if st.GameDepositCounters != nil {
		c := *st.GameDepositCounters
		st.GameDepositCounters = &c
	}
	return st
}

func (m *MemoryStore) put(st *State, prev int64) error {
	cur, ok := m.states[st.UserID]
	if (!ok && prev != 0) || (ok && cur.Version != prev) {
		return ErrConflict
	}
	next := deepCopy(*st)
	next.SK, next.Version = StateSK, prev+1
	m.states[st.UserID] = next
	st.Version = prev + 1
	return nil
}

func (m *MemoryStore) Put(_ context.Context, st *State, prev int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.put(st, prev)
}

func (m *MemoryStore) CommitInflow(_ context.Context, st *State, prev int64, key string, _ time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markers[st.UserID+"|"+key] {
		return false, nil
	}
	if err := m.put(st, prev); err != nil {
		return false, err
	}
	m.markers[st.UserID+"|"+key] = true
	return true, nil
}

func (m *MemoryStore) Counted(_ context.Context, userID, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.markers[userID+"|"+key], nil
}

func (m *MemoryStore) Audit(_ context.Context, userID, event, detail string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Audits = append(m.Audits, userID+"|"+event+"|"+detail)
	return nil
}

func (m *MemoryStore) PutCarried(_ context.Context, cpf string, ex SelfExclusion, exp time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.carried[cpf]; ok && cur.ttl >= exp.Unix() {
		return nil
	}
	m.carried[cpf] = carriedRow{ex: ex, ttl: exp.Unix()}
	return nil
}

func (m *MemoryStore) GetCarried(_ context.Context, cpf string, now time.Time) (*SelfExclusion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.carried[cpf]
	if !ok || row.ttl <= now.Unix() {
		return nil, nil
	}
	ex := row.ex
	return &ex, nil
}
