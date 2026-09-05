package recentplayers

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableRecentPlayers = "poker_recent_players"
	recentRetention    = 90 * 24 * time.Hour
	// eventSKPrefix namespaces the per-viewer hand rows this store writes
	// (issue #199), keeping the query that reads them clear of the
	// pk=viewer/sk=opponent aggregate rows written before #199 — those are
	// simply ignored now and expire on their own TTL.
	eventSKPrefix = "hand#"
	// maxPlayersPerHand is 9-max plus nothing: a hand with more participants
	// than seats is a bug upstream, and this store refuses it rather than
	// writing a row that would misreport a player's opponents.
	maxPlayersPerHand = 9
	// maxEventsScanned bounds every List: one Query of at most this many of
	// the viewer's most recent hand rows, coalesced in memory. **This is the
	// documented ceiling of the projection**: `hands_together` counts a
	// viewer's shared hands within their last maxEventsScanned hands (or the
	// 90-day TTL window, whichever is shorter), not since the beginning of
	// time. A "recent opponents" list is exactly that bounded question, and
	// the bound is what keeps the read one round trip instead of an
	// unbounded walk of a heavy player's history.
	maxEventsScanned = 300
	// maxBatchWriteAttempts bounds the retry of DynamoDB's UnprocessedItems.
	// Every row is a deterministic PutItem, so a retry rewrites the same
	// item — a leftover after the last attempt is reported as an error and
	// the caller (app.go's gamification pipeline) logs it.
	maxBatchWriteAttempts = 3
)

type DynamoStore struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *DynamoStore {
	return &DynamoStore{base: dynamo.NewBase(db, env, tableRecentPlayers)}
}

// handEvent is one participant's row for one completed hand: who else was at
// the table, and when. One row per viewer per hand (9 for a full ring)
// replaces the 9x8=72 directed aggregate updates the old model wrote inside a
// single transaction (issue #199) — 9 plain writes, ~9 WCU, against ~146.
//
// The row is keyed by the hand id, never by a timestamp, so writing it is
// idempotent by construction: a duplicate or retried onHandComplete rewrites
// the same item instead of incrementing a counter twice, which is why this
// store needs no idempotency guard row at all. Hand ids are ULIDs, so
// sk = "hand#"+handID also sorts chronologically, which is what lets List
// read a viewer's most recent hands straight off the table with no GSI.
type handEvent struct {
	PK        string   `dynamodbav:"pk"`
	SK        string   `dynamodbav:"sk"`
	Opponents []string `dynamodbav:"opponents"`
	PlayedAt  int64    `dynamodbav:"played_at"`
	TableID   string   `dynamodbav:"last_table_id"`
	HandID    string   `dynamodbav:"last_hand_id"`
	TTL       int64    `dynamodbav:"ttl"`
}

func (s *DynamoStore) RecordHand(ctx context.Context, hand HandCompletion) error {
	events, err := eventsFor(hand)
	if err != nil || len(events) == 0 {
		return err
	}
	requests := make([]types.WriteRequest, 0, len(events))
	for _, event := range events {
		item, encodeErr := dynamo.Encode(event)
		if encodeErr != nil {
			return fmt.Errorf("recent players: encode hand event: %w", encodeErr)
		}
		requests = append(requests, types.WriteRequest{PutRequest: &types.PutRequest{Item: item}})
	}
	return s.batchWrite(ctx, requests)
}

// eventsFor plans one hand's rows: exactly one per distinct participant,
// each listing that participant's opponents. Pure (no I/O) so the write
// budget per seat count is unit-testable without DynamoDB.
func eventsFor(hand HandCompletion) ([]handEvent, error) {
	if hand.TableID == "" || hand.HandID == "" {
		return nil, nil
	}
	players := dedupe(hand.Players)
	if len(players) > maxPlayersPerHand {
		return nil, fmt.Errorf("recent players: hand has %d players, maximum is %d", len(players), maxPlayersPerHand)
	}
	if len(players) < 2 {
		return nil, nil
	}
	playedAt := hand.PlayedAt.UTC()
	if playedAt.IsZero() {
		playedAt = time.Now().UTC()
	}
	events := make([]handEvent, 0, len(players))
	for _, viewerID := range players {
		events = append(events, handEvent{
			PK:        viewerID,
			SK:        eventSKPrefix + hand.HandID,
			Opponents: slices.DeleteFunc(slices.Clone(players), func(id string) bool { return id == viewerID }),
			PlayedAt:  playedAt.UnixMilli(),
			TableID:   hand.TableID,
			HandID:    hand.HandID,
			TTL:       playedAt.Add(recentRetention).Unix(),
		})
	}
	return events, nil
}

