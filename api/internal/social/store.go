package social

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableSocialEdges = "poker_social_edges"
	// gsiSocialRelationship is sparse by construction: `relationship` is
	// written with omitempty, so a mute- or block-only edge carries no sort
	// key and stays out of the index.
	gsiSocialRelationship = "gsi_relationship"
)

var ErrConcurrentTransition = errors.New("social: relationship changed concurrently")

// EdgeStore owns all mirrored relationship transitions. Implementations must
// use one conditional DynamoDB transaction for both directed rows.
type EdgeStore interface {
	Get(ctx context.Context, ownerPlayerID, otherPlayerID string) (*Edge, error)
	List(ctx context.Context, ownerPlayerID string, relationship Relationship, blockedOnly bool, limit int, startKey map[string]types.AttributeValue) ([]Edge, map[string]types.AttributeValue, error)
	Count(ctx context.Context, ownerPlayerID string, relationship Relationship, saturateAt int) (int, error)
	Apply(ctx context.Context, transition Transition) (*Edge, error)
}

type Store struct {
	base dynamo.Base
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableSocialEdges)}
}

func (s *Store) Get(ctx context.Context, ownerPlayerID, otherPlayerID string) (*Edge, error) {
	item, err := s.base.GetItem(ctx, ownerPlayerID, otherPlayerID)
	if err != nil || item == nil {
		return nil, err
	}
	edge, err := dynamo.Decode[Edge](item)
	if err != nil {
		return nil, fmt.Errorf("social: decode edge: %w", err)
	}
	return edge, nil
}

func (s *Store) BlockedInEitherDirection(ctx context.Context, actorID string, opponentIDs []string) (map[string]bool, error) {
	blocked := make(map[string]bool, len(opponentIDs))
	keys := make([]map[string]types.AttributeValue, 0, len(opponentIDs)*2)
	seen := make(map[string]bool, len(opponentIDs))
	for _, opponentID := range opponentIDs {
		if opponentID == "" || opponentID == actorID || seen[opponentID] {
			continue
		}
		seen[opponentID] = true
		keys = append(keys, edgeKey(actorID, opponentID), edgeKey(opponentID, actorID))
	}
	if len(keys) == 0 {
		return blocked, nil
	}
	request := map[string]types.KeysAndAttributes{s.base.TableName: {Keys: keys}}
	for attempt := 0; attempt < 4 && len(request) > 0; attempt++ {
		out, err := s.base.BatchGetItemRaw(ctx, &dynamodb.BatchGetItemInput{RequestItems: request})
		if err != nil {
			return nil, fmt.Errorf("social: batch read block pairs: %w", err)
		}
		for _, raw := range out.Responses[s.base.TableName] {
			edge, decodeErr := dynamo.Decode[Edge](raw)
			if decodeErr != nil {
				return nil, fmt.Errorf("social: decode block pair: %w", decodeErr)
			}
			if edge.Blocked {
				otherID := edge.OtherPlayerID
				if edge.OwnerPlayerID != actorID {
					otherID = edge.OwnerPlayerID
				}
				blocked[otherID] = true
			}
		}
		request = out.UnprocessedKeys
	}
	if len(request) > 0 {
		return nil, fmt.Errorf("social: block pair reads remained unprocessed")
	}
	return blocked, nil
}

func (s *Store) GetManyFromOwner(ctx context.Context, ownerID string, otherIDs []string) (map[string]Edge, error) {
	result := make(map[string]Edge, len(otherIDs))
	keys := make([]map[string]types.AttributeValue, 0, len(otherIDs))
	seen := make(map[string]bool, len(otherIDs))
	for _, otherID := range otherIDs {
		if otherID != "" && otherID != ownerID && !seen[otherID] {
			seen[otherID] = true
			keys = append(keys, edgeKey(ownerID, otherID))
		}
	}
	if len(keys) == 0 {
		return result, nil
	}
	request := map[string]types.KeysAndAttributes{s.base.TableName: {Keys: keys}}
	for attempt := 0; attempt < 4 && len(request) > 0; attempt++ {
		out, err := s.base.BatchGetItemRaw(ctx, &dynamodb.BatchGetItemInput{RequestItems: request})
		if err != nil {
			return nil, fmt.Errorf("social: batch read owner edges: %w", err)
		}
		for _, raw := range out.Responses[s.base.TableName] {
			edge, decodeErr := dynamo.Decode[Edge](raw)
			if decodeErr != nil {
				return nil, fmt.Errorf("social: decode owner edge: %w", decodeErr)
			}
			result[edge.OtherPlayerID] = *edge
		}
		request = out.UnprocessedKeys
	}
	if len(request) > 0 {
		return nil, fmt.Errorf("social: owner edge reads remained unprocessed")
	}
	return result, nil
}

