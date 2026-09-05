// Package handmeta is the single "player metadata about one hand" resource
// #349 and #347 were asked to share instead of shipping two divergent
// designs: a short note per street, a "mark for review" flag and a list of
// named collections — all on one row per (player, hand) — plus a second,
// small per-player resource (saved /hands filters) that piggybacks on the
// same table under a fixed sort key rather than justifying a table of its
// own. Modeled directly on internal/playernotes: DynamoDB pk/sk,
// Normalize/validation, delete-on-empty Save.
package handmeta

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tableHandMeta = "poker_hand_meta"

	// filtersSK is the fixed sort key for a player's one saved-filters row.
	// Reusing this table for that non-per-hand resource — rather than a
	// second tiny table — is the simplest reading of "one endpoint of player
	// metadata about hands" the sibling issues asked to coordinate on.
	filtersSK    = "filters"
	handSKPrefix = "hand#"

	MaxStreetNoteLength     = 300
	MaxCollectionsPerHand   = 20
	MaxCollectionNameLength = 40
	MaxSavedFilters         = 20
	MaxFilterNameLength     = 40
)

// validStreets bounds street_notes keys to the segments ActionTimeline
// actually renders (see ui/src/components/hands/ActionTimeline.tsx's
// STREET_LABELS) — an arbitrary key would just be dead weight nothing
// displays.
var validStreets = map[string]bool{
	"preflop": true, "flop": true, "turn": true, "river": true, "showdown": true,
}

var (
	ErrInvalidPlayer        = errors.New("handmeta: invalid player")
	ErrInvalidHand          = errors.New("handmeta: invalid hand")
	ErrInvalidStreet        = errors.New("handmeta: invalid street")
	ErrNoteTooLong          = errors.New("handmeta: street note too long")
	ErrTooManyCollections   = errors.New("handmeta: too many collections")
	ErrCollectionNameInvalid = errors.New("handmeta: collection name invalid")
	ErrTooManySavedFilters  = errors.New("handmeta: too many saved filters")
	ErrFilterNameInvalid    = errors.New("handmeta: filter name invalid")
)

// Meta is one player's private annotation of one hand. #349's street notes
// and review marker and #347's collections are the same record — a hand
// gets filed away for one reason, not two independent ones.
type Meta struct {
	PlayerID     string            `dynamodbav:"pk" json:"-"`
	SK           string            `dynamodbav:"sk" json:"-"`
	HandID       string            `dynamodbav:"-" json:"hand_id"`
	StreetNotes  map[string]string `dynamodbav:"street_notes,omitempty" json:"street_notes,omitempty"`
	ReviewMarked bool              `dynamodbav:"review_marked,omitempty" json:"review_marked"`
	Collections  []string          `dynamodbav:"collections,omitempty" json:"collections,omitempty"`
	UpdatedAt    string            `dynamodbav:"updated_at" json:"updated_at"`
}

// SavedFilter is one player-named /hands filter combination — same shape as
// the client's HandsFilter (ui/src/lib/handsHistory.ts). This package does
// not interpret it, only persists it.
type SavedFilter struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	TableID string `json:"table_id"`
}

