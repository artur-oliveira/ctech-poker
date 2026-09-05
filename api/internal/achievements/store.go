package achievements

import (
	"context"
	"fmt"
	"maps"
	"slices"
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

// The aggregate progress row (issue #198). Every achievement counter for one
// player and one currency mode lives on a single item — `pk` = player,
// `sk` = "<mode>#_progress" — as one top-level attribute per key
// (`counterAttrPrefix`+key, a Number) plus that key's last tier-unlock
// timestamp (`unlockAttrPrefix`+key, a String). A whole hand's deltas for one
// player therefore commit as ONE UpdateItem instead of the dozens of
// per-key UpdateItems `Increment`/`IncrementStreak` used to issue (hands
// played, time bank, win, category, earnings, pocket pair, full table /
// heads-up, all-in, ... × every participant).
//
// One item per (player, mode) — not one per key — is what makes that
// possible: DynamoDB's `ADD` works only on top-level attributes, so a nested
// map of counters could not be incremented atomically. The per-key rows
// written before #198 are still read (see legacyCounters/ListAchievements)
// and are absorbed into the aggregate the first time a player records a hand.
const (
	aggregateSK        = "_progress"
	counterAttrPrefix  = "c#"
	unlockAttrPrefix   = "u#"
	legacyCounterField = "counter"
)

func aggregateSKFor(mode string) string { return mode + "#" + aggregateSK }

// ApplyHandProgress applies one player's entire hand in a single write:
// `adds` are counters to increment (ADD), `sets` are counters to overwrite
// (SET — the streak resets, whose whole point is to drop back to a fixed
// value). It returns each touched key's previous and current totals, so the
// caller can detect tier crossings from values that are correct even under
// concurrent hands: `current` comes from DynamoDB's ALL_NEW image and
// `previous` is derived from it by undoing this call's own delta, never by a
// separate read.
func (s *Store) ApplyHandProgress(ctx context.Context, playerID, mode string, adds, sets map[string]int) (map[string]int, map[string]int, error) {
	if playerID == "" || (len(adds) == 0 && len(sets) == 0) {
		return map[string]int{}, map[string]int{}, nil
	}
	for key, by := range adds {
		if by <= 0 {
			return nil, nil, fmt.Errorf("achievements: increment for %s must be positive", key)
		}
	}
	input := s.progressUpdate(playerID, mode, adds, sets)
	// The aggregate must already exist: if it does not, this player's counters
	// may still be in the pre-#198 per-key rows, and starting the aggregate at
	// zero would both lose their totals and re-unlock tiers they passed long
	// ago. seedAggregate absorbs those rows first.
	input.ConditionExpression = aws.String("attribute_exists(pk)")
	out, err := s.base.UpdateItemRaw(ctx, input)
	if err != nil {
		if !dynamo.IsConditionFailed(err) {
			return nil, nil, fmt.Errorf("achievements: apply hand progress: %w", err)
		}
		return s.seedAggregate(ctx, playerID, mode, adds, sets)
	}
	current := countersFrom(out.Attributes)
	return previousOf(current, adds, sets), current, nil
}

func (s *Store) progressUpdate(playerID, mode string, adds, sets map[string]int) *dynamodb.UpdateItemInput {
	names := map[string]string{}
	values := map[string]dynamotypes.AttributeValue{}
	var addClauses, setClauses []string
	for _, key := range slices.Sorted(maps.Keys(adds)) {
		alias, placeholder := fmt.Sprintf("#a%d", len(names)), fmt.Sprintf(":a%d", len(values))
		names[alias], values[placeholder] = counterAttrPrefix+key, &dynamotypes.AttributeValueMemberN{Value: strconv.Itoa(adds[key])}
		addClauses = append(addClauses, alias+" "+placeholder)
	}
	for _, key := range slices.Sorted(maps.Keys(sets)) {
		alias, placeholder := fmt.Sprintf("#s%d", len(names)), fmt.Sprintf(":s%d", len(values))
		names[alias], values[placeholder] = counterAttrPrefix+key, &dynamotypes.AttributeValueMemberN{Value: strconv.Itoa(sets[key])}
		setClauses = append(setClauses, alias+" = "+placeholder)
	}
	var expression string
	if len(addClauses) > 0 {
		expression = "ADD " + strings.Join(addClauses, ", ")
	}
	if len(setClauses) > 0 {
		expression = strings.TrimSpace(expression + " SET " + strings.Join(setClauses, ", "))
	}
	return &dynamodb.UpdateItemInput{
		TableName: aws.String(s.base.TableName),
		Key: map[string]dynamotypes.AttributeValue{
			"pk": &dynamotypes.AttributeValueMemberS{Value: playerID},
			"sk": &dynamotypes.AttributeValueMemberS{Value: aggregateSKFor(mode)},
		},
		UpdateExpression:          aws.String(expression),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		ReturnValues:              dynamotypes.ReturnValueAllNew,
	}
}

// seedAggregate creates a player's aggregate row from whatever per-key rows
// they already have and applies this hand's deltas on top, as one
// create-only PutItem. It runs at most once per player and mode — the first
// hand after this change ships — so there is no migration job: a veteran's
// totals (and their existing unlock timestamps) carry over, and their tier
// crossings are still computed against the totals they actually had.
//
// A concurrent hand can win the create; that loser simply retries the normal
// update path, which now finds the row it needs.
func (s *Store) seedAggregate(ctx context.Context, playerID, mode string, adds, sets map[string]int) (map[string]int, map[string]int, error) {
	legacy, unlocked, err := s.legacyCounters(ctx, playerID, mode)
	if err != nil {
		return nil, nil, err
	}
	current := maps.Clone(legacy)
	for key, value := range sets {
		current[key] = value
	}
	for key, by := range adds {
		current[key] += by
	}
	item := map[string]dynamotypes.AttributeValue{
		"pk": &dynamotypes.AttributeValueMemberS{Value: playerID},
		"sk": &dynamotypes.AttributeValueMemberS{Value: aggregateSKFor(mode)},
	}
	for key, value := range current {
		item[counterAttrPrefix+key] = &dynamotypes.AttributeValueMemberN{Value: strconv.Itoa(value)}
	}
	for key, at := range unlocked {
		item[unlockAttrPrefix+key] = &dynamotypes.AttributeValueMemberS{Value: at}
	}
	_, err = s.base.PutItemRaw(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.base.TableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	if err != nil {
		if !dynamo.IsConditionFailed(err) {
			return nil, nil, fmt.Errorf("achievements: seed progress aggregate: %w", err)
		}
		// Lost the create race: the row exists now, so the plain update works.
		out, updateErr := s.base.UpdateItemRaw(ctx, s.progressUpdate(playerID, mode, adds, sets))
		if updateErr != nil {
			return nil, nil, fmt.Errorf("achievements: apply hand progress: %w", updateErr)
		}
		raced := countersFrom(out.Attributes)
		return previousOf(raced, adds, sets), raced, nil
	}
	return previousOf(current, adds, sets), current, nil
}

// legacyCounters reads a player's pre-#198 per-key rows for one mode: their
// counters and their unlock timestamps. Per-table streak rows
// (streakKeyPrefix) are deliberately excluded — those are live display state
// that UpdateTableStreak keeps in its own items, not achievement progress —
// and they are also why this pages instead of reading one page: a player
// accumulates one such row per table they have ever played.
func (s *Store) legacyCounters(ctx context.Context, playerID, mode string) (map[string]int, map[string]string, error) {
	counters, unlocked := map[string]int{}, map[string]string{}
	prefix := mode + "#"
	var startKey map[string]dynamotypes.AttributeValue
	for page := 0; page < allProgressMaxPages; page++ {
		result, err := s.base.Query(ctx, dynamo.QueryOpts{
			PK: playerID, SKPrefix: prefix, Limit: allProgressPageSize, ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("achievements: read legacy progress: %w", err)
		}
		for _, item := range result.Items {
			row, decodeErr := dynamo.Decode[PlayerAchievementProgress](item)
			if decodeErr != nil {
				return nil, nil, fmt.Errorf("achievements: decode legacy progress: %w", decodeErr)
			}
			key := strings.TrimPrefix(row.Key, prefix)
			if key == aggregateSK || strings.HasPrefix(key, streakKeyPrefix) {
				continue
			}
			counters[key] = row.Count
			if row.UnlockedAt != "" {
				unlocked[key] = row.UnlockedAt
			}
		}
		if len(result.LastEvaluatedKey) == 0 {
			break
		}
		startKey = result.LastEvaluatedKey
	}
	return counters, unlocked, nil
}

// countersFrom reads the counter attributes out of an aggregate item image.
func countersFrom(item map[string]dynamotypes.AttributeValue) map[string]int {
	out := make(map[string]int, len(item))
	for name, value := range item {
		key, ok := strings.CutPrefix(name, counterAttrPrefix)
		if !ok {
			continue
		}
		number, isNumber := value.(*dynamotypes.AttributeValueMemberN)
		if !isNumber {
			continue
		}
		if parsed, err := strconv.Atoi(number.Value); err == nil {
			out[key] = parsed
		}
	}
	return out
}

// rowsFromAggregate expands one aggregate item into the same
// PlayerAchievementProgress rows the per-key model returned, applying the
// same filter ListAchievements always applied: a secret achievement below its
// first tier stays hidden. Keys are sorted so the response is stable across
// calls (a DynamoDB item is a map, with no attribute order).
func rowsFromAggregate(item map[string]dynamotypes.AttributeValue) []PlayerAchievementProgress {
	counters := countersFrom(item)
	out := make([]PlayerAchievementProgress, 0, len(counters))
	for _, key := range slices.Sorted(maps.Keys(counters)) {
		if achievement, ok := achievementForKey(key); ok && achievement.Secret &&
			counters[key] < minimumThreshold(achievement.Tiers) {
			continue
		}
		row := PlayerAchievementProgress{Key: key, Count: counters[key]}
		if at, ok := item[unlockAttrPrefix+key].(*dynamotypes.AttributeValueMemberS); ok {
			row.UnlockedAt = at.Value
		}
		out = append(out, row)
	}
	return out
}

// previousOf reconstructs each touched key's total from before this write.
// An overwritten (SET) counter reports its new value as the previous one too:
// a streak reset must never look like a threshold crossing, which is exactly
// what IncrementStreak's caller did before #198.
func previousOf(current map[string]int, adds, sets map[string]int) map[string]int {
	previous := make(map[string]int, len(adds)+len(sets))
	for key, by := range adds {
		previous[key] = current[key] - by
	}
	for key := range sets {
		previous[key] = current[key]
	}
	return previous
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
	// UnlockedAt is when this key last crossed a tier threshold (issue #72),
	// stamped by StampTierUnlock. Empty for every row written before that
	// existed and for every key still below its first tier — a legacy or
	// still-locked row, never an error, so the client just gets no "recently
	// unlocked" moment for it.
	UnlockedAt string `dynamodbav:"unlocked_at,omitempty" json:"unlocked_at,omitempty"`
}

// StampTierUnlocks records "now" as the moment playerID crossed a tier on
// each of keys. Called by Service.RecordHand once per player per hand with
// every tier that hand actually crossed (issue #198 — it used to be one write
// per tier), i.e. only when a threshold is genuinely crossed: a replayed hand
// hook is already stopped upstream by ClaimHandCounters, and even past that a
// replay moves no counter, so TierCrossed stays false and nothing is
// rewritten. A later, higher tier does overwrite the stamp — the field means
// "last tier unlocked at", which is the recency the achievements page sorts
// and celebrates by. The stamps ride on the same aggregate item as the
// counters, so a player costs one extra write here, never one per tier.
func (s *Store) StampTierUnlocks(ctx context.Context, playerID, mode string, keys []string) error {
	if playerID == "" || len(keys) == 0 {
		return nil
	}
	names := map[string]string{}
	var clauses []string
	for _, key := range slices.Sorted(slices.Values(keys)) {
		if key == "" {
			continue
		}
		alias := fmt.Sprintf("#u%d", len(names))
		names[alias] = unlockAttrPrefix + key
		clauses = append(clauses, alias+" = :now")
	}
	if len(clauses) == 0 {
		return nil
	}
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.base.TableName),
		Key: map[string]dynamotypes.AttributeValue{
			"pk": &dynamotypes.AttributeValueMemberS{Value: playerID},
			"sk": &dynamotypes.AttributeValueMemberS{Value: aggregateSKFor(mode)},
		},
		UpdateExpression:          aws.String("SET " + strings.Join(clauses, ", ")),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: map[string]dynamotypes.AttributeValue{":now": &dynamotypes.AttributeValueMemberS{Value: dynamo.NowStr()}},
	})
	if err != nil {
		return fmt.Errorf("achievements: stamp tier unlocks %s: %w", playerID, err)
	}
	return nil
}

// ListAchievements returns one player's progress rows for a mode. Since #198
// a player's counters live on a single aggregate item, so the common path is
// one GetItem whose rows are returned complete, with no cursor: the catalog
// is bounded at a few dozen keys, and a paginated read of one item would only
// invite a caller to think it had part of the answer. `limit`/`startKey`
// therefore apply only to the pre-#198 per-key rows, which are still served
// verbatim for a player who has not recorded a hand since this shipped.
func (s *Store) ListAchievements(ctx context.Context, playerID, mode string, limit int, startKey map[string]dynamotypes.AttributeValue) ([]PlayerAchievementProgress, map[string]dynamotypes.AttributeValue, error) {
	if playerID == "" || limit < 0 {
		return []PlayerAchievementProgress{}, nil, fmt.Errorf("achievements: invalid playerId or limit values")
	}
	aggregate, err := s.base.GetItem(ctx, playerID, aggregateSKFor(mode))
	if err != nil {
		return nil, nil, fmt.Errorf("achievements: read progress aggregate: %w", err)
	}
	if aggregate != nil {
		return rowsFromAggregate(aggregate), nil, nil
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