func (s *Store) readPair(ctx context.Context, actorID, targetID string) (*Edge, *Edge, error) {
	responses, err := s.base.TransactGetItems(ctx, []types.TransactGetItem{
		{Get: &types.Get{TableName: aws.String(s.base.TableName), Key: edgeKey(actorID, targetID)}},
		{Get: &types.Get{TableName: aws.String(s.base.TableName), Key: edgeKey(targetID, actorID)}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("social: read relationship pair: %w", err)
	}
	var actor, target *Edge
	if len(responses) > 0 && responses[0].Item != nil {
		actor, err = dynamo.Decode[Edge](responses[0].Item)
		if err != nil {
			return nil, nil, fmt.Errorf("social: decode actor edge: %w", err)
		}
	}
	if len(responses) > 1 && responses[1].Item != nil {
		target, err = dynamo.Decode[Edge](responses[1].Item)
		if err != nil {
			return nil, nil, fmt.Errorf("social: decode target edge: %w", err)
		}
	}
	return actor, target, nil
}

func edgeKey(ownerID, otherID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: ownerID},
		"sk": &types.AttributeValueMemberS{Value: otherID},
	}
}

// Apply retries only after re-reading both rows. A rejected transaction is
// never treated as committed state.
func (s *Store) Apply(ctx context.Context, transition Transition) (*Edge, error) {
	for attempt := 0; attempt < 4; attempt++ {
		actorBefore, targetBefore, err := s.readPair(ctx, transition.ActorPlayerID, transition.TargetPlayerID)
		if err != nil {
			return nil, err
		}
		actorAfter, targetAfter, err := PlanTransition(actorBefore, targetBefore, transition)
		if err != nil {
			return nil, err
		}
		items, err := s.writePair(actorBefore, actorAfter, targetBefore, targetAfter)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return actorAfter, nil
		}
		if err := s.base.TransactWrite(ctx, items); err == nil {
			return actorAfter, nil
		} else if !dynamo.IsConditionFailed(err) {
			return nil, fmt.Errorf("social: apply transition: %w", err)
		}
	}
	return nil, ErrConcurrentTransition
}

