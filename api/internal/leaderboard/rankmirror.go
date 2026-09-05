package leaderboard

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/valkey-io/valkey-go"
)

const (
	// rankMirrorPrefix namespaces one sorted set per (mode, metric) board.
	rankMirrorPrefix = "poker:leaderboard:rank:"

	// RankMirrorTTL is the freshness SLA of a materialized board: a rank or
	// total served from the mirror reflects the GSI as of at most this long
	// ago, except for the caller's own row, which is refreshed from their
	// just-read stats row on every request. The whole key expires rather
	// than being incrementally maintained, so a board can never drift
	// permanently — the worst case is one stale window.
	RankMirrorTTL = 5 * time.Minute

	// rankMirrorBuildTTL bounds how long one replica may hold the rebuild
	// claim. A replica that dies mid-rebuild only blocks other rebuilders
	// (readers fall back to the COUNT path) for this long.
	rankMirrorBuildTTL = 30 * time.Second

	// rankMirrorChunk caps how many members go into one ZADD command.
	rankMirrorChunk = 500
)

// rankScript ranks one member against an already-materialized board.
//
// It refreshes the caller's own member first so a player who has played since
// the last rebuild — or who is not in it at all — still gets an answer that is
// exact for their own row, then reads the rank and the board size. Returning
// -1 for a missing key is what tells Go to rebuild (or fall back), and is the
// only branch that costs anything beyond O(log n).
//
// The sorted set stores the NEGATED metric score, so Valkey's (score asc,
// member asc) ordering is exactly (metric desc, player_id asc) — the same
// tiebreak Service.Top sorts by. ZRANK is therefore the 0-based rank.
const rankScript = `
if redis.call('EXISTS', KEYS[1]) == 0 then return -1 end
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[2])
return {redis.call('ZRANK', KEYS[1], ARGV[2]), redis.call('ZCARD', KEYS[1])}`

// rankMirror mirrors a leaderboard GSI partition into a Valkey sorted set so
// a player's exact rank costs O(log n) in Valkey instead of three
// Select:COUNT queries that page the whole partition (issue #202).
type rankMirror struct {
	client valkey.Client
	ttl    time.Duration
}

type boardMember struct {
	playerID string
	score    float64
}

func rankMirrorKey(mode, metric string) string { return rankMirrorPrefix + mode + ":" + metric }

// mirrorScore negates score so ascending sorted-set order is best-first.
func mirrorScore(score float64) string { return strconv.FormatFloat(-score, 'g', -1, 64) }

// rank returns entry's 1-based rank and the board size, or ok=false when the
// board is not materialized right now (the caller then rebuilds, or falls back
// to the COUNT path).
func (m *rankMirror) rank(ctx context.Context, key, playerID string, score float64) (rank, total int64, ok bool, err error) {
	reply := m.client.Do(ctx, m.client.B().Eval().Script(rankScript).Numkeys(1).
		Key(key).Arg(mirrorScore(score), playerID).Build())
	if err := reply.Error(); err != nil {
		return 0, 0, false, fmt.Errorf("leaderboard: rank mirror read: %w", err)
	}
	if missing, convErr := reply.ToInt64(); convErr == nil {
		if missing == -1 {
			return 0, 0, false, nil
		}
		return 0, 0, false, fmt.Errorf("leaderboard: rank mirror returned scalar %d", missing)
	}
	values, err := reply.ToArray()
	if err != nil || len(values) != 2 {
		return 0, 0, false, fmt.Errorf("leaderboard: rank mirror reply shape: %w", err)
	}
	zeroBased, err := values[0].ToInt64()
	if err != nil {
		return 0, 0, false, fmt.Errorf("leaderboard: rank mirror rank: %w", err)
	}
	total, err = values[1].ToInt64()
	if err != nil {
		return 0, 0, false, fmt.Errorf("leaderboard: rank mirror total: %w", err)
	}
	return zeroBased + 1, total, true, nil
}

