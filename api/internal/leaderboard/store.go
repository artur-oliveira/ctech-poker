package leaderboard

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/valkey-io/valkey-go"
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

type Store struct {
	base dynamo.Base
	// mirror is the Valkey sorted-set rank mirror (issue #202). nil falls
	// back to the three-COUNT path below, which is correct but scales with
	// the mode's whole player base.
	mirror *rankMirror
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableStats)}
}

// WithRankMirror enables the Valkey-backed rank mirror for RankOf. Without it
// (dev without Valkey, tests) RankOf keeps its DynamoDB COUNT behaviour.
func (s *Store) WithRankMirror(client valkey.Client) *Store {
	s.mirror = &rankMirror{client: client, ttl: RankMirrorTTL}
	return s
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
	// by syncWinRateRow once the min-hands floor (issue #63) is known from the
	// post-update counters — and that follow-up write is skipped entirely while
	// the row stays below the floor (issue #217).
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
	return s.syncWinRateRow(ctx, playerID, mode, played, won, isRanked(out.Attributes))
}

// isRanked reports whether a stats row currently carries the sparse
// gsi_win_rate_pk key, i.e. whether it is on the win_rate board right now.
func isRanked(attrs map[string]types.AttributeValue) bool {
	_, ok := attrs["gsi_win_rate_pk"]
	return ok
}

// syncWinRateRow keeps the win_rate board's two derived attributes in step
// with the counters that were just bumped: win_rate_score (the GSI's sort
// key, DynamoDB has no division in an update expression so the ratio has to
// be materialized) and the sparse gsi_win_rate_pk key that decides board
// membership at the MinHandsForWinRateRank floor (issue #63). Both move in
// ONE conditional write instead of the two this used to take.
//
// The write is skipped entirely for a row below the floor that is already off
// the board — the steady state for most players, and the second half of issue
// #217's "three writes per participant per hand". Such a row is in no GSI, so
// its stale win_rate_score is read by nobody: Service.Top and Service.MyRank
// both recompute the displayed rate from the counters, and Service.MyRank
// reports a sub-floor player as unranked. A row that still carries a stale key
// from before the floor existed is still cleaned on its owner's next hand.
//
// The condition pins the exact counter version observed, so a slower writer
// can never overwrite a newer rate; on conflict it reloads and recomputes.
func (s *Store) syncWinRateRow(ctx context.Context, playerID, mode string, played, won int64, ranked bool) error {
	sk := statsSK + "#" + mode
	key := map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: playerID}, "sk": &types.AttributeValueMemberS{Value: sk}}
	for attempt := 0; attempt < 5; attempt++ {
		eligible := played >= MinHandsForWinRateRank
		if !eligible && !ranked {
			return nil
		}
		input := &dynamodb.UpdateItemInput{
			Key:                      key,
			ConditionExpression:      new("#played = :played AND #won = :won"),
			ExpressionAttributeNames: map[string]string{"#played": "hands_played", "#won": "hands_won", "#ratepk": "gsi_win_rate_pk"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":played": &types.AttributeValueMemberN{Value: strconv.FormatInt(played, 10)},
				":won":    &types.AttributeValueMemberN{Value: strconv.FormatInt(won, 10)},
			},
		}
		if eligible {
			rate := 0.0
			if played > 0 {
				rate = float64(won) / float64(played)
			}
			input.UpdateExpression = new("SET #rate = :rate, #ratepk = :mode")
			input.ExpressionAttributeNames["#rate"] = "win_rate_score"
			input.ExpressionAttributeValues[":rate"] = &types.AttributeValueMemberN{Value: strconv.FormatFloat(rate, 'f', 9, 64)}
			input.ExpressionAttributeValues[":mode"] = &types.AttributeValueMemberS{Value: mode}
		} else {
			input.UpdateExpression = new("REMOVE #ratepk")
		}
		_, err := s.base.UpdateItemRaw(ctx, input)
		if err == nil {
			return nil
		}
		if !dynamo.IsConditionFailed(err) {
			return fmt.Errorf("leaderboard: sync win rate row: %w", err)
		}
		item, getErr := s.base.GetItem(ctx, playerID, sk)
		if getErr != nil {
			return fmt.Errorf("leaderboard: reload win rate counters: %w", getErr)
		}
		played, won, ranked = number(item["hands_played"]), number(item["hands_won"]), isRanked(item)
	}
	return fmt.Errorf("leaderboard: win rate update remained contended")
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

