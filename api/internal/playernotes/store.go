package playernotes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/api-commons/dynamo"
)

const (
	tablePlayerNotes = "poker_player_notes"
	MaxNoteLength    = 500
)

var (
	ErrInvalidOpponent = errors.New("playernotes: invalid opponent")
	ErrInvalidTag      = errors.New("playernotes: invalid tag")
	ErrNoteTooLong     = errors.New("playernotes: note too long")
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
