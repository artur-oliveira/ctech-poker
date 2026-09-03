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

// handGuardPK/handGuardSK namespace the per-hand idempotency guard (issue
// #66) inside the same table, the same "no new table" reuse streakKeyPrefix
// above already established. pk carries table_id+hand_id (guaranteed unique
// per real player_id pk since a player_id never starts with "hand_guard#"),
// so it can never collide with a real progress row.
func handGuardPK(tableID, handID string) string { return "hand_guard#" + tableID + "#" + handID }

const handGuardSK = "guard"

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableProgress)}
}

// ClaimHandCounters reports whether this call is the first, fleet-wide, to
// reach this hand's non-idempotent achievement/leaderboard counters (issue
// #66). internal/handhook's SET NX claim already dedupes which instance may
// run onHandComplete at all, but it fails OPEN on a Valkey error (deliberate
// there — "a double credit is at least visible and bounded", see
// handhook.go) — so on a Valkey blip during a hand completion, two instances
// can both pass that claim and both reach Service.RecordHand's bare ADD
// increments, which have no guard of their own. This is that guard: an
// atomic conditional PutItem, following this codebase's established
// "guard#table_id#hand_id" idempotency idiom (pokerstats.Store.RecordHand,
// matchup.Store.RecordHand), reusing this table rather than a new one (same
// reasoning as streakKeyPrefix above). Unlike handhook's claim, an error here
// is reported to the caller rather than defaulting to "claimed": this guard
// IS the correctness backstop for these counters, so an ambiguous outcome
// must never risk a second increment landing.
func (s *Store) ClaimHandCounters(ctx context.Context, tableID, handID string) (bool, error) {
	if tableID == "" || handID == "" {
		return true, nil
	}
	item, err := dynamo.Encode(struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
	}{PK: handGuardPK(tableID, handID), SK: handGuardSK})
	if err != nil {
		return false, fmt.Errorf("achievements: encode hand guard: %w", err)
	}
	_, err = s.base.PutItemRaw(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.base.TableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	if err != nil {
		if dynamo.IsConditionFailed(err) {
			return false, nil
		}
		return false, fmt.Errorf("achievements: claim hand counters %s/%s: %w", tableID, handID, err)
	}
	return true, nil
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

// allProgressPageSize / allProgressMaxPages bound the batched read behind the
// full-state summary endpoint. The achievement catalog is bounded (~dozens of
// keys), so one page almost always suffices; the internal loop and the hard
// page cap exist only so a player who has touched more distinct keys than a
// single DynamoDB page holds still gets their complete state, never a
// truncated one, and an unbounded fetch loop can never happen.
const (
	allProgressPageSize = 200
	allProgressMaxPages = 20
)

// AllAchievements returns every progress row for one player in a single logical
// read, following DynamoDB's pagination cursor internally up to
// allProgressMaxPages. It reuses ListAchievements' row filtering: streak
// counters are dropped and still-locked secret achievements stay hidden, while
// a secret achievement already past its first tier is included.
func (s *Store) AllAchievements(ctx context.Context, playerID, mode string) ([]PlayerAchievementProgress, error) {
	if playerID == "" {
		return nil, fmt.Errorf("achievements: invalid playerId")
	}
	var (
		out      []PlayerAchievementProgress
		startKey map[string]dynamotypes.AttributeValue
	)
	for page := 0; page < allProgressMaxPages; page++ {
		rows, next, err := s.ListAchievements(ctx, playerID, mode, allProgressPageSize, startKey)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if len(next) == 0 {
			break
		}
		startKey = next
	}
	return out, nil
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
