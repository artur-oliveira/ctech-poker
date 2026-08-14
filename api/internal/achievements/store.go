package achievements

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const tableProgress = "poker_achievement_progress"

// streakKeyPrefix namespaces the hot/cold table-streak counter inside the
// same DynamoDB table as starred achievement progress (no new table, no CDK
// change), keyed apart from any real achievement so ListAchievements below
// can exclude it from a player's achievement list.
const streakKeyPrefix = "streak#"

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableProgress)}
}

func (s *Store) Increment(ctx context.Context, playerID, mode, key string, by int) (int, int, error) {
	if by <= 0 {
		return 0, 0, fmt.Errorf("achievements: increment must be positive")
	}
	sk := mode + "#" + key
	out, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.base.TableName),
		Key: map[string]dynamotypes.AttributeValue{
			"pk": &dynamotypes.AttributeValueMemberS{Value: playerID},
			"sk": &dynamotypes.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression:          aws.String("ADD #counter :by"),
		ExpressionAttributeNames:  map[string]string{"#counter": "counter"},
		ExpressionAttributeValues: map[string]dynamotypes.AttributeValue{":by": &dynamotypes.AttributeValueMemberN{Value: strconv.Itoa(by)}},
		ReturnValues:              dynamotypes.ReturnValueAllNew,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("achievements: increment: %w", err)
	}
	currentValue, ok := out.Attributes["counter"].(*dynamotypes.AttributeValueMemberN)
	if !ok {
		return 0, 0, fmt.Errorf("achievements: increment returned no counter")
	}
	current, err := strconv.Atoi(currentValue.Value)
	if err != nil {
		return 0, 0, fmt.Errorf("achievements: parse counter: %w", err)
	}
	return current - by, current, nil
}

func (s *Store) IncrementStreak(ctx context.Context, playerID, mode, key string, reset bool, resetTo int) (int, error) {
	sk := mode + "#" + key
	expression := "ADD #counter :one"
	values := map[string]dynamotypes.AttributeValue{":one": &dynamotypes.AttributeValueMemberN{Value: "1"}}
	if reset {
		expression = "SET #counter = :value"
		values = map[string]dynamotypes.AttributeValue{":value": &dynamotypes.AttributeValueMemberN{Value: strconv.Itoa(resetTo)}}
	}
	out, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.base.TableName),
		Key: map[string]dynamotypes.AttributeValue{
			"pk": &dynamotypes.AttributeValueMemberS{Value: playerID},
			"sk": &dynamotypes.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression:          aws.String(expression),
		ExpressionAttributeNames:  map[string]string{"#counter": "counter"},
		ExpressionAttributeValues: values,
		ReturnValues:              dynamotypes.ReturnValueAllNew,
	})
	if err != nil {
		return 0, fmt.Errorf("achievements: streak: %w", err)
	}
	v, ok := out.Attributes["counter"].(*dynamotypes.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("achievements: streak returned no counter")
	}
	current, err := strconv.Atoi(v.Value)
	return current, err
}

