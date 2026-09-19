// Package chatprefs persists each player's personal chat-filter addition
// (#327): extra words masked only for that player, on top of the table-wide
// floor internal/chatfilter's global Filter already enforces. It never
// stores or expresses a way to remove a floor word — Save below only ever
// replaces the caller's own additive word list.
package chatprefs

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
	tableChatPrefs = "poker_chat_prefs"

	// MaxExtraWords / MaxWordLength bound one player's addition to a small,
	// boring DynamoDB item — mirrors playernotes' MaxLabels/MaxLabelLength,
	// same reasoning: not a measured product limit, just a sane ceiling.
	MaxExtraWords = 50
	MaxWordLength = 32
)

var (
	ErrInvalidPlayer = errors.New("chatprefs: invalid player")
	ErrTooManyWords  = errors.New("chatprefs: too many extra words")
	ErrWordTooLong   = errors.New("chatprefs: word too long")
)

// Preferences is one player's personal chat-filter addition.
type Preferences struct {
	PlayerID   string   `dynamodbav:"pk" json:"-"`
	ExtraWords []string `dynamodbav:"extra_words,omitempty" json:"extra_words,omitempty"`
	UpdatedAt  string   `dynamodbav:"updated_at" json:"updated_at"`
}

// NormalizeExtraWords trims, lowercases and dedupes, same normalization
// chatfilter.New itself applies, so a stored word compares byte-for-byte
// against the one Effective will later mask. Bounded so one player's list
// can't grow into every viewer-facing filter build being O(n) in an
// unbounded n.
func NormalizeExtraWords(words []string) ([]string, error) {
	if len(words) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(words))
	seen := make(map[string]bool, len(words))
	for _, word := range words {
		word = strings.ToLower(strings.TrimSpace(word))
		if word == "" || seen[word] {
			continue
		}
		if utf8.RuneCountInString(word) > MaxWordLength {
			return nil, ErrWordTooLong
		}
		seen[word] = true
		out = append(out, word)
	}
	if len(out) > MaxExtraWords {
		return nil, ErrTooManyWords
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tableChatPrefs)}
}

// Get answers playerID's stored preferences, or nil if they never set any
// (equivalent to "floor only, no personal additions").
func (s *Store) Get(ctx context.Context, playerID string) (*Preferences, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrInvalidPlayer
	}
	item, err := s.base.GetItem(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("chatprefs: get: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	prefs, err := dynamo.Decode[Preferences](item)
	if err != nil {
		return nil, fmt.Errorf("chatprefs: decode: %w", err)
	}
	return prefs, nil
}

// Save atomically replaces the caller's own extra-word list. An empty list
// clears it (goes back to floor-only), keeping empty rows out of DynamoDB —
// same shape as playernotes.Store.Save.
func (s *Store) Save(ctx context.Context, playerID string, extraWords []string) (*Preferences, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrInvalidPlayer
	}
	normalized, err := NormalizeExtraWords(extraWords)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		if _, err := s.base.DeleteItem(ctx, playerID); err != nil {
			return nil, fmt.Errorf("chatprefs: delete: %w", err)
		}
		return nil, nil
	}
	prefs := Preferences{PlayerID: playerID, ExtraWords: normalized, UpdatedAt: dynamo.NowStr()}
	item, err := dynamo.Encode(prefs)
	if err != nil {
		return nil, fmt.Errorf("chatprefs: encode: %w", err)
	}
	if err := s.base.PutItem(ctx, item); err != nil {
		return nil, fmt.Errorf("chatprefs: save: %w", err)
	}
	return &prefs, nil
}
