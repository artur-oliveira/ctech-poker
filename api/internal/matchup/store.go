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
	"log/slog"
	"slices"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

const (
	tableMatchups = "poker_player_matchups"

	// appliedHandsCap / appliedHandsKeep bound the per-pair replay memory
	// (issue #201). Each pair item carries the hand ids already applied to it
	// in an `applied_hands` string set; that set is the pair's idempotency
	// guard, so it replaces the guard *item* #65 used to write next to every
	// increment. It is pruned back to appliedHandsKeep members once it grows
	// past appliedHandsCap, which bounds both the item size and the replay
	// window: a duplicate of one of a pair's last appliedHandsKeep shared
	// hands is always rejected, an older one is not (see pruneApplied).
	appliedHandsCap  = 12
	appliedHandsKeep = 8
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

// RecordHand applies one completed hand to every unordered pair within
// outcome.Participants: exactly one plain, conditional UpdateItem per pair
// (issue #201). #65 had already chunked the old one-transaction-per-hand
// write, but the model it chunked still cost 2 transactional items per pair —
// a create-only guard item plus the increment — i.e. 72 items and ~144 WCU
// for a full ring's C(9,2)=36 pairs. Both halves of that are gone here: the
// guard moved *into* the pair item as the `applied_hands` set (so a pair
// costs one small write, not two), and with a single item per pair there is
// nothing left for a transaction to make atomic, so TransactWriteItems — and
// its 2x WCU — is gone too. A full ring now costs 36 writes / ~36 WCU.
//
// Idempotency is still per pair, and is now a property of the pair's own
// item: applyPair's condition rejects a hand id already present in that
// pair's applied_hands, so a duplicate onHandComplete double-counts no pair,
// and a run that died part-way through the pairs is completed — never
// double-applied — by a retry, since every pair is guarded independently.
func (s *Store) RecordHand(ctx context.Context, mode, tableID, handID string, outcome hand.HandOutcome) error {
	if tableID == "" || handID == "" {
		return nil
	}
	for _, d := range deltasFor(mode, outcome) {
		if err := s.applyPair(ctx, handID, d); err != nil {
			return err
		}
	}
	return nil
}

// applyPair applies one pair's delta as a single conditional UpdateItem. The
// hand id is added to the pair's applied_hands set in the same expression
// that moves the counters, so the guard can never land without its increment
// (the failure mode a separate guard item has) and the increment can never
// land twice. handID is a ULID minted per hand, so it is both unique and
// lexicographically time-ordered — pruneApplied relies on that ordering.
func (s *Store) applyPair(ctx context.Context, handID string, d pairDelta) error {
	values := map[string]types.AttributeValue{
		":hands":   number(d.handsTogether),
		":winLow":  number(d.winsLow),
		":winHigh": number(d.winsHigh),
		":tie":     number(d.ties),
		":now":     &types.AttributeValueMemberS{Value: dynamo.NowStr()},
		":hand":    &types.AttributeValueMemberS{Value: handID},
		":handSet": &types.AttributeValueMemberSS{Value: []string{handID}},
	}
	adds := "hands_together :hands, wins_low :winLow, wins_high :winHigh, ties :tie, applied_hands :handSet"
	if d.headsUp {
		values[":huHands"] = number(1)
		values[":netLow"] = number(d.netLow)
		values[":netHigh"] = number(d.netHigh)
		adds += ", heads_up_hands_together :huHands, net_change_low :netLow, net_change_high :netHigh"
	}
	out, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.base.TableName),
		Key:                       map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: d.key}},
		UpdateExpression:          aws.String("ADD " + adds + " SET updated_at = :now"),
		ConditionExpression:       aws.String("attribute_not_exists(applied_hands) OR NOT contains(applied_hands, :hand)"),
		ExpressionAttributeValues: values,
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil
		}
		return fmt.Errorf("matchup: record hand: %w", err)
	}
	s.pruneApplied(ctx, d.key, handID, out.Attributes)
	return nil
}

// pruneApplied trims a pair's applied_hands set back to the newest
// appliedHandsKeep hand ids once it passes appliedHandsCap, so the guard set
// cannot grow with the pair's lifetime and take the item — and its per-write
// WCU — up with it. A prune failure is logged, not returned: the counters are
// already committed and must not be retried, and the only cost is a set left
// one hand too long, which the pair's next hand tries to trim again.
func (s *Store) pruneApplied(ctx context.Context, pairKey, handID string, attrs map[string]types.AttributeValue) {
	set, ok := attrs["applied_hands"].(*types.AttributeValueMemberSS)
	if !ok || len(set.Value) <= appliedHandsCap {
		return
	}
	stale := staleHandIDs(set.Value, handID)
	if len(stale) == 0 {
		return
	}
	_, err := s.base.UpdateItemRaw(ctx, &dynamodb.UpdateItemInput{
		TableName:        aws.String(s.base.TableName),
		Key:              map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: pairKey}},
		UpdateExpression: aws.String("DELETE applied_hands :stale"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":stale": &types.AttributeValueMemberSS{Value: stale},
		},
	})
	if err != nil {
		slog.Warn("matchup: prune applied hands failed", "pair", pairKey, "err", err)
	}
}

// staleHandIDs returns the members of applied that pruneApplied should drop:
// everything but the newest appliedHandsKeep. Hand ids are ULIDs, so "newest"
// is simply the largest lexicographically. keepHandID (the hand just applied)
// is never dropped, so a retry of *that* hand is still rejected even when the
// prune lands first. Pure, so the arithmetic is testable without DynamoDB.
func staleHandIDs(applied []string, keepHandID string) []string {
	stale := slices.Clone(applied)
	slices.Sort(stale)
	if len(stale) <= appliedHandsKeep {
		return nil
	}
	stale = stale[:len(stale)-appliedHandsKeep]
	return slices.DeleteFunc(stale, func(id string) bool { return id == keepHandID })
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
