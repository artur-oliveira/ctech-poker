package table

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/reactions"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

func (a *Actor) handlePostBigBlind(ctx context.Context, c PostBigBlindCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error {
		return a.mutate(func() error {
			if !a.isSeated(c.PlayerID) {
				return fmt.Errorf("table: player %s is not seated", c.PlayerID)
			}
			a.cached.MarkReadyToPost(c.PlayerID)
			return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "post_big_blind",
			})
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) handleChat(ctx context.Context, c ChatCmd) error {
	if c.ActionID == "" {
		return errors.New("table: action_id is required")
	}
	if c.Message == "" {
		return errors.New("table: chat message is required")
	}
	return a.commitActivity(ctx, true, func() error {
		return a.mutate(func() error {
			if !a.isSeated(c.PlayerID) {
				return fmt.Errorf("table: player %s is not seated", c.PlayerID)
			}
			a.markLastAction(c.PlayerID)
			now := timeNowFunc().UnixMilli()
			a.activity.Chat = append(a.activity.Chat, tablestore.ChatMessage{
				ID: c.ActionID, PlayerID: c.PlayerID, Message: c.Message, Timestamp: now,
			})
			if len(a.activity.Chat) > maxPersistedChatMessages {
				a.activity.Chat = append([]tablestore.ChatMessage(nil), a.activity.Chat[len(a.activity.Chat)-maxPersistedChatMessages:]...)
			}
			return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "chat", Message: c.Message,
			})
		})
	})
}

func (a *Actor) handleReaction(ctx context.Context, c ReactionCmd) error {
	if c.ActionID == "" {
		return errors.New("table: action_id is required")
	}
	if c.ReactionID == "" {
		return errors.New("table: reaction_id is required")
	}
	if !reactions.IsKnown(c.ReactionID) {
		return errors.New("table: unknown reaction_id")
	}
	if reactions.IsPremium(c.ReactionID) {
		if a.reactionOwnership == nil {
			return errors.New("table: reaction ownership check unavailable")
		}
		owned, err := a.reactionOwnership(ctx, c.PlayerID, c.ReactionID)
		if err != nil {
			return err
		}
		if !owned {
			return errors.New("table: reaction not owned")
		}
	}
	// Reactions already have a dedicated fan-out frame. Persist them so a
	// reconnect can restore the short-lived effect, but do not also broadcast
	// a full table snapshot for the same cosmetic action.
	return a.commitActivity(ctx, false, func() error {
		return a.mutate(func() error {
			if !a.isSeated(c.PlayerID) {
				return fmt.Errorf("table: player %s is not seated", c.PlayerID)
			}
			if c.TargetPlayerID != "" && (c.TargetPlayerID == c.PlayerID || !a.isSeated(c.TargetPlayerID)) {
				return errors.New("table: invalid reaction target")
			}
			var extra []types.TransactWriteItem
			if reactions.IsPremium(c.ReactionID) {
				if a.reactionMarkUsed == nil {
					return errors.New("table: reaction usage recorder unavailable")
				}
				// This conditional write commits atomically with the reaction and is
				// the serialization point against refunds: exactly one can win.
				usedIntent, err := a.reactionMarkUsed(ctx, c.PlayerID, c.ReactionID)
				if err != nil {
					return fmt.Errorf("table: build premium reaction usage: %w", err)
				}
				if usedIntent != nil {
					extra = append(extra, *usedIntent)
				}
			}
			a.markLastAction(c.PlayerID)
			now := timeNowFunc().UnixMilli()
			a.activity.Reactions = append(a.activity.Reactions, tablestore.Reaction{
				ID: c.ActionID, PlayerID: c.PlayerID, ReactionID: c.ReactionID,
				TargetPlayerID: c.TargetPlayerID, Timestamp: now, ExpiresAt: now + reactionLifetime.Milliseconds(),
			})
			if len(a.activity.Reactions) > maxPersistedReactions {
				a.activity.Reactions = append([]tablestore.Reaction(nil), a.activity.Reactions[len(a.activity.Reactions)-maxPersistedReactions:]...)
			}
			return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, ActionID: c.ActionID, Action: "reaction",
				ReactionID: c.ReactionID, TargetPlayerID: c.TargetPlayerID,
			}, extra...)
		})
	})
}

