package playernotes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tablePlayerNotes = "poker_player_notes"
	MaxNoteLength    = 500

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
	ErrNoteTooLong     = errors.New("playernotes: note too long")
	// ErrTooManyOpponents keeps one request's fan-out bounded by the screen
	// asking, not by whatever id list a client feels like sending.
	ErrTooManyOpponents = errors.New("playernotes: too many opponents")
)

type Note struct {
	ViewerID   string `dynamodbav:"pk" json:"-"`
	OpponentID string `dynamodbav:"sk" json:"opponent_id"`
	Tag        string `dynamodbav:"tag,omitempty" json:"tag,omitempty"`
	Text       string `dynamodbav:"note,omitempty" json:"note,omitempty"`
	UpdatedAt  string `dynamodbav:"updated_at" json:"updated_at"`
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePlayerNotes)}
}

func Normalize(viewerID, opponentID, tag, text string) (Note, error) {
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
	return Note{ViewerID: viewerID, OpponentID: opponentID, Tag: tag, Text: text}, nil
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
// An empty note and tag means removal, keeping empty rows out of DynamoDB.
func (s *Store) Save(ctx context.Context, viewerID, opponentID, tag, text string) (*Note, error) {
	note, err := Normalize(viewerID, opponentID, tag, text)
	if err != nil {
		return nil, err
	}
	if note.Tag == "" && note.Text == "" {
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
