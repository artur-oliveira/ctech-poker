// Package table drives one table's hand.Table from exactly one goroutine —
// the Actor's Run loop — so the engine's non-thread-safe Table is safe to
// reach from multiple concurrent WebSocket connections without a mutex.
package table

import (
	"context"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

var timeNowFunc = time.Now

// Actor is the single writer for one table's hand.Table. Every mutation goes
// through Dispatch → the cmds channel → Run's loop, so hand.Table (which has
// no internal locking) is only ever touched from this one goroutine.
type Actor struct {
	id        string
	table     *hand.Table
	store     *tablestore.Store
	broadcast func(viewerID string, snap hand.Snapshot)

	cmds chan Command

	handID  string
	seq     int
	seenIDs map[string]bool // action_id de-dup, reset every new hand
}

func New(id string, t *hand.Table, store *tablestore.Store, broadcast func(string, hand.Snapshot)) *Actor {
	return &Actor{
		id:        id,
		table:     t,
		store:     store,
		broadcast: broadcast,
		cmds:      make(chan Command, 64),
		seenIDs:   make(map[string]bool),
	}
}

// Dispatch enqueues cmd and blocks until Run has processed it.
func (a *Actor) Dispatch(cmd Command) error {
	a.cmds <- cmd
	return <-cmd.reply()
}

// Run processes commands serially until ctx is cancelled (lease lost, or
// clean shutdown) — the only place hand.Table is ever mutated.
func (a *Actor) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-a.cmds:
			err := a.handle(cmd)
			cmd.reply() <- err
		}
	}
}

func (a *Actor) handle(cmd Command) error {
	switch c := cmd.(type) {
	case ReadyCmd:
		return a.handleReady(c)
	case ActCmd:
		return a.handleAct(c)
	case DisconnectCmd:
		return a.handleDisconnect(c)
	case ReconnectCmd:
		return a.handleReconnect(c)
	default:
		return nil
	}
}

func (a *Actor) handleReady(c ReadyCmd) error {
	for _, p := range a.table.PlayersForActor() {
		if p.ID == c.PlayerID {
			p.Ready = c.Ready
		}
	}
	if a.table.Stage() == hand.WaitingForPlayers || a.table.Stage() == hand.Complete {
		if err := a.table.StartHand(); err == nil {
			a.handID = newHandID()
			a.seq = 0
			a.seenIDs = make(map[string]bool)
			a.persistSnapshot()
		}
		// A "need at least 2 ready players" error is not a caller error —
		// it just means the table keeps waiting; swallow it here.
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleAct(c ActCmd) error {
	if a.seenIDs[c.ActionID] {
		a.broadcastAll() // resend current state so a retried ack still gets one
		return nil
	}
	if err := a.table.Act(c.PlayerID, c.Action, c.Amount); err != nil {
		return err
	}
	a.seenIDs[c.ActionID] = true
	a.seq++
	if a.store != nil {
		_ = a.store.AppendAction(context.Background(), a.id, a.handID, a.seq, tablestore.ActionLogEntry{
			TableID: a.id, HandID: a.handID, Seq: a.seq,
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: string(c.Action), Amount: c.Amount,
		})
	}
	if a.table.Stage() == hand.Complete {
		a.persistSnapshot()
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleDisconnect(c DisconnectCmd) error { return nil }
func (a *Actor) handleReconnect(c ReconnectCmd) error   { return nil }

func (a *Actor) persistSnapshot() {
	if a.store == nil {
		return
	}
	_ = a.store.SaveSnapshot(context.Background(), a.id, a.handID, a.seq, a.table.ViewFor(""))
}

func (a *Actor) broadcastAll() {
	if a.broadcast == nil {
		return
	}
	for _, p := range a.table.PlayersForActor() {
		a.broadcast(p.ID, a.table.ViewFor(p.ID))
	}
}

func newHandID() string {
	return timeNowFunc().Format("20060102T150405.000000000")
}

// TableForTest exposes the underlying hand.Table for integration-test
// assertions — Actor's whole purpose is to be the only mutator, so this is
// deliberately read-oriented (callers should use ViewFor, never mutate the
// returned *hand.Table directly).
func (a *Actor) TableForTest() *hand.Table { return a.table }
