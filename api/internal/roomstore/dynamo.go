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
	gsiBucket    = "gsi_bucket"
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
		GSIBucket    string `dynamodbav:"gsi_bucket,omitempty"`
		GSIShareCode string `dynamodbav:"gsi_share_code,omitempty"`
	}{
		PK: r.ID, SK: roomSK, Room: r,
		GSIPublic:    publicIndexValue(r),
		GSIBucket:    bucketIndexValue(r),
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

// BucketKey is one lobby bucket's partition in gsi_bucket: every public room
// that a (currency mode, blinds, seats) pick could seat a player in, and
// nothing else. It is immutable for the life of a room — none of its parts
// ever change after creation — so a seat count write-through never has to
// touch the index entry.
func BucketKey(currencyMode string, smallBlind, bigBlind int64, maxSeats int) string {
	if currencyMode == "" {
		// Predates the field: sandbox by construction, same rule the lobby
		// aggregate applies.
		currencyMode = CurrencyModeSandbox
	}
	return fmt.Sprintf("public#%s#%d#%d#%d", currencyMode, smallBlind, bigBlind, maxSeats)
}

// bucketIndexValue keeps gsi_bucket sparse the same way publicIndexValue keeps
// gsi_public sparse: private rooms carry no value and so can never be handed
// out by a bucket query.
func bucketIndexValue(r Room) string {
	if r.Visibility != "public" {
		return ""
	}
	return BucketKey(r.CurrencyMode, r.SmallBlind, r.BigBlind, r.MaxSeats)
}

// bucketCandidateLimit bounds one join attempt's read to a single Query page.
// A bucket holding more open tables than this has far more capacity than one
// player needs, and seatInBucket only ever walks candidates until one seats
// them — so reading more would just bill for rooms nobody reaches.
const bucketCandidateLimit = 50

// ListBucket returns one bucket's public rooms — a single Query against
// gsi_bucket, so the cost of a join attempt is a function of that bucket, not
// of how many public rooms exist globally (#213). Rooms created before
// gsi_bucket existed carry no index value and are invisible here; public
// tables are ephemeral (cmd/tablecleanup deletes them), so those age out on
// their own rather than needing a backfill.
func (s *Store) ListBucket(ctx context.Context, currencyMode string, smallBlind, bigBlind int64, maxSeats int) ([]Room, error) {
	key := BucketKey(currencyMode, smallBlind, bigBlind, maxSeats)
	result, err := s.base.QueryGSI(ctx, gsiBucket, gsiBucket, key, bucketCandidateLimit, nil)
	if err != nil {
		return nil, fmt.Errorf("roomstore: list bucket: %w", err)
	}
	return decodeRooms(result.Items)
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

// listAllPublicMaxPages bounds ListAllPublic so a pathological index can
// never turn one lobby request into an unbounded scan. At 100 rooms a page
// that is 2 000 public rooms — far past anything the lobby is sized for.
const listAllPublicMaxPages = 20

// ListAllPublic walks every page of gsi_public, unlike ListPublic which
// returns one. The lobby's bucket aggregate has to be correct beyond the
// first page (#76) — a per-page count silently under-reports availability.
func (s *Store) ListAllPublic(ctx context.Context) ([]Room, error) {
	var (
		out      []Room
		startKey map[string]types.AttributeValue
	)
	for page := 0; page < listAllPublicMaxPages; page++ {
		rooms, lastKey, err := s.ListPublic(ctx, 100, startKey)
		if err != nil {
			return nil, err
		}
		out = append(out, rooms...)
		if len(lastKey) == 0 {
			break
		}
		startKey = lastKey
	}
	return out, nil
}

func (s *Store) ListPublic(ctx context.Context, limit int, startKey map[string]types.AttributeValue) ([]Room, map[string]types.AttributeValue, error) {
	result, err := s.base.QueryGSI(ctx, gsiPublic, "gsi_public", "public", limit, startKey)
	if err != nil {
		return nil, nil, fmt.Errorf("roomstore: list public: %w", err)
	}
	out, err := decodeRooms(result.Items)
	if err != nil {
		return nil, nil, err
	}
	return out, result.LastEvaluatedKey, nil
}

func decodeRooms(items []map[string]types.AttributeValue) ([]Room, error) {
	out := make([]Room, 0, len(items))
	for _, item := range items {
		r, err := dynamo.Decode[Room](item)
		if err != nil {
			return nil, fmt.Errorf("roomstore: decode: %w", err)
		}
		out = append(out, *r)
	}
	return out, nil
}
