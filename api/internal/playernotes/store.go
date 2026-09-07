package playernotes

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tablePlayerNotes = "poker_player_notes"
	MaxNoteLength    = 500

	// MaxLabels / MaxLabelLength bound the searchable free-text labels a
	// single note may carry (#345). They exist so one note stays a small
	// DynamoDB item and so the in-memory search below stays linear in a
	// bounded amount of text — not because the product needs exactly these
	// numbers.
	MaxLabels      = 10
	MaxLabelLength = 24

	// MaxBatchOpponentIDs bounds one GetMany call. A table seats at most 9 and
	// a hand has at most 8 opponents, so this is already far above any real
	// screen; it matches the social relationship batch the same screens call
	// with the same id list, and stays well under DynamoDB's 100-key cap for
	// a single BatchGetItem request entry.
	MaxBatchOpponentIDs = 25
)

var (
	ErrInvalidOpponent = errors.New("playernotes: invalid opponent")
	ErrInvalidTag      = errors.New("playernotes: invalid tag")
	ErrInvalidLabel    = errors.New("playernotes: invalid label")
	ErrNoteTooLong     = errors.New("playernotes: note too long")
	// ErrTooManyOpponents keeps one request's fan-out bounded by the screen
	// asking, not by whatever id list a client feels like sending.
	ErrTooManyOpponents = errors.New("playernotes: too many opponents")
)