func (a *Actor) handlePreselect(ctx context.Context, c PreselectCmd) error {
	if c.ActionID == "" {
		return errors.New("table: action_id is required")
	}
	if c.Selection != "" && c.Selection != "check_fold" && c.Selection != "fold" &&
		c.Selection != "call" && c.Selection != "call_any" && c.Selection != "all_in" {
		return errors.New("table: invalid action preselection")
	}
	return a.commitActivity(ctx, true, func() error {
		return a.mutate(func() error {
			if !a.isSeated(c.PlayerID) {
				return fmt.Errorf("table: player %s is not seated", c.PlayerID)
			}
			stage := a.cached.ViewFor("").Stage
			if c.ExpectedHandID == "" || a.handID != c.ExpectedHandID {
				return errors.New("table: stale action state")
			}
			// New clients scope this harmless future intent to hand+street instead
			// of the whole table version. Activity, presence and another player's
			// action may legitimately advance version while the frame is in flight.
			// Keep exact-version validation only as a rolling-deploy fallback for
			// older clients that do not send expected_stage yet.
			if c.ExpectedStage != "" {
				if c.ExpectedStage != stage {
					return errors.New("table: stale action state")
				}
			} else if c.ExpectedSnapshotVersion == 0 || uint64(a.version) != c.ExpectedSnapshotVersion {
				return errors.New("table: stale action state")
			}
			if c.Selection == "call" && (c.Amount <= 0 || c.Amount != a.cached.ProspectiveCallAmountForActor(c.PlayerID)) {
				return errors.New("table: fixed call amount changed")
			}
			if c.Selection == "check_fold" {
				c.Amount = a.cached.ProspectiveCallAmountForActor(c.PlayerID)
			} else if c.Selection != "call" {
				c.Amount = 0
			}
			if a.activity.Preselections == nil {
				a.activity.Preselections = make(map[string]tablestore.Preselection)
			}
			if c.Selection == "" {
				delete(a.activity.Preselections, c.PlayerID)
			} else {
				a.activity.Preselections[c.PlayerID] = tablestore.Preselection{
					Selection: c.Selection, Amount: c.Amount, HandID: a.handID, Stage: stage,
				}
			}
			a.markLastAction(c.PlayerID)
			action := "preselect_action"
			if c.Selection == "" {
				action = "clear_preselection"
			}
			return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
				PlayerID: c.PlayerID, ActionID: c.ActionID, Action: action, Selection: c.Selection, Amount: c.Amount,
			})
		})
	})
}

// prunePreselections enforces the lifetime promised on the wire: a prepared
// action belongs to exactly one hand and one betting street. Keeping stale
// entries hidden only at snapshot time is insufficient because the inline
// executor would otherwise find and execute them when that player becomes
// current on a later street or hand.
func (a *Actor) prunePreselections() {
	if a.cached == nil || a.activity.Preselections == nil {
		return
	}
	stage := a.cached.ViewFor("").Stage
	for playerID, preselection := range a.activity.Preselections {
		if preselection.HandID != a.handID || preselection.Stage != stage {
			delete(a.activity.Preselections, playerID)
		}
	}
}

