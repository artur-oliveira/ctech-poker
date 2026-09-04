// Package tablenotify closes the one real gap the 2026-09-04 incident
// exposed in the multi-process fleet (docs/specs/2026-09-04-cross-instance-stale-turn-timer.md
// and its follow-up spec): the per-viewer state push (app.go's broadcast
// closure, over api-commons/ws.Registry) already fans out across every
// process instantly, so what a player SEES has always been correct. What was
// missing is that each process's own in-memory *table.Actor — the one
// actually enforcing THAT process's connections' real turn/time-bank
// timers via time.AfterFunc — has no way to learn a sibling process just
// committed, short of its own next unrelated reload trigger (a ping-paced
// ReconnectCmd, up to tableconn.SyncInterval behind, or one of its own
// players acting). That gap is where a resumed deadline could already have
// decayed by a meaningful amount before this process ever re-armed against
// it.
//
// This package publishes one lightweight "table X changed" signal per commit
// over a single shared Valkey Pub/Sub channel — every table shares it, rather
// than one channel per table, so subscribing is a one-time cost per process
// instead of scaling with how many tables are live. tablemanager.Manager
// subscribes once and, for each message, dispatches table.ExternalChangeCmd
// to whichever local Actor is running that table (if any) so it reloads and
// re-arms immediately.
//
// Fire-and-forget throughout: DynamoDB's conditional commit is always the
// source of truth. A dropped or delayed message here only costs the slower
// existing reload path for whichever process missed it — never correctness.
package tablenotify

import (
	"context"
	"log/slog"
	"time"

	"github.com/valkey-io/valkey-go"
)

// channel is shared by every table on purpose — see package doc.
const channel = "poker:tablechanged"

// publishTimeout bounds Notify's round trip so an unreachable Valkey cannot
// add latency to the caller (Actor.commit runs on every player action).
const publishTimeout = 2 * time.Second

// resubscribeBackoff paces Listen's retry after a dropped receive loop, so a
// genuinely down Valkey does not spin this goroutine.
const resubscribeBackoff = time.Second

// Service publishes and subscribes table-changed signals over one shared
// Valkey client. A nil *Service (dev/tests without a cache) makes both
// Notify and Listen no-ops, matching tableconn.Service/tablestreak's
// nil-degrades-gracefully convention.
type Service struct{ client valkey.Client }

func NewService(client valkey.Client) *Service { return &Service{client: client} }

// Notify announces that tableID's persisted state just changed. See the
// package doc for why this never returns an error: the caller (Actor.commit)
// has nothing useful to do with one, and must never block or fail a real
// commit over a signal that only ever speeds up a sibling process along.
func (s *Service) Notify(ctx context.Context, tableID string) {
	if s == nil || s.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if err := s.client.Do(ctx, s.client.B().Publish().Channel(channel).Message(tableID).Build()).Error(); err != nil {
		slog.Warn("table change publish failed", "table_id", tableID, "err", err)
	}
}

// Listen blocks, invoking onChange with each published table ID, until ctx
// is cancelled. A receive error (a dropped connection, a Valkey restart)
// logs and retries after resubscribeBackoff rather than giving up — every
// process's own slower reload path already covers however long a
// reconnect takes.
func (s *Service) Listen(ctx context.Context, onChange func(tableID string)) {
	if s == nil || s.client == nil {
		return
	}
	for ctx.Err() == nil {
		err := s.client.Receive(ctx, s.client.B().Subscribe().Channel(channel).Build(), func(msg valkey.PubSubMessage) {
			onChange(msg.Message)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Warn("table change subscribe interrupted", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(resubscribeBackoff):
		}
	}
}
