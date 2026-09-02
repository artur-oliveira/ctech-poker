package leaderboard

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
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

	// maxRankCountPages bounds the pagination loop in countGSI: a COUNT
	// query still pages every ~1MB of matched items, so a partition with an
	// unbounded number of players could in principle loop forever. This is
	// the single-partition-GSI hotspot flagged in issue #62 — acceptable for
	// today's scale, but the right long-term fix is the Valkey ZSET mirror
	// described there, not a bigger cap here.
	maxRankCountPages = 200
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

// gsiFor maps a rankable metric to its GSI name, partition-key attribute, and
// sort-key attribute (the field the GSI is actually ordered by).
func gsiFor(metric string) (index, pkField, sortField string) {
	switch metric {
	case "hands_played":
		return gsiHandsPlayed, "gsi_hands_played_pk", "hands_played"
	case "win_rate":
		return gsiWinRate, "gsi_win_rate_pk", "win_rate_score"
	default:
		return gsiHandsWon, "gsi_hands_won_pk", "hands_won"
	}
}

func (s *Store) Top(ctx context.Context, mode, metric string, limit int, startKey map[string]types.AttributeValue) ([]Entry, map[string]types.AttributeValue, error) {
	index, key, _ := gsiFor(metric)
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

// PlayerEntry loads a single player's stats row for mode, without going
// through the GSI. Returns (nil, nil) when the player has no row yet (they
// have never played a hand in this mode) — the caller's "unranked" case.
func (s *Store) PlayerEntry(ctx context.Context, playerID, mode string) (*Entry, error) {
	sk := statsSK + "#" + mode
	item, err := s.base.GetItem(ctx, playerID, sk)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: get player entry: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	e, err := dynamo.Decode[Entry](item)
	if err != nil {
		return nil, fmt.Errorf("leaderboard: decode player entry: %w", err)
	}
	return e, nil
}

// RankOf returns entry's 1-based rank and the total number of ranked rows for
// mode/metric, computed with COUNT queries against the metric's GSI instead
// of fetching and sorting the whole board. The rank is exact, including ties:
// it counts rows with a strictly better score, then rows tied on score but
// ordered before entry by player_id (the same tiebreak Service.Top's in-memory
// sort uses), then adds 1.
//
// Three GSI queries, each Select:COUNT and each paginating only over its own
// bounded slice (better-than, tied-before, and the mode's full partition for
// the total) — no items are fetched or sorted in this process. The full-count
// query for the mode's total is the one query genuinely bounded only by the
// mode's total player count; see the maxRankCountPages comment.
func (s *Store) RankOf(ctx context.Context, mode, metric string, entry Entry) (rank int64, total int64, err error) {
	index, pkField, sortField := gsiFor(metric)
	score := scoreFor(metric, entry)
	scoreAV := &types.AttributeValueMemberN{Value: formatScore(score)}

	better, err := s.countGSI(ctx, index, pkField, mode, countOpts{
		sortCond:   "#sort > :val",
		sortNames:  map[string]string{"#sort": sortField},
		sortValues: map[string]types.AttributeValue{":val": scoreAV},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("leaderboard: count better: %w", err)
	}
	tiedBefore, err := s.countGSI(ctx, index, pkField, mode, countOpts{
		sortCond:   "#sort = :val",
		sortNames:  map[string]string{"#sort": sortField},
		sortValues: map[string]types.AttributeValue{":val": scoreAV},
		filterCond: "pk < :pid",
		filterValues: map[string]types.AttributeValue{
			":pid": &types.AttributeValueMemberS{Value: entry.PlayerID},
		},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("leaderboard: count tied: %w", err)
	}
	total, err = s.countGSI(ctx, index, pkField, mode, countOpts{})
	if err != nil {
		return 0, 0, fmt.Errorf("leaderboard: count total: %w", err)
	}
	return better + tiedBefore + 1, total, nil
}

// scoreFor extracts the value entry is ranked by for metric, matching
// Service.Top's sort keys.
func scoreFor(metric string, entry Entry) float64 {
	switch metric {
	case "hands_played":
		return float64(entry.HandsPlayed)
	case "win_rate":
		return entry.WinRate
	default:
		return float64(entry.HandsWon)
	}
}

// formatScore renders score as the DynamoDB Number literal it must exactly
// equal for a tied-score comparison to match: win_rate_score is materialized
// with 9 decimal places (materializeWinRate), the integer metrics need none.
func formatScore(score float64) string {
	if score == float64(int64(score)) {
		return strconv.FormatInt(int64(score), 10)
	}
	return strconv.FormatFloat(score, 'f', 9, 64)
}

// countOpts configures countGSI's key condition (always PK equality on mode,
// plus an optional comparison on the metric's sort key) and an optional
// FilterExpression evaluated after it (e.g. narrowing a tied-score page down
// to rows ordered before one player_id).
type countOpts struct {
	sortCond     string
	sortNames    map[string]string
	sortValues   map[string]types.AttributeValue
	filterCond   string
	filterNames  map[string]string
	filterValues map[string]types.AttributeValue
}

// countGSI runs a Select:COUNT query against index, paginating until
// exhausted (or maxRankCountPages) and summing each page's Count — which
// DynamoDB reports post-filter, so a FilterExpression in opts is reflected
// correctly in the total.
func (s *Store) countGSI(ctx context.Context, index, pkField, mode string, opts countOpts) (int64, error) {
	names := map[string]string{"#pk": pkField}
	values := map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: mode}}
	cond := "#pk = :pk"
	if opts.sortCond != "" {
		cond += " AND " + opts.sortCond
		for k, v := range opts.sortNames {
			names[k] = v
		}
		for k, v := range opts.sortValues {
			values[k] = v
		}
	}
	for k, v := range opts.filterNames {
		names[k] = v
	}
	for k, v := range opts.filterValues {
		values[k] = v
	}

	var total int64
	var startKey map[string]types.AttributeValue
	for page := 0; page < maxRankCountPages; page++ {
		input := &dynamodb.QueryInput{
			IndexName:                 aws.String(index),
			KeyConditionExpression:    aws.String(cond),
			ExpressionAttributeNames:  names,
			ExpressionAttributeValues: values,
			Select:                    types.SelectCount,
			ExclusiveStartKey:         startKey,
		}
		if opts.filterCond != "" {
			input.FilterExpression = aws.String(opts.filterCond)
		}
		out, err := s.base.QueryRaw(ctx, input)
		if err != nil {
			return 0, fmt.Errorf("leaderboard: count query: %w", err)
		}
		total += int64(out.Count)
		if out.LastEvaluatedKey == nil {
			break
		}
		startKey = out.LastEvaluatedKey
	}
	return total, nil
}