type Note struct {
	ViewerID   string `dynamodbav:"pk" json:"-"`
	OpponentID string `dynamodbav:"sk" json:"opponent_id"`
	Tag        string `dynamodbav:"tag,omitempty" json:"tag,omitempty"`
	// Labels are the searchable free-text tags (#345). Tag above stays what
	// it always was — one colour from a fixed enum, used as a visual
	// highlight — so an existing client keeps working unchanged; Labels is
	// additive and normalized the same way (trimmed, lowercased, deduped).
	Labels    []string `dynamodbav:"labels,omitempty" json:"labels,omitempty"`
	Text      string   `dynamodbav:"note,omitempty" json:"note,omitempty"`
	UpdatedAt string   `dynamodbav:"updated_at" json:"updated_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePlayerNotes)}
}

func Normalize(viewerID, opponentID, tag, text string, labels []string) (Note, error) {
	viewerID = strings.TrimSpace(viewerID)
	opponentID = strings.TrimSpace(opponentID)
	tag = strings.ToLower(strings.TrimSpace(tag))
	text = strings.TrimSpace(text)
	if viewerID == "" || opponentID == "" || viewerID == opponentID {
		return Note{}, ErrInvalidOpponent
	}
	if tag != "" && !validTag(tag) {
		return Note{}, ErrInvalidTag
	}
	if utf8.RuneCountInString(text) > MaxNoteLength {
		return Note{}, ErrNoteTooLong
	}
	normalizedLabels, err := normalizeLabels(labels)
	if err != nil {
		return Note{}, err
	}
	return Note{ViewerID: viewerID, OpponentID: opponentID, Tag: tag, Text: text, Labels: normalizedLabels}, nil
}

// normalizeLabels trims, lowercases and dedupes while preserving the caller's
// order, so a label is one canonical string and Matches can compare it
// exactly. An empty entry is dropped rather than rejected — a trailing comma
// in a client's input is not an error worth failing a save over.
func normalizeLabels(labels []string) ([]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(labels))
	seen := make(map[string]bool, len(labels))
	for _, label := range labels {
		label = strings.ToLower(strings.TrimSpace(label))
		if label == "" || seen[label] {
			continue
		}
		if utf8.RuneCountInString(label) > MaxLabelLength {
			return nil, ErrInvalidLabel
		}
		seen[label] = true
		normalized = append(normalized, label)
	}
	if len(normalized) > MaxLabels {
		return nil, ErrInvalidLabel
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

// Matches reports whether the note satisfies a label filter and/or a free-text
// query. Both are matched in memory over the caller's own bounded note set —
// search is always within one viewer's notes, never across players, so a GSI
// would buy nothing that a Query the caller already makes does not (#345).
func (n Note) Matches(label, query string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	query = strings.ToLower(strings.TrimSpace(query))
	if label != "" && !slices.Contains(n.Labels, label) {
		return false
	}
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(n.Text), query) {
		return true
	}
	for _, own := range n.Labels {
		if strings.Contains(own, query) {
			return true
		}
	}
	return false
}

// Filter keeps the notes matching label/query, in order. An empty filter
// returns notes untouched.
func Filter(notes []Note, label, query string) []Note {
	if strings.TrimSpace(label) == "" && strings.TrimSpace(query) == "" {
		return notes
	}
	filtered := make([]Note, 0, len(notes))
	for _, note := range notes {
		if note.Matches(label, query) {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

func validTag(tag string) bool {
	switch tag {
	case "red", "orange", "yellow", "green", "blue", "purple":
		return true
	default:
		return false
	}
}

func (s *Store) List(ctx context.Context, viewerID string) ([]Note, error) {
	if strings.TrimSpace(viewerID) == "" {
		return nil, ErrInvalidOpponent
	}
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: viewerID, Limit: 500})
	if err != nil {
		return nil, fmt.Errorf("playernotes: list: %w", err)
	}
	notes := make([]Note, 0, len(result.Items))
	for _, item := range result.Items {
		note, err := dynamo.Decode[Note](item)
		if err != nil {
			return nil, fmt.Errorf("playernotes: decode: %w", err)
		}
		notes = append(notes, *note)
	}
	return notes, nil
}

// GetMany reads only the notes for the opponents actually on screen — the
// seats at a table, or the players in one hand — in a single BatchGetItem.
// The full List below reads up to 500 notes to answer the same question,
// which is why neither the table nor the hand detail uses it any more (#209).
//
// Opponents with no note are simply absent from the result: the caller keys
// by opponent_id, so a partial answer can never attach one player's note to
// another.
func (s *Store) GetMany(ctx context.Context, viewerID string, opponentIDs []string) ([]Note, error) {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" {
		return nil, ErrInvalidOpponent
	}
	if len(opponentIDs) > MaxBatchOpponentIDs {
		return nil, ErrTooManyOpponents
	}
	keys := make([]map[string]types.AttributeValue, 0, len(opponentIDs))
	seen := make(map[string]bool, len(opponentIDs))
	for _, opponentID := range opponentIDs {
		opponentID = strings.TrimSpace(opponentID)
		if opponentID == "" || opponentID == viewerID || seen[opponentID] {
			continue
		}
		seen[opponentID] = true
		keys = append(keys, map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: viewerID},
			"sk": &types.AttributeValueMemberS{Value: opponentID},
		})
	}
	notes := make([]Note, 0, len(keys))
	if len(keys) == 0 {
		return notes, nil
	}
	request := map[string]types.KeysAndAttributes{s.base.TableName: {Keys: keys}}
	for attempt := 0; attempt < 4 && len(request) > 0; attempt++ {
		out, err := s.base.BatchGetItemRaw(ctx, &dynamodb.BatchGetItemInput{RequestItems: request})
		if err != nil {
			return nil, fmt.Errorf("playernotes: batch get: %w", err)
		}
		for _, item := range out.Responses[s.base.TableName] {
			note, err := dynamo.Decode[Note](item)
			if err != nil {
				return nil, fmt.Errorf("playernotes: decode: %w", err)
			}
			notes = append(notes, *note)
		}
		request = out.UnprocessedKeys
	}
	if len(request) > 0 {
		return nil, fmt.Errorf("playernotes: batch get remained unprocessed")
	}
	return notes, nil
}

// Save atomically replaces the caller's private annotation for one opponent.
// An empty note, tag and label set means removal, keeping empty rows out of
// DynamoDB.
func (s *Store) Save(ctx context.Context, viewerID, opponentID, tag, text string, labels []string) (*Note, error) {
	note, err := Normalize(viewerID, opponentID, tag, text, labels)
	if err != nil {
		return nil, err
	}
	if note.Tag == "" && note.Text == "" && len(note.Labels) == 0 {
		if _, err := s.base.DeleteItem(ctx, note.ViewerID, note.OpponentID); err != nil {
			return nil, fmt.Errorf("playernotes: delete: %w", err)
		}
		return nil, nil
	}
	note.UpdatedAt = dynamo.NowStr()
	item, err := dynamo.Encode(note)
	if err != nil {
		return nil, fmt.Errorf("playernotes: encode: %w", err)
	}
	if err := s.base.PutItem(ctx, item); err != nil {
		return nil, fmt.Errorf("playernotes: save: %w", err)
	}
	return &note, nil
}