// mutatingSnapshot captures everything a handler is allowed to change in
// a.cached before committing: the engine's own exported state, the current
// hand ID (StartHand rotates it), and the persisted activity sidecar
// (chat/reactions/preselections). Restoring all three together — never just
// a.cached alone — is what makes (*Actor).mutate's guarantee cover every
// handler that touches any of them, not only the ones a future author
// remembers to snapshot.
//
// state is captured as marshaled DynamoDB attribute values, not as a bare
// hand.State, deliberately: hand.State.ExportState is a SHALLOW copy — its
// Players slice holds the exact same *Player pointers the live table
// mutates in place (Ready, Stack, HoleCards, Contributed, ...). A plain
// `before := a.cached.ExportState()` / `a.cached = hand.NewTableFromState(before)`
// pair — the very convention every apply*AndCommit handler used before this
// change — silently fails to undo any in-place field mutation on an
// already-seated player, because "before" aliases the same struct the
// handler just mutated: restoring from it is then a no-op for that player.
// Round-tripping through attributevalue.MarshalMap/UnmarshalMap — the exact
// encoding CommitAction uses for the real write — forces a genuine deep
// copy the same way a real reload from DynamoDB would, so the restored
// table is never just a second reference to the mutated one.
type mutatingSnapshot struct {
	stateAV  map[string]types.AttributeValue
	handID   string
	activity tablestore.TableActivity
}

func (a *Actor) snapshotForMutate() (mutatingSnapshot, error) {
	av, err := attributevalue.MarshalMap(a.cached.ExportState())
	if err != nil {
		return mutatingSnapshot{}, err
	}
	return mutatingSnapshot{
		stateAV:  av,
		handID:   a.handID,
		activity: cloneActivity(a.activity),
	}, nil
}

func (s mutatingSnapshot) restore(a *Actor) {
	var state hand.State
	if err := attributevalue.UnmarshalMap(s.stateAV, &state); err != nil {
		// The marshal that produced s.stateAV just succeeded moments ago on
		// this exact type, so a failure to reverse it here means hand.State's
		// encoding is broken in a way that also breaks every real commit —
		// not something worth trusting a half-restored a.cached over. Drop
		// the whole cache instead, the same fallback handleSafely's panic
		// recovery (#29/PR #126) uses, so the next command reloads
		// authoritative state from the store rather than trusting anything
		// left over from this failed restore.
		slog.Error("table: failed to restore pre-mutation snapshot", "table_id", a.id, "err", err)
		a.cached = nil
		a.version = 0
		a.handID = ""
		a.activity = tablestore.TableActivity{}
		return
	}
	a.cached = hand.NewTableFromState(state)
	a.handID = s.handID
	a.activity = s.activity
}

// cloneActivity deep-copies a TableActivity so a restored snapshot can never
// alias the live actor's slices/map — a shallow struct copy would still
// share the same backing Chat/Reactions arrays and Preselections map, so an
// in-place mutation made after the snapshot was taken (e.g. a delete on the
// live map) would silently corrupt the "before" copy too.
func cloneActivity(act tablestore.TableActivity) tablestore.TableActivity {
	clone := tablestore.TableActivity{
		Chat:      append([]tablestore.ChatMessage(nil), act.Chat...),
		Reactions: append([]tablestore.Reaction(nil), act.Reactions...),
	}
	if act.Preselections != nil {
		clone.Preselections = maps.Clone(act.Preselections)
	}
	return clone
}

// mutate is the structural guard for every handler that mutates a.cached,
// a.handID and/or a.activity before committing. It snapshots all three, runs
// fn, and on ANY error fn returns — a validation rejection partway through, an
// engine error, or the final a.commit call itself failing — restores the
// exact pre-call snapshot. This is what makes the cache-rollback obligation
// structural instead of convention: a handler can no longer forget the
// snapshot/restore dance, because there is no path through mutate that
// leaves a partial mutation trusted in the actor's cache without either a
// matching successful commit or an automatic restore. fn is expected to
// itself call a.commit(...) as its last successful step; mutate does not
// call commit for callers, so they keep full control over the
// ActionLogEntry and any extra transact items.
//
// This is exactly the failure mode that produced the 2026-09-01 duplicate
// seat incident (a handler's uncommitted mutation left trusted in a.cached,
// persisted for real by a later, unrelated successful commit) — see
// docs/specs/2026-09-01-duplicate-seat-commit-guard.md. It complements,
// rather than duplicates, PR #126's separate panic-recovery guard (#29): a
// panic mid-handler unwinds past any deferred restore this file could add,
// so that guard instead drops the whole cache in its recover() to force a
// reload from the store on the next command. mutate only ever needs to
// handle a normal returned error, which never unwinds the stack — so it can
// restore the snapshot in place, without a round trip back to the store.
func (a *Actor) mutate(fn func() error) error {
	if a.cached == nil {
		return fn()
	}
	before, err := a.snapshotForMutate()
	if err != nil {
		return fmt.Errorf("table: snapshot table state before mutating: %w", err)
	}
	if err := fn(); err != nil {
		before.restore(a)
		return err
	}
	return nil
}