func (s *Store) writePair(actorBefore, actorAfter, targetBefore, targetAfter *Edge) ([]types.TransactWriteItem, error) {
	items := make([]types.TransactWriteItem, 0, 2)
	for _, pair := range [][2]*Edge{{actorBefore, actorAfter}, {targetBefore, targetAfter}} {
		before, after := pair[0], pair[1]
		if edgeEqual(before, after) {
			continue
		}
		item, err := s.conditionalWrite(before, after)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) conditionalWrite(before, after *Edge) (types.TransactWriteItem, error) {
	var key map[string]types.AttributeValue
	if after != nil {
		key = edgeKey(after.OwnerPlayerID, after.OtherPlayerID)
	} else {
		key = edgeKey(before.OwnerPlayerID, before.OtherPlayerID)
	}
	condition := "attribute_not_exists(pk)"
	values := map[string]types.AttributeValue(nil)
	names := map[string]string(nil)
	if before != nil {
		condition = "#version = :version"
		names = map[string]string{"#version": "version"}
		values = map[string]types.AttributeValue{":version": &types.AttributeValueMemberN{Value: strconv.FormatInt(before.Version, 10)}}
	}
	if after == nil {
		return types.TransactWriteItem{Delete: &types.Delete{
			TableName: s.tableName(), Key: key, ConditionExpression: aws.String(condition),
			ExpressionAttributeNames: names, ExpressionAttributeValues: values,
		}}, nil
	}
	encoded, err := dynamo.Encode(*after)
	if err != nil {
		return types.TransactWriteItem{}, fmt.Errorf("social: encode edge: %w", err)
	}
	return types.TransactWriteItem{Put: &types.Put{
		TableName: s.tableName(), Item: encoded, ConditionExpression: aws.String(condition),
		ExpressionAttributeNames: names, ExpressionAttributeValues: values,
	}}, nil
}

func (s *Store) tableName() *string { return aws.String(s.base.TableName) }

func (s *Store) List(ctx context.Context, ownerPlayerID string, relationship Relationship, blockedOnly bool, limit int, startKey map[string]types.AttributeValue) ([]Edge, map[string]types.AttributeValue, error) {
	if limit < 1 || limit > 50 {
		limit = 50
	}
	names := map[string]string{}
	values := map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: ownerPlayerID}}
	filter := ""
	if relationship != RelationshipNone {
		filter = "#relationship = :relationship"
		names["#relationship"] = "relationship"
		values[":relationship"] = &types.AttributeValueMemberS{Value: string(relationship)}
	}
	if blockedOnly {
		if filter != "" {
			filter += " AND "
		}
		filter += "#blocked = :blocked"
		names["#blocked"] = "blocked"
		values[":blocked"] = &types.AttributeValueMemberBOOL{Value: true}
	}
	input := &dynamodb.QueryInput{
		KeyConditionExpression: aws.String("pk = :pk"), ExpressionAttributeValues: values,
		Limit: aws.Int32(int32(limit)), ExclusiveStartKey: startKey,
	}
	if filter != "" {
		input.FilterExpression = aws.String(filter)
		input.ExpressionAttributeNames = names
	}
	out, err := s.base.QueryRaw(ctx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("social: list edges: %w", err)
	}
	edges := make([]Edge, 0, len(out.Items))
	for _, item := range out.Items {
		edge, err := dynamo.Decode[Edge](item)
		if err != nil {
			return nil, nil, fmt.Errorf("social: decode listed edge: %w", err)
		}
		edges = append(edges, *edge)
	}
	return edges, out.LastEvaluatedKey, nil
}

// maxCountPages bounds Count's pagination. A page reads at most `saturateAt`
// items, so a single Count evaluates at most maxCountPages*saturateAt rows —
// the "explicitly limited" budget issue #208 asks for, in place of the
// unbounded loop this used to be.
const maxCountPages = 8

// Count returns how many of ownerPlayerID's edges carry relationship,
// saturating at saturateAt.
//
// Every caller compares the answer against a cap (MaxFriends,
// MaxPendingOutgoing), never displays it, so counting past that cap buys
// nothing and used to cost a page-through of the player's whole edge
// partition — friends, blocks, mutes and every incoming request alike.
// Stopping at the cap turns the common case into one query.
//
// relationship is a key condition on the sparse gsi_relationship index rather
// than a FilterExpression (#278): every row the query reads is a row that
// counts, so a partition padded with tens of thousands of non-matching rows
// can no longer exhaust the page budget and under-report. The index reuses
// the base table's own `pk`/`relationship` attributes, so rows written before
// it existed are in it without a backfill.
func (s *Store) Count(ctx context.Context, ownerPlayerID string, relationship Relationship, saturateAt int) (int, error) {
	if saturateAt <= 0 {
		return 0, nil
	}
	total := 0
	var start map[string]types.AttributeValue
	for page := 0; page < maxCountPages && total < saturateAt; page++ {
		out, err := s.base.QueryRaw(ctx, &dynamodb.QueryInput{
			IndexName:              aws.String(gsiSocialRelationship),
			KeyConditionExpression: aws.String("pk = :pk AND #relationship = :relationship"),
			ExpressionAttributeNames: map[string]string{
				"#relationship": "relationship",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":           &types.AttributeValueMemberS{Value: ownerPlayerID},
				":relationship": &types.AttributeValueMemberS{Value: string(relationship)},
			},
			ExclusiveStartKey: start,
			Limit:             aws.Int32(int32(saturateAt)),
			Select:            types.SelectCount,
		})
		if err != nil {
			return 0, fmt.Errorf("social: count relationships: %w", err)
		}
		total += int(out.Count)
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		start = out.LastEvaluatedKey
	}
	if total > saturateAt {
		total = saturateAt
	}
	return total, nil
}