// claimRebuild takes the single-rebuilder lease for key. Only the winner pays
// the one full partition read per TTL window; everyone else answers from the
// COUNT fallback until the winner publishes, so a cold board can never turn
// into a partition-read stampede.
func (m *rankMirror) claimRebuild(ctx context.Context, key string) (bool, error) {
	resp := m.client.Do(ctx, m.client.B().Set().Key(key+":building").Value("1").
		Nx().ExSeconds(int64(rankMirrorBuildTTL.Seconds())).Build())
	if err := resp.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return false, nil
		}
		return false, fmt.Errorf("leaderboard: rank mirror claim: %w", err)
	}
	return true, nil
}

// publish writes members into a scratch key and RENAMEs it over the live one,
// so a reader either sees the previous board or the new one, never a partial
// build. An empty board publishes nothing (there is nothing to rank against);
// the claim's own TTL then paces the next attempt.
func (m *rankMirror) publish(ctx context.Context, key string, members []boardMember) error {
	defer m.client.Do(ctx, m.client.B().Del().Key(key+":building").Build())
	if len(members) == 0 {
		return nil
	}
	scratch := key + ":building:set"
	if err := m.client.Do(ctx, m.client.B().Del().Key(scratch).Build()).Error(); err != nil {
		return fmt.Errorf("leaderboard: rank mirror clear scratch: %w", err)
	}
	for start := 0; start < len(members); start += rankMirrorChunk {
		end := start + rankMirrorChunk
		if end > len(members) {
			end = len(members)
		}
		cmd := m.client.B().Zadd().Key(scratch).ScoreMember()
		for _, member := range members[start:end] {
			cmd = cmd.ScoreMember(-member.score, member.playerID)
		}
		if err := m.client.Do(ctx, cmd.Build()).Error(); err != nil {
			return fmt.Errorf("leaderboard: rank mirror zadd: %w", err)
		}
	}
	if err := m.client.Do(ctx, m.client.B().Expire().Key(scratch).Seconds(int64(m.ttl.Seconds())).Build()).Error(); err != nil {
		return fmt.Errorf("leaderboard: rank mirror expire: %w", err)
	}
	if err := m.client.Do(ctx, m.client.B().Rename().Key(scratch).Newkey(key).Build()).Error(); err != nil {
		return fmt.Errorf("leaderboard: rank mirror publish: %w", err)
	}
	return nil
}

// loadBoardMembers pages metric's GSI once for mode, projecting only the
// player id and the ranked score. This is the single unbounded read the whole
// design still contains — it is paid at most once per RankMirrorTTL per
// (mode, metric) across the fleet, instead of three times per page view.
func (s *Store) loadBoardMembers(ctx context.Context, mode, metric string) ([]boardMember, error) {
	index, pkField, sortField := gsiFor(metric)
	var members []boardMember
	var startKey map[string]types.AttributeValue
	for page := 0; page < maxRankCountPages; page++ {
		out, err := s.base.QueryRaw(ctx, &dynamodb.QueryInput{
			IndexName:              aws.String(index),
			KeyConditionExpression: aws.String("#gsipk = :mode"),
			ProjectionExpression:   aws.String("#pk, #sort"),
			ExpressionAttributeNames: map[string]string{
				"#gsipk": pkField, "#pk": "pk", "#sort": sortField,
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":mode": &types.AttributeValueMemberS{Value: mode},
			},
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, fmt.Errorf("leaderboard: load board members: %w", err)
		}
		for _, item := range out.Items {
			id, ok := item["pk"].(*types.AttributeValueMemberS)
			if !ok || id.Value == "" {
				continue
			}
			score, _ := strconv.ParseFloat(numberString(item[sortField]), 64)
			members = append(members, boardMember{playerID: id.Value, score: score})
		}
		if out.LastEvaluatedKey == nil {
			break
		}
		startKey = out.LastEvaluatedKey
	}
	return members, nil
}

func numberString(value types.AttributeValue) string {
	if n, ok := value.(*types.AttributeValueMemberN); ok {
		return n.Value
	}
	return "0"
}
