package botfunding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/metrics"
)

const tableProgress = "poker_achievement_progress"
const windowKind = "bot_window"
const guardKind = "bot_hand_guard"

type Window struct {
	PlayerID  string `dynamodbav:"pk"`
	Kind      string `dynamodbav:"sk"`
	StartedAt int64  `dynamodbav:"started_at"`
	ExpiresAt int64  `dynamodbav:"expires_at"`
	NetProfit int64  `dynamodbav:"net_profit"`
	TTL       int64  `dynamodbav:"ttl"`
}

type Store struct {
	db        *dynamodb.Client
	tableName string
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{db: db, tableName: env + "_" + tableProgress}
}

func handKey(tableID, handID string) string {
	sum := sha256.Sum256([]byte(tableID + "\x00" + handID))
	return "bot_hand_guard#" + hex.EncodeToString(sum[:])
}

func windowKey(playerID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: playerID},
		"sk": &types.AttributeValueMemberS{Value: windowKind},
	}
}

func guardKey(tableID, handID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: handKey(tableID, handID)},
		"sk": &types.AttributeValueMemberS{Value: guardKind},
	}
}

func (s *Store) Load(ctx context.Context, playerID string) (*Window, error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName), ConsistentRead: aws.Bool(true), Key: windowKey(playerID),
	})
	if err != nil {
		return nil, fmt.Errorf("bot funding: load window: %w", err)
	}
	if len(out.Item) == 0 {
		return nil, nil
	}
	var window Window
	if err := attributevalue.UnmarshalMap(out.Item, &window); err != nil {
		return nil, fmt.Errorf("bot funding: decode window: %w", err)
	}
	return &window, nil
}

func (s *Store) AlreadyRecorded(ctx context.Context, tableID, handID string) (bool, error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName), ConsistentRead: aws.Bool(true), Key: guardKey(tableID, handID),
	})
	if err != nil {
		return false, fmt.Errorf("bot funding: load hand guard: %w", err)
	}
	return len(out.Item) > 0, nil
}

func (s *Store) Create(ctx context.Context, playerID, tableID, handID string, delta int64, now time.Time) (*Window, error) {
	window := &Window{PlayerID: playerID, Kind: windowKind, StartedAt: now.UnixMilli(),
		ExpiresAt: now.Add(24 * time.Hour).UnixMilli(), NetProfit: delta, TTL: now.Add(25 * time.Hour).Unix()}
	item, err := attributevalue.MarshalMap(window)
	if err != nil {
		return nil, fmt.Errorf("bot funding: encode window: %w", err)
	}
	guard, err := guardItem(playerID, tableID, handID, now)
	if err != nil {
		return nil, err
	}
	out, err := s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal,
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(s.tableName), Item: item,
				ConditionExpression: aws.String("attribute_not_exists(pk) OR expires_at <= :now"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":now": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.UnixMilli(), 10)},
				}}},
			{Put: &types.Put{TableName: aws.String(s.tableName), Item: guard,
				ConditionExpression: aws.String("attribute_not_exists(pk)")}},
		},
	})
	if err != nil {
		return nil, err
	}
	recordCapacity(out.ConsumedCapacity)
	return window, nil
}

func (s *Store) Update(ctx context.Context, current *Window, playerID, tableID, handID string, delta int64, now time.Time) (*Window, error) {
	guard, err := guardItem(playerID, tableID, handID, now)
	if err != nil {
		return nil, err
	}
	out, err := s.db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal,
		TransactItems: []types.TransactWriteItem{
			{Update: &types.Update{TableName: aws.String(s.tableName), Key: windowKey(playerID),
				UpdateExpression:    aws.String("SET net_profit = net_profit + :delta"),
				ConditionExpression: aws.String("started_at = :started AND expires_at > :now AND net_profit = :expected"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":delta":    &types.AttributeValueMemberN{Value: strconv.FormatInt(delta, 10)},
					":started":  &types.AttributeValueMemberN{Value: strconv.FormatInt(current.StartedAt, 10)},
					":expected": &types.AttributeValueMemberN{Value: strconv.FormatInt(current.NetProfit, 10)},
					":now":      &types.AttributeValueMemberN{Value: strconv.FormatInt(now.UnixMilli(), 10)},
				}}},
			{Put: &types.Put{TableName: aws.String(s.tableName), Item: guard,
				ConditionExpression: aws.String("attribute_not_exists(pk)")}},
		},
	})
	if err != nil {
		return nil, err
	}
	recordCapacity(out.ConsumedCapacity)
	updated := *current
	updated.NetProfit += delta
	return &updated, nil
}

func guardItem(playerID, tableID, handID string, now time.Time) (map[string]types.AttributeValue, error) {
	item, err := attributevalue.MarshalMap(struct {
		PK       string `dynamodbav:"pk"`
		SK       string `dynamodbav:"sk"`
		PlayerID string `dynamodbav:"player_id"`
		TTL      int64  `dynamodbav:"ttl"`
	}{PK: handKey(tableID, handID), SK: guardKind, PlayerID: playerID, TTL: now.Add(7 * 24 * time.Hour).Unix()})
	if err != nil {
		return nil, fmt.Errorf("bot funding: encode hand guard: %w", err)
	}
	return item, nil
}

func recordCapacity(consumed []types.ConsumedCapacity) {
	for _, c := range consumed {
		if c.WriteCapacityUnits != nil {
			metrics.Record("BotFundingWriteCapacityUnits", metrics.Count, nil, *c.WriteCapacityUnits)
		} else if c.CapacityUnits != nil {
			metrics.Record("BotFundingWriteCapacityUnits", metrics.Count, nil, *c.CapacityUnits)
		}
	}
}

func IsConflict(err error) bool {
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) {
		return false
	}
	for _, reason := range canceled.CancellationReasons {
		if aws.ToString(reason.Code) == "ConditionalCheckFailed" {
			return true
		}
	}
	return false
}
