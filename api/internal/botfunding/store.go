package botfunding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/metrics"
)

const tableProgress = "poker_achievement_progress"

type Window struct {
	PlayerID  string          `dynamodbav:"pk"`
	Kind      string          `dynamodbav:"sk"`
	StartedAt int64           `dynamodbav:"started_at"`
	ExpiresAt int64           `dynamodbav:"expires_at"`
	NetProfit int64           `dynamodbav:"net_profit"`
	Hands     map[string]int8 `dynamodbav:"hands"`
	TTL       int64           `dynamodbav:"ttl"`
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
	return "h" + hex.EncodeToString(sum[:12])
}

func (s *Store) Load(ctx context.Context, playerID string) (*Window, error) {
	out, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName), ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: playerID},
			"sk": &types.AttributeValueMemberS{Value: "bot_window"},
		},
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

func (s *Store) Create(ctx context.Context, playerID, tableID, handID string, delta int64, now time.Time) (*Window, error) {
	started, expires := now.UnixMilli(), now.Add(24*time.Hour).UnixMilli()
	window := &Window{PlayerID: playerID, Kind: "bot_window", StartedAt: started, ExpiresAt: expires,
		NetProfit: delta, Hands: map[string]int8{handKey(tableID, handID): 1}, TTL: now.Add(25 * time.Hour).Unix()}
	item, err := attributevalue.MarshalMap(window)
	if err != nil {
		return nil, fmt.Errorf("bot funding: encode window: %w", err)
	}
	out, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName), Item: item,
		ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal,
		ConditionExpression:    aws.String("attribute_not_exists(pk) OR expires_at <= :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":now": &types.AttributeValueMemberN{Value: strconv.FormatInt(started, 10)},
		},
	})
	if err != nil {
		return nil, err
	}
	if out.ConsumedCapacity != nil && out.ConsumedCapacity.CapacityUnits != nil {
		metrics.Record("BotFundingWriteCapacityUnits", metrics.Count, nil, *out.ConsumedCapacity.CapacityUnits)
	}
	return window, nil
}

func (s *Store) Update(ctx context.Context, current *Window, playerID, tableID, handID string, delta int64, now time.Time) (*Window, error) {
	key := handKey(tableID, handID)
	out, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName), ReturnValues: types.ReturnValueAllNew,
		ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal,
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: playerID},
			"sk": &types.AttributeValueMemberS{Value: "bot_window"},
		},
		UpdateExpression:         aws.String("SET net_profit = net_profit + :delta, #hands.#hand = :seen"),
		ConditionExpression:      aws.String("started_at = :started AND expires_at > :now AND attribute_not_exists(#hands.#hand)"),
		ExpressionAttributeNames: map[string]string{"#hands": "hands", "#hand": key},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":delta":   &types.AttributeValueMemberN{Value: strconv.FormatInt(delta, 10)},
			":seen":    &types.AttributeValueMemberN{Value: "1"},
			":started": &types.AttributeValueMemberN{Value: strconv.FormatInt(current.StartedAt, 10)},
			":now":     &types.AttributeValueMemberN{Value: strconv.FormatInt(now.UnixMilli(), 10)},
		},
	})
	if err != nil {
		return nil, err
	}
	if out.ConsumedCapacity != nil && out.ConsumedCapacity.CapacityUnits != nil {
		metrics.Record("BotFundingWriteCapacityUnits", metrics.Count, nil, *out.ConsumedCapacity.CapacityUnits)
	}
	var window Window
	if err := attributevalue.UnmarshalMap(out.Attributes, &window); err != nil {
		return nil, fmt.Errorf("bot funding: decode updated window: %w", err)
	}
	return &window, nil
}

func IsConflict(err error) bool { return dynamo.IsConditionFailed(err) }
