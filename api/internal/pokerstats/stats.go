// Package pokerstats materializes private, player-scoped poker tendencies
// from the authoritative action log.
package pokerstats

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	dynamo "gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

const (
	tableStats   = "poker_player_poker_stats"
	guardTTLDays = 90
)

type HandMetric struct {
	PlayerID       string
	VPIP           bool
	PFR            bool
	ThreeBet       bool
	ThreeBetChance bool
}

type Stats struct {
	Hands           int64   `dynamodbav:"hands" json:"hands"`
	VPIPHands       int64   `dynamodbav:"vpip_hands" json:"vpip_hands"`
	PFRHands        int64   `dynamodbav:"pfr_hands" json:"pfr_hands"`
	ThreeBetHands   int64   `dynamodbav:"three_bet_hands" json:"three_bet_hands"`
	ThreeBetChances int64   `dynamodbav:"three_bet_chances" json:"three_bet_chances"`
	VPIPRate        float64 `dynamodbav:"-" json:"vpip_rate"`
	PFRRate         float64 `dynamodbav:"-" json:"pfr_rate"`
	ThreeBetRate    float64 `dynamodbav:"-" json:"three_bet_rate"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableStats)}
}

// RecordHand atomically creates one hand guard and increments every
// participant. A duplicate completion callback therefore changes no counter.
func (s *Store) RecordHand(ctx context.Context, tableID, handID string, metrics []HandMetric) error {
	if tableID == "" || handID == "" || len(metrics) == 0 {
		return nil
	}
	guard, err := dynamo.Encode(struct {
		PK  string `dynamodbav:"pk"`
		TTL int64  `dynamodbav:"ttl"`
	}{
		PK:  "guard#" + tableID + "#" + handID,
		TTL: time.Now().Add(guardTTLDays * 24 * time.Hour).Unix(),
	})
	if err != nil {
		return fmt.Errorf("pokerstats: encode guard: %w", err)
	}
	items := []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(guard)}
	for _, metric := range metrics {
		if metric.PlayerID == "" {
			continue
		}
		values := map[string]types.AttributeValue{
			":hands":  number(1),
			":vpip":   number(boolInt(metric.VPIP)),
			":pfr":    number(boolInt(metric.PFR)),
			":three":  number(boolInt(metric.ThreeBet)),
			":chance": number(boolInt(metric.ThreeBetChance)),
			":now":    &types.AttributeValueMemberS{Value: dynamo.NowStr()},
		}
		items = append(items, s.base.BuildRawUpdateTxItem(
			"stats#"+metric.PlayerID, nil,
			"ADD hands :hands, vpip_hands :vpip, pfr_hands :pfr, three_bet_hands :three, three_bet_chances :chance SET updated_at = :now",
			"", nil, values,
		))
	}
	if len(items) == 1 {
		return nil
	}
	if err := s.base.TransactWrite(ctx, items); err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil
		}
		return fmt.Errorf("pokerstats: record hand: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, playerID string) (Stats, error) {
	item, err := s.base.GetItem(ctx, "stats#"+playerID)
	if err != nil {
		return Stats{}, fmt.Errorf("pokerstats: get: %w", err)
	}
	if item == nil {
		return Stats{}, nil
	}
	stats, err := dynamo.Decode[Stats](item)
	if err != nil {
		return Stats{}, fmt.Errorf("pokerstats: decode: %w", err)
	}
	stats.calculateRates()
	return *stats, nil
}

func (s *Stats) calculateRates() {
	if s.Hands > 0 {
		s.VPIPRate = float64(s.VPIPHands) / float64(s.Hands)
		s.PFRRate = float64(s.PFRHands) / float64(s.Hands)
	}
	if s.ThreeBetChances > 0 {
		s.ThreeBetRate = float64(s.ThreeBetHands) / float64(s.ThreeBetChances)
	}
}

func number(value int) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.Itoa(value)}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// Analyze derives one binary metric set per participant. It processes only
// decisions made before the first public board card and ignores blind/system
// log entries. An old ambiguous "all_in" row counts for VPIP but never PFR.
func Analyze(participants []string, actions []tablestore.ActionLogEntry) []HandMetric {
	metrics := make(map[string]*HandMetric, len(participants))
	for _, id := range participants {
		if id != "" {
			metrics[id] = &HandMetric{PlayerID: id}
		}
	}
	raiseCount := 0
	hasRaised := make(map[string]bool)
	for _, entry := range actions {
		metric := metrics[entry.PlayerID]
		if metric == nil {
			if entry.Frame != nil && len(entry.Frame.Board) > 0 {
				break
			}
			continue
		}
		action := entry.BettingAction
		if action == "" {
			action = entry.Action
		}
		isRaise := action == "raise" || action == "bet"
		isVoluntary := action == "call" || isRaise || (entry.Action == "all_in" && entry.BettingAction == "")
		if isVoluntary {
			metric.VPIP = true
		}
		if raiseCount > 0 && !hasRaised[entry.PlayerID] &&
			(action == "fold" || action == "call" || action == "check" || isRaise) {
			metric.ThreeBetChance = true
		}
		if isRaise {
			metric.PFR = true
			if raiseCount == 1 && !hasRaised[entry.PlayerID] {
				metric.ThreeBet = true
			}
			raiseCount++
			hasRaised[entry.PlayerID] = true
		}
		if entry.Frame != nil && len(entry.Frame.Board) > 0 {
			break
		}
	}
	out := make([]HandMetric, 0, len(metrics))
	for _, id := range participants {
		if metric := metrics[id]; metric != nil {
			out = append(out, *metric)
		}
	}
	return out
}
