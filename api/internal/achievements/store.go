package achievements

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	dynamo "gopkg.aoctech.app/api-commons/dynamo"
)

const tableProgress = "poker_achievement_progress"

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
	value := 1
	if reset {
		expression, value = "SET #counter = :value", resetTo
	}
	out, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.base.TableName),
		Key: map[string]dynamotypes.AttributeValue{
			"pk": &dynamotypes.AttributeValueMemberS{Value: playerID},
			"sk": &dynamotypes.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression:         aws.String(expression),
		ExpressionAttributeNames: map[string]string{"#counter": "counter"},
		ExpressionAttributeValues: map[string]dynamotypes.AttributeValue{
			":one":   &dynamotypes.AttributeValueMemberN{Value: "1"},
			":value": &dynamotypes.AttributeValueMemberN{Value: strconv.Itoa(value)},
		},
		ReturnValues: dynamotypes.ReturnValueAllNew,
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
