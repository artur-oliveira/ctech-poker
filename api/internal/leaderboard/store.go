package leaderboard

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableStats     = "poker_leaderboard_stats"
	statsSK        = "stats"
	gsiHandsWon    = "gsi_hands_won"
	gsiHandsPlayed = "gsi_hands_played"
	gsiWinRate     = "gsi_win_rate"
)

// MinHandsForWinRateRank is the minimum hands_played a player must have in a
// currency mode before they are eligible for that mode's win_rate leaderboard
// (issue #63). The gsi_win_rate_pk key is only written once a row crosses this
// floor and REMOVEd below it, so the sparse GSI excludes sub-floor rows from
// the ranking query outright; the service layer also filters defensively.
// Other metrics (hands_won, hands_played) are unaffected.
const MinHandsForWinRateRank = 100

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableStats)}
}

// IncrementStats bumps this hand's counters and, since a write here happens
// every hand anyway, piggybacks a refresh of the denormalized player_name
// (DynamoDB has no join, so the leaderboard row must carry its own copy) —
// cheaper than eagerly cascading a rename everywhere the player_id appears.
// Skipped entirely when name is unknown, so a caller that can't resolve it
// never blanks out a name written by a previous hand.
func (s *Store) IncrementStats(ctx context.Context, playerID, name, mode string, playedDelta, wonDelta int) error {
	sk := statsSK + "#" + mode
	key := map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: playerID}, "sk": &types.AttributeValueMemberS{Value: sk}}
	// gsi_win_rate_pk is deliberately NOT set here: it is a sparse key managed
	// by syncWinRateRankKey once the min-hands floor (issue #63) is known from
	// the post-update counters.
	updateExpr := "ADD #played :played, #won :won SET #updated = :now, #wonpk = :all, #playedpk = :all"
	names := map[string]string{"#played": "hands_played", "#won": "hands_won", "#updated": "updated_at", "#wonpk": "gsi_hands_won_pk", "#playedpk": "gsi_hands_played_pk"}
	values := map[string]types.AttributeValue{
		":played": &types.AttributeValueMemberN{Value: strconv.Itoa(playedDelta)}, ":won": &types.AttributeValueMemberN{Value: strconv.Itoa(wonDelta)},
		":now": &types.AttributeValueMemberS{Value: dynamo.NowStr()}, ":all": &types.AttributeValueMemberS{Value: mode},
	}
	if name != "" {
		updateExpr += ", #name = :name"
		names["#name"] = "player_name"
		values[":name"] = &types.AttributeValueMemberS{Value: name}
	}
	out, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:                       key,
		UpdateExpression:          new(updateExpr),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		return fmt.Errorf("leaderboard: increment stats: %w", err)
	}
	played, won := number(out.Attributes["hands_played"]), number(out.Attributes["hands_won"])
	if err := s.materializeWinRate(ctx, playerID, mode, played, won); err != nil {
		return err
	}
	return s.syncWinRateRankKey(ctx, playerID, mode, played)
}

// syncWinRateRankKey adds gsi_win_rate_pk once a row reaches
// MinHandsForWinRateRank hands and REMOVEs it below the floor (issue #63).
// Because it runs on every counter update it also lazily backfills existing
// sub-floor rows that still carry a stale key from before this change — no
// migration job needed, the row is cleaned on its owner's next hand.
func (s *Store) syncWinRateRankKey(ctx context.Context, playerID, mode string, played int64) error {
	key := map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: playerID}, "sk": &types.AttributeValueMemberS{Value: statsSK + "#" + mode}}
	input := &dynamodb.UpdateItemInput{Key: key, ExpressionAttributeNames: map[string]string{"#ratepk": "gsi_win_rate_pk"}}
	if played >= MinHandsForWinRateRank {
		input.UpdateExpression = new("SET #ratepk = :mode")
		input.ExpressionAttributeValues = map[string]types.AttributeValue{":mode": &types.AttributeValueMemberS{Value: mode}}
	} else {
		input.UpdateExpression = new("REMOVE #ratepk")
	}
	if _, err := s.base.UpdateItemRaw(ctx, input); err != nil {
		return fmt.Errorf("leaderboard: sync win rate rank key: %w", err)
	}
	return nil
}