// batchWrite sends every viewer's row for one hand in a single
// BatchWriteItem call (one round trip, not one per participant — this runs on
// the post-hand pipeline) and retries only what DynamoDB reports as
// unprocessed, which is that API's documented way of signalling per-item
// throttling rather than failure.
func (s *DynamoStore) batchWrite(ctx context.Context, requests []types.WriteRequest) error {
	for attempt := 0; attempt < maxBatchWriteAttempts && len(requests) > 0; attempt++ {
		out, err := s.base.BatchWriteItemRaw(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{s.base.TableName: requests},
		})
		if err != nil {
			return fmt.Errorf("recent players: record hand: %w", err)
		}
		requests = out.UnprocessedItems[s.base.TableName]
	}
	if len(requests) > 0 {
		return fmt.Errorf("recent players: record hand: %d rows still unprocessed after %d attempts",
			len(requests), maxBatchWriteAttempts)
	}
	return nil
}

// List returns viewerPlayerID's opponents ordered by recency, coalesced from
// the viewer's own bounded hand rows (see maxEventsScanned). startKey carries
// an offset into that coalesced list rather than a DynamoDB
// ExclusiveStartKey: opponents are derived from many rows, so a row cursor
// cannot address a position in the result. The cursor stays opaque to the
// caller either way (internal/api/v1's decodeCursor/buildNextCursor
// round-trip any attribute map).
func (s *DynamoStore) List(ctx context.Context, viewerPlayerID string, startKey map[string]types.AttributeValue, limit int) (Page, error) {
	if limit < 1 || limit > 50 {
		limit = 50
	}
	result, err := s.base.Query(ctx, dynamo.QueryOpts{
		PK: viewerPlayerID, SKPrefix: eventSKPrefix,
		Limit: maxEventsScanned, ScanIndexForward: false,
	})
	if err != nil {
		return Page{}, fmt.Errorf("recent players: list: %w", err)
	}
	players := make([]Player, 0, len(result.Items))
	index := make(map[string]int, len(result.Items))
	cutoff := time.Now().Add(-recentRetention).UnixMilli()
	for _, raw := range result.Items {
		event, decodeErr := dynamo.Decode[handEvent](raw)
		if decodeErr != nil {
			return Page{}, fmt.Errorf("recent players: decode: %w", decodeErr)
		}
		if event.PlayedAt < cutoff {
			continue
		}
		for _, opponentID := range event.Opponents {
			if opponentID == "" || opponentID == viewerPlayerID {
				continue
			}
			if at, seen := index[opponentID]; seen {
				players[at].HandsTogether++
				continue
			}
			index[opponentID] = len(players)
			players = append(players, Player{
				ViewerPlayerID: viewerPlayerID, OpponentPlayerID: opponentID,
				LastPlayedAt: event.PlayedAt, LastTableID: event.TableID, LastHandID: event.HandID,
				HandsTogether: 1,
			})
		}
	}
	// Rows arrive newest-first, so the first sighting of an opponent is
	// already their most recent shared hand — but two opponents first seen in
	// different rows still need ordering between them, and a stable tiebreak
	// so a page boundary can't repeat or skip an opponent.
	sort.SliceStable(players, func(i, j int) bool {
		if players[i].LastPlayedAt != players[j].LastPlayedAt {
			return players[i].LastPlayedAt > players[j].LastPlayedAt
		}
		return players[i].OpponentPlayerID < players[j].OpponentPlayerID
	})
	return pageFrom(players, offsetFrom(startKey), limit), nil
}

// pageFrom slices the coalesced list at offset and reports the next offset
// when more opponents remain.
func pageFrom(players []Player, offset, limit int) Page {
	if offset >= len(players) {
		return Page{Players: []Player{}}
	}
	end := offset + limit
	if end > len(players) {
		end = len(players)
	}
	page := Page{Players: players[offset:end]}
	if end < len(players) {
		page.LastKey = map[string]types.AttributeValue{
			cursorOffsetField: &types.AttributeValueMemberN{Value: strconv.Itoa(end)},
		}
	}
	return page
}

const cursorOffsetField = "offset"

// offsetFrom reads the coalesced-list offset out of a cursor, treating
// anything unparseable (including a stale row-key cursor issued before #199)
// as "start from the beginning" — the same forgiving contract
// internal/api/v1's decodeCursor already has for a malformed cursor.
func offsetFrom(startKey map[string]types.AttributeValue) int {
	raw, ok := startKey[cursorOffsetField].(*types.AttributeValueMemberN)
	if !ok {
		return 0
	}
	offset, err := strconv.Atoi(raw.Value)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func dedupe(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
