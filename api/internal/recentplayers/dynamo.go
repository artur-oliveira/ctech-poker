package recentplayers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableRecentPlayers = "poker_recent_players"
	gsiRecentPlayers   = "gsi_recent"
	recentRetention    = 90 * 24 * time.Hour
	guardRetention     = 7 * 24 * time.Hour
	recentGuardSK      = "recent_guard"
)

type DynamoStore struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *DynamoStore {
	return &DynamoStore{base: dynamo.NewBase(db, env, tableRecentPlayers)}
}

func (s *DynamoStore) RecordHand(ctx context.Context, hand HandCompletion) error {
	if hand.TableID == "" || hand.HandID == "" || len(hand.Players) < 2 {
		return nil
	}
	playedAt := hand.PlayedAt.UTC()
	if playedAt.IsZero() {
		playedAt = time.Now().UTC()
	}
	guard, err := dynamo.Encode(struct {
		PK  string `dynamodbav:"pk"`
		SK  string `dynamodbav:"sk"`
		TTL int64  `dynamodbav:"ttl"`
	}{PK: "hand#" + hand.TableID + "#" + hand.HandID, SK: recentGuardSK, TTL: playedAt.Add(guardRetention).Unix()})
	if err != nil {
		return fmt.Errorf("recent players: encode guard: %w", err)
	}
	items := []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(guard)}
	seen := make(map[string]bool, len(hand.Players))
	players := make([]string, 0, len(hand.Players))
	for _, id := range hand.Players {
		if id != "" && !seen[id] {
			seen[id] = true
			players = append(players, id)
		}
	}
	if len(players) > 9 {
		return fmt.Errorf("recent players: hand has %d players, maximum is 9", len(players))
	}
	for _, viewerID := range players {
		for _, opponentID := range players {
			if viewerID == opponentID {
				continue
			}
			values := map[string]types.AttributeValue{
				":one":    &types.AttributeValueMemberN{Value: "1"},
				":played": &types.AttributeValueMemberN{Value: strconv.FormatInt(playedAt.UnixMilli(), 10)},
				":table":  &types.AttributeValueMemberS{Value: hand.TableID},
				":hand":   &types.AttributeValueMemberS{Value: hand.HandID},
				":gpk":    &types.AttributeValueMemberS{Value: viewerID},
				":gsk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("%020d#%s", playedAt.UnixMilli(), opponentID)},
				":ttl":    &types.AttributeValueMemberN{Value: strconv.FormatInt(playedAt.Add(recentRetention).Unix(), 10)},
			}
			items = append(items, s.base.BuildRawUpdateTxItem(viewerID, new(opponentID),
				"ADD hands_together :one SET last_played_at = :played, last_table_id = :table, last_hand_id = :hand, gsi_recent_pk = :gpk, gsi_recent_sk = :gsk, #ttl = :ttl",
				"", map[string]string{"#ttl": "ttl"}, values))
		}
	}
	if len(items) == 1 {
		return nil
	}
	if err := s.base.TransactWrite(ctx, items); err != nil {
		if dynamo.IsConditionFailed(err) {
			return nil
		}
		return fmt.Errorf("recent players: record hand: %w", err)
	}
	return nil
}

func (s *DynamoStore) List(ctx context.Context, viewerPlayerID string, startKey map[string]types.AttributeValue, limit int) (Page, error) {
	if limit < 1 || limit > 50 {
		limit = 50
	}
	out, err := s.base.QueryRaw(ctx, &dynamodb.QueryInput{
		TableName: aws.String(s.base.TableName), IndexName: aws.String(gsiRecentPlayers),
		KeyConditionExpression:    aws.String("gsi_recent_pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: viewerPlayerID}},
		Limit:                     aws.Int32(int32(limit)), ScanIndexForward: aws.Bool(false), ExclusiveStartKey: startKey,
	})
	if err != nil {
		return Page{}, fmt.Errorf("recent players: list: %w", err)
	}
	page := Page{Players: make([]Player, 0, len(out.Items)), LastKey: out.LastEvaluatedKey}
	cutoff := time.Now().Add(-recentRetention).UnixMilli()
	for _, raw := range out.Items {
		item, decodeErr := dynamo.Decode[Player](raw)
		if decodeErr != nil {
			return Page{}, fmt.Errorf("recent players: decode: %w", decodeErr)
		}
		if item.OpponentPlayerID != viewerPlayerID && item.LastPlayedAt >= cutoff {
			page.Players = append(page.Players, *item)
		}
	}
	return page, nil
}