func number(value types.AttributeValue) int64 {
	if n, ok := value.(*types.AttributeValueMemberN); ok {
		parsed, err := strconv.ParseInt(n.Value, 10, 64)
		if err != nil {
			slog.Warn("leaderboard numeric attribute parse failed", "err", err)
		}
		return parsed
	}
	return 0
}

// materializeWinRate conditionally writes the ratio for the exact counter
// version observed. If another hand updates the counters first, it reloads
// and recomputes so an older writer can never overwrite a newer rate.
func (s *Store) materializeWinRate(ctx context.Context, playerID, mode string, played, won int64) error {
	sk := statsSK + "#" + mode
	for attempt := 0; attempt < 5; attempt++ {
		rate := 0.0
		if played > 0 {
			rate = float64(won) / float64(played)
		}
		_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
			Key:                      map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: playerID}, "sk": &types.AttributeValueMemberS{Value: sk}},
			UpdateExpression:         new("SET #rate = :rate"),
			ConditionExpression:      new("#played = :played AND #won = :won"),
			ExpressionAttributeNames: map[string]string{"#rate": "win_rate_score", "#played": "hands_played", "#won": "hands_won"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":rate":   &types.AttributeValueMemberN{Value: strconv.FormatFloat(rate, 'f', 9, 64)},
				":played": &types.AttributeValueMemberN{Value: strconv.FormatInt(played, 10)}, ":won": &types.AttributeValueMemberN{Value: strconv.FormatInt(won, 10)},
			},
		})
		if err == nil {
			return nil
		}
		if !dynamo.IsConditionFailed(err) {
			return fmt.Errorf("leaderboard: materialize win rate: %w", err)
		}
		item, getErr := s.base.GetItem(ctx, playerID, sk)
		if getErr != nil {
			return fmt.Errorf("leaderboard: reload win rate counters: %w", getErr)
		}
		played, won = number(item["hands_played"]), number(item["hands_won"])
	}
	return fmt.Errorf("leaderboard: win rate update remained contended")
}

func (s *Store) IncrementAchievementPoints(ctx context.Context, playerID, mode string, points int) error {
	sk := statsSK + "#" + mode
	for i := 0; i < points; i++ {
		if _, err := s.base.AtomicIncrement(ctx, playerID, &sk, "achievement_points"); err != nil {
			return fmt.Errorf("leaderboard: increment achievement points: %w", err)
		}
	}
	if err := s.base.UpsertAttrs(ctx, playerID, &sk, map[string]any{"gsi_hands_won_pk": mode, "gsi_hands_played_pk": mode}); err != nil {
		return fmt.Errorf("leaderboard: index achievement row: %w", err)
	}
	// win_rate eligibility follows hands_played only (issue #63) — achievement
	// points must not put a sub-floor row back on the win_rate board.
	item, err := s.base.GetItem(ctx, playerID, sk)
	if err != nil {
		return fmt.Errorf("leaderboard: reload hands for win rate key: %w", err)
	}
	return s.syncWinRateRankKey(ctx, playerID, mode, number(item["hands_played"]))
}

func (s *Store) Top(ctx context.Context, mode, metric string, limit int, startKey map[string]types.AttributeValue) ([]Entry, map[string]types.AttributeValue, error) {
	index, key := gsiHandsWon, "gsi_hands_won_pk"
	if metric == "hands_played" {
		index, key = gsiHandsPlayed, "gsi_hands_played_pk"
	} else if metric == "win_rate" {
		index, key = gsiWinRate, "gsi_win_rate_pk"
	}
	result, err := s.base.Query(ctx, dynamo.QueryOpts{
		PK: mode, PKField: key, IndexName: index,
		ScanIndexForward: false, Limit: limit, ExclusiveStartKey: startKey,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("leaderboard: query top: %w", err)
	}
	out := make([]Entry, 0, len(result.Items))
	for _, item := range result.Items {
		e, err := dynamo.Decode[Entry](item)
		if err != nil {
			return nil, nil, fmt.Errorf("leaderboard: decode: %w", err)
		}
		out = append(out, *e)
	}
	return out, result.LastEvaluatedKey, nil
}
