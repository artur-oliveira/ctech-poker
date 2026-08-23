// Package matchup materializes an incrementally-updated head-to-head
// aggregate for every unordered pair of players who have shared a hand,
// following internal/pokerstats's exact shape (Store{base dynamo.Base}) so
// a per-opponent comparator never has to page a player's entire hand
// history at query time (see docs/specs/2026-08-21-head-to-head-stats.md's
// "Why not derive this from existing history at query time").
package matchup

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

const (
	tableMatchups = "poker_player_matchups"
	guardTTLDays  = 90
)

// Stats is one unordered player pair's cumulative head-to-head record,
// scoped by currency mode. Every field is relative to idLow/idHigh (see
// pairKey), never to a "viewer" — the same item is correct regardless of
// which of the two players queries it; viewer/opponent remapping happens
// once, in the read handler (internal/api/v1), not here.
type Stats struct {
	HandsTogether        int64 `dynamodbav:"hands_together"`
	WinsLow              int64 `dynamodbav:"wins_low"`
	WinsHigh             int64 `dynamodbav:"wins_high"`
	Ties                 int64 `dynamodbav:"ties"`
	HeadsUpHandsTogether int64 `dynamodbav:"heads_up_hands_together"`
	// NetChangeLow/NetChangeHigh are each player's own cumulative
	// Payouts-Contributions across heads-up hands only (a 3+-way pot has no
	// single correct per-opponent attribution — see deltasFor). Two
	// independent fields, not one negated field: HandOutcome.Payouts is net
	// of rake but Contributions is gross, so a heads-up hand is not
	// zero-sum between the two players and idHigh's result can never be
	// safely derived as -NetChangeLow.
	NetChangeLow  int64 `dynamodbav:"net_change_low"`
	NetChangeHigh int64 `dynamodbav:"net_change_high"`
}

// PairStats is one Get result: the raw idLow/idHigh-relative Stats plus
// which supplied player landed on which side, so a caller can remap to
// viewer/opponent.
type PairStats struct {
	IDLow  string
	IDHigh string
	Stats  Stats
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableMatchups)}
}

// pairKey returns the pair's item key plus the two ids in lexicographic
// order, so the same unordered pair always resolves to the same item
// regardless of who's "viewer."
func pairKey(mode, a, b string) (key, idLow, idHigh string) {
	idLow, idHigh = a, b
	if idHigh < idLow {
		idLow, idHigh = idHigh, idLow
	}
	return "pair#" + mode + "#" + idLow + "#" + idHigh, idLow, idHigh
}

// pairDelta is one hand's contribution to one unordered pair, computed pure
// (no I/O) so the counting logic is unit-testable without DynamoDB.
type pairDelta struct {
	key, idLow, idHigh                     string
	handsTogether, winsLow, winsHigh, ties int64
	headsUp                                bool
	netLow, netHigh                        int64
}

// deltasFor computes one pairDelta per unordered pair within
// outcome.Participants. Hand-win/loss/tie is well-defined for any table
// size using membership in outcome.Winners (the same check
// internal/app's handItemFor uses); NetChangeLow/NetChangeHigh only move on
// a genuine heads-up hand (exactly 2 participants) — publishing a
// per-opponent chip result for a 3+-way pot would be quietly wrong for
// every hand a third player contributed to or won.
func deltasFor(mode string, outcome hand.HandOutcome) []pairDelta {
	if len(outcome.Participants) < 2 {
		return nil
	}
	winners := make(map[string]bool, len(outcome.Winners))
	for _, id := range outcome.Winners {
		winners[id] = true
	}
	headsUp := len(outcome.Participants) == 2
	var deltas []pairDelta
	for i := 0; i < len(outcome.Participants); i++ {
		for j := i + 1; j < len(outcome.Participants); j++ {
			a, b := outcome.Participants[i], outcome.Participants[j]
			if a == "" || b == "" || a == b {
				continue
			}
			key, idLow, idHigh := pairKey(mode, a, b)
			d := pairDelta{key: key, idLow: idLow, idHigh: idHigh, handsTogether: 1, headsUp: headsUp}
			switch {
			case winners[idLow] && winners[idHigh]:
				d.ties = 1
			case winners[idLow]:
				d.winsLow = 1
			case winners[idHigh]:
				d.winsHigh = 1
			}
			if headsUp {
				d.netLow = outcome.Payouts[idLow] - outcome.Contributions[idLow]
				d.netHigh = outcome.Payouts[idHigh] - outcome.Contributions[idHigh]
			}
			deltas = append(deltas, d)
		}
	}
	return deltas
}

