// Package recentplayers defines the bounded, per-viewer opponent history.
package recentplayers

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Player struct {
	ViewerPlayerID   string `dynamodbav:"pk"`
	OpponentPlayerID string `dynamodbav:"sk"`
	LastPlayedAt     int64  `dynamodbav:"last_played_at"`
	LastTableID      string `dynamodbav:"last_table_id"`
	LastHandID       string `dynamodbav:"last_hand_id"`
	HandsTogether    int64  `dynamodbav:"hands_together"`
	RecentPartition  string `dynamodbav:"gsi_recent_pk"`
	RecentSort       string `dynamodbav:"gsi_recent_sk"`
	TTL              int64  `dynamodbav:"ttl"`
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