// IncrementAchievementPoints adds points in a single ADD, alongside the two
// dense GSI keys the row needs to be rankable at all. It used to loop one
// AtomicIncrement per star plus an upsert, a GetItem and a rank-key write
// (issue #217).
func (s *Store) IncrementAchievementPoints(ctx context.Context, playerID, mode string, points int) error {
	sk := statsSK + "#" + mode
	out, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		Key:              map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: playerID}, "sk": &types.AttributeValueMemberS{Value: sk}},
		UpdateExpression: new("ADD #points :points SET #wonpk = :all, #playedpk = :all"),
		ExpressionAttributeNames: map[string]string{
			"#points": "achievement_points", "#wonpk": "gsi_hands_won_pk", "#playedpk": "gsi_hands_played_pk",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":points": &types.AttributeValueMemberN{Value: strconv.Itoa(points)},
			":all":    &types.AttributeValueMemberS{Value: mode},
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		return fmt.Errorf("leaderboard: increment achievement points: %w", err)
	}
	// win_rate eligibility follows hands_played only (issue #63) — achievement
	// points must not put a sub-floor row back on the win_rate board. The
	// counters did not move here, so the materialized rate is still correct and
	// only a membership mismatch (a row on the wrong side of the floor) is
	// worth a second write.
	played, ranked := number(out.Attributes["hands_played"]), isRanked(out.Attributes)
	if ranked == (played >= MinHandsForWinRateRank) {
		return nil
	}
	return s.syncWinRateRow(ctx, playerID, mode, played, number(out.Attributes["hands_won"]), ranked)
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
// mode/metric. The rank is exact, including ties: rows with a strictly better
// score come first, then rows tied on score but ordered before entry by
// player_id (the same tiebreak Service.Top's in-memory sort uses).
//
// The fast path is the Valkey rank mirror (issue #202): one Lua round trip,
// O(log n), no DynamoDB read at all beyond the caller's own stats row. A cold
// board is rebuilt by exactly one replica — one full GSI page-through per
// RankMirrorTTL for the whole fleet — while everyone else answers from the
// fallback below.
//
// The fallback is three GSI queries, each Select:COUNT and each paginating
// over its own slice (better-than, tied-before, and the mode's full partition
// for the total). It is correct but its total-count query is bounded only by
// the mode's player base; see the maxRankCountPages comment. It stays as the
// no-Valkey and mirror-degraded path.
func (s *Store) RankOf(ctx context.Context, mode, metric string, entry Entry) (rank int64, total int64, err error) {
	if s.mirror != nil {
		if rank, total, ok := s.rankFromMirror(ctx, mode, metric, entry); ok {
			return rank, total, nil
		}
	}
	return s.rankByCount(ctx, mode, metric, entry)
}

// rankFromMirror answers from the sorted set, rebuilding it once if this
// replica wins the claim. Any Valkey or DynamoDB trouble is logged and
// reported as a miss so the caller degrades to COUNT rather than failing a
// page view over a cache.
func (s *Store) rankFromMirror(ctx context.Context, mode, metric string, entry Entry) (int64, int64, bool) {
	key := rankMirrorKey(mode, metric)
	score := scoreFor(metric, entry)
	rank, total, ok, err := s.mirror.rank(ctx, key, entry.PlayerID, score)
	if err != nil {
		slog.Warn("leaderboard rank mirror read failed", "err", err, "mode", mode, "metric", metric)
		return 0, 0, false
	}
	if ok {
		return rank, total, true
	}
	claimed, err := s.mirror.claimRebuild(ctx, key)
	if err != nil {
		slog.Warn("leaderboard rank mirror claim failed", "err", err, "mode", mode, "metric", metric)
		return 0, 0, false
	}
	if !claimed {
		return 0, 0, false
	}
	members, err := s.loadBoardMembers(ctx, mode, metric)
	if err != nil {
		slog.Warn("leaderboard rank mirror rebuild failed", "err", err, "mode", mode, "metric", metric)
		// Still release the claim, otherwise nobody retries for its whole TTL.
		if pubErr := s.mirror.publish(ctx, key, nil); pubErr != nil {
			slog.Warn("leaderboard rank mirror release failed", "err", pubErr)
		}
		return 0, 0, false
	}
	if err := s.mirror.publish(ctx, key, members); err != nil {
		slog.Warn("leaderboard rank mirror publish failed", "err", err, "mode", mode, "metric", metric)
		return 0, 0, false
	}
	rank, total, ok, err = s.mirror.rank(ctx, key, entry.PlayerID, score)
	if err != nil || !ok {
		return 0, 0, false
	}
	return rank, total, true
}

func (s *Store) rankByCount(ctx context.Context, mode, metric string, entry Entry) (rank int64, total int64, err error) {
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
// with 9 decimal places (syncWinRateRow), the integer metrics need none.
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