// UpdateTableStreak advances playerID's running win/loss streak for one
// table: continuing the same sign is a plain ADD, crossing zero resets to
// ±1. Two conditional writes (never read-then-write) keep this correct under
// concurrent calls, the same correctness convention every other table
// mutation in this codebase follows.
func (s *Store) UpdateTableStreak(ctx context.Context, playerID, mode, tableID string, won bool) (int, error) {
	sk := mode + "#" + streakKeyPrefix + tableID
	key := map[string]dynamotypes.AttributeValue{
		"pk": &dynamotypes.AttributeValueMemberS{Value: playerID},
		"sk": &dynamotypes.AttributeValueMemberS{Value: sk},
	}
	delta, condition := "1", "attribute_not_exists(#counter) OR #counter >= :zero"
	if !won {
		delta, condition = "-1", "attribute_not_exists(#counter) OR #counter <= :zero"
	}
	out, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		TableName:                aws.String(s.base.TableName),
		Key:                      key,
		UpdateExpression:         aws.String("ADD #counter :delta"),
		ConditionExpression:      aws.String(condition),
		ExpressionAttributeNames: map[string]string{"#counter": "counter"},
		ExpressionAttributeValues: map[string]dynamotypes.AttributeValue{
			":delta": &dynamotypes.AttributeValueMemberN{Value: delta},
			":zero":  &dynamotypes.AttributeValueMemberN{Value: "0"},
		},
		ReturnValues: dynamotypes.ReturnValueAllNew,
	})
	if err != nil {
		if !dynamo.IsConditionFailed(err) {
			return 0, fmt.Errorf("achievements: table streak: %w", err)
		}
		resetTo := "1"
		if !won {
			resetTo = "-1"
		}
		out, err = s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
			TableName:                 aws.String(s.base.TableName),
			Key:                       key,
			UpdateExpression:          aws.String("SET #counter = :value"),
			ExpressionAttributeNames:  map[string]string{"#counter": "counter"},
			ExpressionAttributeValues: map[string]dynamotypes.AttributeValue{":value": &dynamotypes.AttributeValueMemberN{Value: resetTo}},
			ReturnValues:              dynamotypes.ReturnValueAllNew,
		})
		if err != nil {
			return 0, fmt.Errorf("achievements: table streak reset: %w", err)
		}
	}
	v, ok := out.Attributes["counter"].(*dynamotypes.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("achievements: table streak returned no counter")
	}
	current, err := strconv.Atoi(v.Value)
	if err != nil {
		return 0, fmt.Errorf("achievements: parse table streak: %w", err)
	}
	return current, nil
}

// PlayerAchievementProgress is one player's progress row for a single
// achievement key (pk: playerID, sk: achievement key, counter: current
// count) — distinct from Achievement, which describes the catalog
// definition (metric + tier thresholds), not a player's progress.
type PlayerAchievementProgress struct {
	Key   string `dynamodbav:"sk" json:"key"`
	Count int    `dynamodbav:"counter" json:"count"`
}

func (s *Store) ListAchievements(ctx context.Context, playerID, mode string, limit int, startKey map[string]dynamotypes.AttributeValue) ([]PlayerAchievementProgress, map[string]dynamotypes.AttributeValue, error) {
	if playerID == "" || limit < 0 {
		return []PlayerAchievementProgress{}, nil, fmt.Errorf("achievements: invalid playerId or limit values")
	}
	prefix := mode + "#"
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, SKPrefix: prefix, Limit: limit, ExclusiveStartKey: startKey})
	if err != nil {
		return nil, nil, err
	}
	out := make([]PlayerAchievementProgress, 0, len(result.Items))
	for _, item := range result.Items {
		e, err := dynamo.Decode[PlayerAchievementProgress](item)
		if err != nil {
			return nil, nil, fmt.Errorf("achievements: decode: %w", err)
		}
		e.Key = strings.TrimPrefix(e.Key, prefix)
		if strings.HasPrefix(e.Key, streakKeyPrefix) {
			continue
		}
		if achievement, ok := achievementForKey(e.Key); ok && achievement.Secret &&
			e.Count < minimumThreshold(achievement.Tiers) {
			continue
		}
		out = append(out, *e)
	}
	return out, result.LastEvaluatedKey, nil
}

func achievementForKey(key string) (Achievement, bool) {
	for _, achievement := range Catalog {
		if achievement.Key == key {
			return achievement, true
		}
	}
	return Achievement{}, false
}

func minimumThreshold(tiers []Tier) int {
	if len(tiers) == 0 {
		return 0
	}
	minimum := tiers[0].Threshold
	for _, tier := range tiers[1:] {
		if tier.Threshold < minimum {
			minimum = tier.Threshold
		}
	}
	return minimum
}