func (a *Actor) commitActivity(ctx context.Context, broadcast bool, apply func() error) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	err := apply()
	if errors.Is(err, tablestore.ErrVersionConflict) {
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		err = apply()
	}
	if errors.Is(err, tablestore.ErrDuplicateAction) {
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
		err = nil
	}
	if err != nil {
		return err
	}
	if broadcast {
		a.broadcastAll()
	}
	return nil
}

// handleSnapshot loads the table (seeding on first touch) and returns the
// viewer-specific snapshot. Built inside Run so it never races broadcastAll's
// concurrent ViewFor calls over a.cached.
func (a *Actor) handleReady(ctx context.Context, c ReadyCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	apply := func() error { return a.applyReadyAndCommit(ctx, c) }
	if err := a.retryOnConflict(ctx, apply); err != nil {
		if !errors.Is(err, tablestore.ErrDuplicateAction) {
			return err
		}
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) applyReadyAndCommit(ctx context.Context, c ReadyCmd) error {
	// c.Ready can drive tryStartHand (a full StartHand() — dealing, dealer
	// rotation, blind posting) straight into a.cached, and a.handID alongside
	// it. mutate is what guarantees a commit failure below that isn't a
	// version conflict (a transient store error, not just something
	// retryOnConflict already reloads on) can't leave that uncommitted
	// mutation trusted in this actor's cache with no matching
	// poker_action_log entry — that is exactly what let the 2026-09-01
	// incident's ghost seat and dropped player survive to be persisted for
	// real by a later, unrelated successful commit.
	return a.mutate(func() error {
		if !a.isSeated(c.PlayerID) {
			return fmt.Errorf("table: player %s is not seated", c.PlayerID)
		}
		a.markLastAction(c.PlayerID)
		for _, p := range a.cached.PlayersForActor() {
			if p.ID == c.PlayerID {
				p.Ready = c.Ready
			}
		}
		action := "not_ready"
		if c.Ready {
			a.cached.RequestReturnFromSitOut(c.PlayerID)
			action = "ready"
			// Sit-out (ready:false) never raises the ready-player count, so it must not
			// trigger tryStartHand: doing so during Stage==Complete forced the "not enough
			// ready players" fallback early, snapping the table back to WaitingForPlayers
			// and clearing payouts before next_hand_unix_ms elapsed — killing the other
			// player's win banner mid-countdown. armNextHandTimer still starts the next
			// hand once the grace period actually ends.
			if a.cached.Stage() == hand.WaitingForPlayers {
				a.tryStartHand(ctx)
			}
		} else {
			a.cached.SitOutForActor(c.PlayerID)
		}
		return a.commit(ctx, c.ActionID, &tablestore.ActionLogEntry{
			PlayerID: c.PlayerID, ActionID: c.ActionID, Action: action,
		})
	})
}

// tryStartHand attempts to start a new hand if the table is between hands.
// "need at least 2 ready players" is not a caller error — the table just
// keeps waiting; StartHand's own error is swallowed here on purpose. Called
// from both a Ready toggle and a fresh Join, since a join alone can now bring
// the table to 2+ ready players (auto-ready on join). Complete is deliberately
// excluded: nextHandCmd owns that transition and preserves the reveal window.
func (a *Actor) tryStartHand(ctx context.Context) {
	if a.cached.Stage() == hand.WaitingForPlayers {
		if err := a.cached.StartHand(); err == nil {
			a.handID = newHandID()
			a.prunePreselections()
		}
	}
}

// saveHandHistorySnapshot persists the table's current (pre-reset) state to
// poker_table_state_history for audit purposes. Best-effort: this is an
// append-only audit copy, not the authoritative item, so a failure here never
// blocks the hand transition — it only logs. A version-conflict
// retry re-running tryStartHand is harmless: once another instance has
// already advanced the stage past Complete, the reloaded cache no longer
// satisfies the Complete guard above, so the snapshot is not repeated.
func (a *Actor) saveHandHistorySnapshot(ctx context.Context) {
	if a.store == nil {
		return
	}
	if err := a.store.SaveTableStateHistory(ctx, a.id, timeNowFunc().Unix(), a.cached.ExportState()); err != nil {
		slog.WarnContext(ctx, "table hand history snapshot failed",
			"table_id", a.id, "hand_id", a.handID, "err", err)
	}
}

func (a *Actor) handleAct(ctx context.Context, c ActCmd) error {
	if err := a.ensureLoaded(ctx, false); err != nil {
		return err
	}
	if err := a.validateActionPrecondition(ctx, c); err != nil {
		return err
	}
	a.markLastAction(c.PlayerID)
	_, err := a.applyActAndCommit(ctx, c)
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		// Two distinct reasons to reload and retry exactly once:
		//   - ErrVersionConflict: another instance committed first: definite
		//     staleness.
		//   - trustCache==true and any other error: this instance normally
		//     trusts a.cached between commits (ensureLoaded(ctx,false) is a
		//     no-op once cached is set) and only re-reads on a version
		//     conflict from ITS OWN write attempts. But ARCHITECTURE.md §2 /
		//     tablews.go's RegisterTableWS explicitly allow any instance to
		//     accept any table's connections directly, with no proxying to
		//     the lease holder — so another instance can commit actions this
		//     one never observes, and its next local Act() call (e.g. a
		//     turn-order check) can reject a genuinely legal action against
		//     stale data without ever reaching a conditional write to
		//     conflict on. Retrying here can't mask a truly invalid action:
		//     if it's genuinely invalid, re-running it against freshly
		//     loaded state reproduces the identical rejection.
		if errors.Is(err, tablestore.ErrVersionConflict) || a.trustCache {
			if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
				return reloadErr
			}
			a.markLastAction(c.PlayerID)
			_, err = a.applyActAndCommit(ctx, c)
		}
	}
	if errors.Is(err, tablestore.ErrDuplicateAction) {
		// The guard proves another commit already won. Discard the speculative
		// local mutation before outcome logging or broadcasting.
		if reloadErr := a.ensureLoaded(ctx, true); reloadErr != nil {
			return reloadErr
		}
	}
	if err != nil && !errors.Is(err, tablestore.ErrDuplicateAction) {
		return err
	}
	if err := a.commitOutcomeLogEntries(ctx); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) validateActionPrecondition(ctx context.Context, c ActCmd) error {
	// Internal/system commands and older direct unit tests omit preconditions.
	// The WebSocket boundary requires both for every user-originated act.
	if c.ExpectedSnapshotVersion == 0 && c.ExpectedHandID == "" {
		return nil
	}
	if c.ExpectedSnapshotVersion == 0 || c.ExpectedHandID == "" {
		return fmt.Errorf("table: incomplete action precondition")
	}
	if uint64(a.version) != c.ExpectedSnapshotVersion || a.handID != c.ExpectedHandID {
		if err := a.ensureLoaded(ctx, true); err != nil {
			return err
		}
	}
	if uint64(a.version) != c.ExpectedSnapshotVersion || a.handID != c.ExpectedHandID {
		return fmt.Errorf("table: stale action state")
	}
	return nil
}