type savedFiltersRow struct {
	PlayerID  string        `dynamodbav:"pk"`
	SK        string        `dynamodbav:"sk"`
	Filters   []SavedFilter `dynamodbav:"filters,omitempty"`
	UpdatedAt string        `dynamodbav:"updated_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableHandMeta)}
}

func handSK(handID string) string { return handSKPrefix + handID }

// NormalizeMeta validates and trims one player's annotation of one hand. An
// empty street note is dropped rather than stored — empty text means no
// annotation, never a placeholder row (#349's UX requirement).
func NormalizeMeta(
	playerID, handID string, streetNotes map[string]string, reviewMarked bool, collections []string,
) (Meta, error) {
	playerID = strings.TrimSpace(playerID)
	handID = strings.TrimSpace(handID)
	if playerID == "" {
		return Meta{}, ErrInvalidPlayer
	}
	if handID == "" {
		return Meta{}, ErrInvalidHand
	}
	notes := make(map[string]string, len(streetNotes))
	for street, text := range streetNotes {
		street = strings.ToLower(strings.TrimSpace(street))
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if !validStreets[street] {
			return Meta{}, ErrInvalidStreet
		}
		if utf8.RuneCountInString(text) > MaxStreetNoteLength {
			return Meta{}, ErrNoteTooLong
		}
		notes[street] = text
	}
	cols, err := normalizeCollections(collections)
	if err != nil {
		return Meta{}, err
	}
	return Meta{
		PlayerID: playerID, SK: handSK(handID), HandID: handID,
		StreetNotes: notes, ReviewMarked: reviewMarked, Collections: cols,
	}, nil
}

func normalizeCollections(collections []string) ([]string, error) {
	if len(collections) > MaxCollectionsPerHand {
		return nil, ErrTooManyCollections
	}
	seen := make(map[string]bool, len(collections))
	out := make([]string, 0, len(collections))
	for _, name := range collections {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if utf8.RuneCountInString(name) > MaxCollectionNameLength {
			return nil, ErrCollectionNameInvalid
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// Get reads one player's annotation of one hand. A nil result with no error
// means no annotation was ever saved — not a not-found error.
func (s *Store) Get(ctx context.Context, playerID, handID string) (*Meta, error) {
	playerID = strings.TrimSpace(playerID)
	handID = strings.TrimSpace(handID)
	if playerID == "" {
		return nil, ErrInvalidPlayer
	}
	if handID == "" {
		return nil, ErrInvalidHand
	}
	item, err := s.base.GetItem(ctx, playerID, handSK(handID))
	if err != nil {
		return nil, fmt.Errorf("handmeta: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	meta, err := dynamo.Decode[Meta](item)
	if err != nil {
		return nil, fmt.Errorf("handmeta: decode: %w", err)
	}
	meta.HandID = handID
	return meta, nil
}

// Save atomically replaces the caller's whole annotation of one hand. No
// street notes, no review mark and no collections deletes the row — the same
// delete-on-empty convention playernotes.Save uses to keep empty rows out of
// DynamoDB.
func (s *Store) Save(
	ctx context.Context, playerID, handID string, streetNotes map[string]string, reviewMarked bool, collections []string,
) (*Meta, error) {
	meta, err := NormalizeMeta(playerID, handID, streetNotes, reviewMarked, collections)
	if err != nil {
		return nil, err
	}
	if len(meta.StreetNotes) == 0 && !meta.ReviewMarked && len(meta.Collections) == 0 {
		if _, err := s.base.DeleteItem(ctx, meta.PlayerID, meta.SK); err != nil {
			return nil, fmt.Errorf("handmeta: delete: %w", err)
		}
		return nil, nil
	}
	meta.UpdatedAt = dynamo.NowStr()
	item, err := dynamo.Encode(meta)
	if err != nil {
		return nil, fmt.Errorf("handmeta: encode: %w", err)
	}
	if err := s.base.PutItem(ctx, item); err != nil {
		return nil, fmt.Errorf("handmeta: save: %w", err)
	}
	return &meta, nil
}

// ListMarked answers /hands' "Coleções" tab: every hand the player marked
// (review flag or filed into any collection), in one bounded Query — the
// same whole-namespace-List shape playernotes.List uses for one viewer's own
// partition.
func (s *Store) ListMarked(ctx context.Context, playerID string) ([]Meta, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrInvalidPlayer
	}
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, Limit: 500})
	if err != nil {
		return nil, fmt.Errorf("handmeta: list: %w", err)
	}
	metas := make([]Meta, 0, len(result.Items))
	for _, item := range result.Items {
		meta, err := dynamo.Decode[Meta](item)
		if err != nil {
			return nil, fmt.Errorf("handmeta: decode: %w", err)
		}
		if meta.SK == filtersSK {
			continue // the saved-filters row shares this partition; skip it here
		}
		meta.HandID = strings.TrimPrefix(meta.SK, handSKPrefix)
		metas = append(metas, *meta)
	}
	return metas, nil
}

// ListSavedFilters reads the caller's saved /hands filters. Empty (never
// saved any) is not an error.
func (s *Store) ListSavedFilters(ctx context.Context, playerID string) ([]SavedFilter, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrInvalidPlayer
	}
	item, err := s.base.GetItem(ctx, playerID, filtersSK)
	if err != nil {
		return nil, fmt.Errorf("handmeta: list saved filters: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	row, err := dynamo.Decode[savedFiltersRow](item)
	if err != nil {
		return nil, fmt.Errorf("handmeta: decode saved filters: %w", err)
	}
	return row.Filters, nil
}

// SaveFilters replaces the caller's whole saved-filter list in one write —
// add/rename/remove all resolve to a single PUT of the full set, the same
// "one small row is the whole resource" shape used elsewhere for a
// per-player value (e.g. sessionlog's session row). An empty list deletes
// the row.
func (s *Store) SaveFilters(ctx context.Context, playerID string, filters []SavedFilter) ([]SavedFilter, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrInvalidPlayer
	}
	if len(filters) > MaxSavedFilters {
		return nil, ErrTooManySavedFilters
	}
	normalized := make([]SavedFilter, 0, len(filters))
	for _, f := range filters {
		name := strings.TrimSpace(f.Name)
		if name == "" || utf8.RuneCountInString(name) > MaxFilterNameLength {
			return nil, ErrFilterNameInvalid
		}
		normalized = append(normalized, SavedFilter{
			Name: name, Outcome: strings.TrimSpace(f.Outcome), TableID: strings.TrimSpace(f.TableID),
		})
	}
	if len(normalized) == 0 {
		if _, err := s.base.DeleteItem(ctx, playerID, filtersSK); err != nil {
			return nil, fmt.Errorf("handmeta: delete saved filters: %w", err)
		}
		return nil, nil
	}
	row := savedFiltersRow{PlayerID: playerID, SK: filtersSK, Filters: normalized, UpdatedAt: dynamo.NowStr()}
	item, err := dynamo.Encode(row)
	if err != nil {
		return nil, fmt.Errorf("handmeta: encode saved filters: %w", err)
	}
	if err := s.base.PutItem(ctx, item); err != nil {
		return nil, fmt.Errorf("handmeta: save saved filters: %w", err)
	}
	return normalized, nil
}
