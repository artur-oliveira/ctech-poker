// api/internal/roomstore/dynamo.go
package roomstore

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableRooms   = "poker_rooms"
	gsiPublic    = "gsi_public"
	gsiShareCode = "gsi_share_code"

	roomSK         = "meta"
	inviteSKPrefix = "invite#"
)

type Store struct {
	base dynamo.Base
}

func (s *Store) GetInviteGrant(ctx context.Context, roomID, playerID string) (*InviteGrant, error) {
	item, err := s.base.GetItem(ctx, roomID, inviteSKPrefix+playerID)
	if err != nil {
		return nil, fmt.Errorf("roomstore: get invite grant: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	grant, err := dynamo.Decode[InviteGrant](item)
	if err != nil {
		return nil, fmt.Errorf("roomstore: decode invite grant: %w", err)
	}
	if grant.ExpiresAt <= time.Now().UTC().UnixMilli() {
		return nil, nil
	}
	return grant, nil
}

func (s *Store) HasInviteGrant(ctx context.Context, roomID, playerID string) (bool, error) {
	grant, err := s.GetInviteGrant(ctx, roomID, playerID)
	return grant != nil, err
}

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableRooms)}
}

func (s *Store) Create(ctx context.Context, r Room) error {
	item, err := dynamo.Encode(struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
		Room
		GSIPublic    string `dynamodbav:"gsi_public,omitempty"`
		GSIShareCode string `dynamodbav:"gsi_share_code,omitempty"`
	}{
		PK: r.ID, SK: roomSK, Room: r,
		GSIPublic:    publicIndexValue(r),
		GSIShareCode: r.ShareCode,
	})
	if err != nil {
		return fmt.Errorf("roomstore: encode: %w", err)
	}
	return s.base.PutItem(ctx, item)
}

// publicIndexValue is set only for public rooms — a sparse GSI so private
// rooms never appear in the public lobby listing, by construction rather
// than by an application-level filter that could be forgotten at a new call
// site.
func publicIndexValue(r Room) string {
	if r.Visibility == "public" {
		return "public"
	}
	return ""
}

// SetSeatsTaken persists the table actor's live occupied-seat count so the
// lobby can list "active tables" without querying tablemanager.
func (s *Store) SetSeatsTaken(ctx context.Context, roomID string, seatsTaken int) error {
	sk := roomSK
	ok, err := s.base.UpdateItem(ctx, roomID, &sk, map[string]any{"seats_taken": seatsTaken})
	if err != nil {
		return fmt.Errorf("roomstore: set seats taken: %w", err)
	}
	if !ok {
		return fmt.Errorf("roomstore: room disappeared while setting seats taken")
	}
	return nil
}

// Delete removes roomID's record entirely, dropping it out of gsi_public
// along with it. Room PK == the table's PK (same ID space), so
// cmd/tablecleanup calls this with the same tableID it just archived.
func (s *Store) Delete(ctx context.Context, roomID string) error {
	_, err := s.base.DeleteItem(ctx, roomID, roomSK)
	if err != nil {
		return fmt.Errorf("roomstore: delete: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, roomID string) (*Room, error) {
	item, err := s.base.GetItem(ctx, roomID, roomSK)
	if err != nil {
		return nil, fmt.Errorf("roomstore: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Room](item)
}

func (s *Store) GetByShareCode(ctx context.Context, code string) (*Room, error) {
	result, err := s.base.QueryGSI(ctx, gsiShareCode, "gsi_share_code", code, 1, nil)
	if err != nil {
		return nil, fmt.Errorf("roomstore: query share code: %w", err)
	}
	if len(result.Items) == 0 {
		return nil, nil
	}
	return dynamo.Decode[Room](result.Items[0])
}

func (s *Store) ListPublic(ctx context.Context, limit int, startKey map[string]types.AttributeValue) ([]Room, map[string]types.AttributeValue, error) {
	result, err := s.base.QueryGSI(ctx, gsiPublic, "gsi_public", "public", limit, startKey)
	if err != nil {
		return nil, nil, fmt.Errorf("roomstore: list public: %w", err)
	}
	out := make([]Room, 0, len(result.Items))
	for _, item := range result.Items {
		r, err := dynamo.Decode[Room](item)
		if err != nil {
			return nil, nil, fmt.Errorf("roomstore: decode: %w", err)
		}
		out = append(out, *r)
	}
	return out, result.LastEvaluatedKey, nil
}