// RecordHand atomically applies one completed hand to every unordered pair
// within outcome.Participants. Each pair carries its own create-only
// idempotency guard inside the same TransactWriteItems call (mirrors
// pokerstats.Store.RecordHand's guard-plus-increments shape, extended to
// per-pair guards since one hand now touches many independent items), so a
// duplicate onHandComplete invocation for the same hand double-counts no
// pair — and, since all guards ride in one transaction, either the whole
// hand's pairs commit or none do. tableID disambiguates the guard because
// hand ids are only unique within a table (mirrors
// pokerstats.Store.RecordHand's "guard#"+tableID+"#"+handID key). A 9-max
// table caps this at C(9,2)=36 pairs * 2 items (guard + update) = 72 items,
// under DynamoDB's 100-item TransactWriteItems limit.
func (s *Store) RecordHand(ctx context.Context, mode, tableID, handID string, outcome hand.HandOutcome) error {
	if tableID == "" || handID == "" {
		return nil
	}
	deltas := deltasFor(mode, outcome)
	if len(deltas) == 0 {
		return nil
	}
	items := make([]types.TransactWriteItem, 0, len(deltas)*2)
	for _, d := range deltas {
		guard, err := dynamo.Encode(struct {
			PK  string `dynamodbav:"pk"`
			TTL int64  `dynamodbav:"ttl"`
		}{
			PK:  "guard#" + tableID + "#" + handID + "#" + d.key,
			TTL: time.Now().Add(guardTTLDays * 24 * time.Hour).Unix(),
		})
		if err != nil {
			return fmt.Errorf("matchup: encode guard: %w", err)
		}
		items = append(items, s.base.BuildPutTxItemIfAbsent(guard))

		values := map[string]types.AttributeValue{
			":hands":   number(d.handsTogether),
			":winLow":  number(d.winsLow),
			":winHigh": number(d.winsHigh),
			":tie":     number(d.ties),
			":now":     &types.AttributeValueMemberS{Value: dynamo.NowStr()},
		}
		updateExpr := "ADD hands_together :hands, wins_low :winLow, wins_high :winHigh, ties :tie SET updated_at = :now"
		if d.headsUp {
			values[":huHands"] = number(1)
			values[":netLow"] = number(d.netLow)
			values[":netHigh"] = number(d.netHigh)
			updateExpr = "ADD hands_together :hands, wins_low :winLow, wins_high :winHigh, ties :tie, " +
				"heads_up_hands_together :huHands, net_change_low :netLow, net_change_high :netHigh SET updated_at = :now"
		}
		items = append(items, s.base.BuildRawUpdateTxItem(d.key, nil, updateExpr, "", nil, values))
	}
	if err := s.base.TransactWrite(ctx, items); err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil
		}
		return fmt.Errorf("matchup: record hand: %w", err)
	}
	return nil
}

// Get returns playerA/playerB's head-to-head stats, zeroed (not an error)
// when the pair has never shared a hand — a pair with no history isn't an
// error, same as pokerstats.Store.Get's item==nil branch.
func (s *Store) Get(ctx context.Context, mode, playerA, playerB string) (PairStats, error) {
	key, idLow, idHigh := pairKey(mode, playerA, playerB)
	result := PairStats{IDLow: idLow, IDHigh: idHigh}
	item, err := s.base.GetItem(ctx, key)
	if err != nil {
		return PairStats{}, fmt.Errorf("matchup: get: %w", err)
	}
	if item == nil {
		return result, nil
	}
	stats, err := dynamo.Decode[Stats](item)
	if err != nil {
		return PairStats{}, fmt.Errorf("matchup: decode: %w", err)
	}
	result.Stats = *stats
	return result, nil
}

func number(v int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)}
}
