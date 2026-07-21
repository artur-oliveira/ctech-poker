package leaderboard

import (
	"context"
	"fmt"
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

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableStats)}
}

func (s *Store) IncrementStats(ctx context.Context, playerID string, playedDelta, wonDelta int) error {
	sk := statsSK
	key := map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: playerID}, "sk": &types.AttributeValueMemberS{Value: sk}}
	out, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:                      key,
		UpdateExpression:         awsString("ADD #played :played, #won :won SET #updated = :now, #wonpk = :all, #playedpk = :all, #ratepk = :all"),
		ExpressionAttributeNames: map[string]string{"#played": "hands_played", "#won": "hands_won", "#updated": "updated_at", "#wonpk": "gsi_hands_won_pk", "#playedpk": "gsi_hands_played_pk", "#ratepk": "gsi_win_rate_pk"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":played": &types.AttributeValueMemberN{Value: strconv.Itoa(playedDelta)}, ":won": &types.AttributeValueMemberN{Value: strconv.Itoa(wonDelta)},
			":now": &types.AttributeValueMemberS{Value: dynamo.NowStr()}, ":all": &types.AttributeValueMemberS{Value: "all"},
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		return fmt.Errorf("leaderboard: increment stats: %w", err)
	}
	played, won := number(out.Attributes["hands_played"]), number(out.Attributes["hands_won"])
	return s.materializeWinRate(ctx, playerID, played, won)
}

func awsString(v string) *string { return &v }

func number(value types.AttributeValue) int64 {
	if n, ok := value.(*types.AttributeValueMemberN); ok {
		parsed, _ := strconv.ParseInt(n.Value, 10, 64)
		return parsed
	}
	return 0
}

// materializeWinRate conditionally writes the ratio for the exact counter
// version observed. If another hand updates the counters first, it reloads
// and recomputes so an older writer can never overwrite a newer rate.
func (s *Store) materializeWinRate(ctx context.Context, playerID string, played, won int64) error {
	sk := statsSK
	for attempt := 0; attempt < 5; attempt++ {
		rate := 0.0
		if played > 0 {
			rate = float64(won) / float64(played)
		}
		_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
			Key:                      map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: playerID}, "sk": &types.AttributeValueMemberS{Value: sk}},
			UpdateExpression:         awsString("SET #rate = :rate"),
			ConditionExpression:      awsString("#played = :played AND #won = :won"),
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

func (s *Store) IncrementAchievementPoints(ctx context.Context, playerID string, points int) error {
	sk := statsSK
	for i := 0; i < points; i++ {
		if _, err := s.base.AtomicIncrement(ctx, playerID, &sk, "achievement_points"); err != nil {
			return fmt.Errorf("leaderboard: increment achievement points: %w", err)
		}
	}
	if err := s.base.UpsertAttrs(ctx, playerID, &sk, map[string]any{"gsi_hands_won_pk": "all", "gsi_hands_played_pk": "all"}); err != nil {
		return fmt.Errorf("leaderboard: index achievement row: %w", err)
	}
	return nil
}

func (s *Store) Top(ctx context.Context, metric string, limit int) ([]Entry, error) {
	index, key := gsiHandsWon, "gsi_hands_won_pk"
	if metric == "hands_played" {
		index, key = gsiHandsPlayed, "gsi_hands_played_pk"
	} else if metric == "win_rate" {
		index, key = gsiWinRate, "gsi_win_rate_pk"
	}
	result, err := s.base.Query(ctx, dynamo.QueryOpts{
		PK: "all", PKField: key, IndexName: index,
		ScanIndexForward: false, Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("leaderboard: query top: %w", err)
	}
	out := make([]Entry, 0, len(result.Items))
	for _, item := range result.Items {
		e, err := dynamo.Decode[Entry](item)
		if err != nil {
			return nil, fmt.Errorf("leaderboard: decode: %w", err)
		}
		out = append(out, *e)
	}
	return out, nil
}
