// Package recentplayers defines the bounded, per-viewer opponent history.
package recentplayers

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Player is one opponent's entry in a viewer's recent-players list. Since
// #199 it is derived at read time by coalescing the viewer's own per-hand
// rows (see DynamoStore.List), not stored as an item of its own — hence no
// dynamodbav tags: nothing decodes into this shape any more.
type Player struct {
	ViewerPlayerID   string
	OpponentPlayerID string
	LastPlayedAt     int64
	LastTableID      string
	LastHandID       string
	HandsTogether    int64
}

type HandCompletion struct {
	TableID  string
	HandID   string
	Players  []string
	PlayedAt time.Time
}

type Page struct {
	Players []Player
	LastKey map[string]types.AttributeValue
}
