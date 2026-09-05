// Package tablehandoff lets one instance ask whichever instance owns a set
// of WebSocket connIDs to close them deliberately, for the explicit
// session-handoff feature (#353): "continue here, disconnect the other
// device."
//
// It is internal/tablenotify's shape (one shared Valkey Pub/Sub channel,
// subscribed once per process) carrying a payload instead of a bare signal:
// tablenotify only ever needs to say "table X changed, go reload it";
// this needs to say "close exactly these connIDs," which only the instance
// that actually holds one of them can act on (see internal/wsdrain.
// CloseByConnID, which silently ignores any connID it doesn't recognize).
//
// Fire-and-forget throughout, same reasoning as tablenotify: nothing here
// decides table state. A dropped or delayed message only means the old
// device's socket stays open a bit longer than the player asked for — never
// a correctness issue. See docs/specs/2026-09-05-session-handoff-tableconn.md.
package tablehandoff

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/valkey-io/valkey-go"
)

// channel is shared by every table on purpose, same reasoning as
// tablenotify: subscribing is a one-time cost per process.
const channel = "poker:tablehandoff"

// publishTimeout bounds RequestClose's round trip so an unreachable Valkey
// cannot add latency to the caller (Actor.handleRequestHandoff, on the
// player's own interactive command budget).
const publishTimeout = 2 * time.Second

// resubscribeBackoff paces Listen's retry after a dropped receive loop.
const resubscribeBackoff = time.Second

// message is the wire payload published on channel.
type message struct {
	TableID string   `json:"table_id"`
	ConnIDs []string `json:"conn_ids"`
}

// Service publishes and subscribes handoff-close requests over one shared
// Valkey client. A nil *Service (dev/tests without a cache) makes both
// RequestClose and Listen no-ops, matching tablenotify.Service's convention.
type Service struct{ client valkey.Client }

func NewService(client valkey.Client) *Service { return &Service{client: client} }

// RequestClose announces that connIDs (all belonging to one player at
// tableID) should be closed. See table.HandoffCloser, which this satisfies.
func (s *Service) RequestClose(ctx context.Context, tableID string, connIDs []string) {
	if s == nil || s.client == nil || len(connIDs) == 0 {
		return
	}
	payload, err := json.Marshal(message{TableID: tableID, ConnIDs: connIDs})
	if err != nil {
		slog.Warn("handoff close payload encode failed", "table_id", tableID, "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if err := s.client.Do(ctx, s.client.B().Publish().Channel(channel).Message(string(payload)).Build()).Error(); err != nil {
		slog.Warn("handoff close publish failed", "table_id", tableID, "err", err)
	}
}

// Listen blocks, invoking onClose with each published set of connIDs, until
// ctx is cancelled. onClose is expected to call wsdrain.CloseByConnID (or an
// equivalent), which is itself a no-op for any connID this process doesn't
// hold — so every process can safely run the same onClose unconditionally.
func (s *Service) Listen(ctx context.Context, onClose func(connIDs []string)) {
	if s == nil || s.client == nil {
		return
	}
	for ctx.Err() == nil {
		err := s.client.Receive(ctx, s.client.B().Subscribe().Channel(channel).Build(), func(msg valkey.PubSubMessage) {
			var m message
			if err := json.Unmarshal([]byte(msg.Message), &m); err != nil {
				slog.Warn("handoff close payload decode failed", "err", err)
				return
			}
			onClose(m.ConnIDs)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Warn("handoff close subscribe interrupted", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(resubscribeBackoff):
		}
	}
}
