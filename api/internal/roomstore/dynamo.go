// api/internal/roomstore/dynamo.go
package roomstore

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableRooms   = "poker_rooms"
	gsiPublic    = "gsi_public"
	gsiShareCode = "gsi_share_code"

	roomSK = "meta"
)

type Store struct {
	base dynamo.Base
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

func (s *Store) ListPublic(ctx context.Context, limit int, startKeyToken string) ([]Room, string, error) {
	result, err := s.base.QueryGSI(ctx, gsiPublic, "gsi_public", "public", limit, nil)
	if err != nil {
		return nil, "", fmt.Errorf("roomstore: list public: %w", err)
	}
	out := make([]Room, 0, len(result.Items))
	for _, item := range result.Items {
		r, err := dynamo.Decode[Room](item)
		if err != nil {
			return nil, "", fmt.Errorf("roomstore: decode: %w", err)
		}
		out = append(out, *r)
	}
	// Pagination tokens are out of scope for this MVP list (rooms count is
	// small pre-launch); startKeyToken is accepted for forward-compatible
	// callers but always returns "" today.
	return out, "", nil
}
