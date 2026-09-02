# 2026-09-01 — Duplicate-seat commit guard

## Incident

Table `01M1C5GQR7HWXSNSSX8Q49XN9X` (sandbox) ended up with one player ("Kauan")
occupying three seats simultaneously, and a second player ("Thiago"/"Dexther")
silently dropped from `Players` mid-session, during a ~30min window
(2026-09-01 15:20–15:58 UTC) where the ASG's spot-capacity rebalancing
repeatedly terminated instances faster than replacements could launch
(`UnfulfillableCapacity`).

Root cause, confirmed from `poker_action_log` (version 1868→1869 on that
table): a handler mutated `a.cached` in place (seat append via
`AddMidHandJoiner`/`AddWaitingPlayer`, or `tryStartHand`'s full `StartHand()`),
then failed to commit for a **non**-version-conflict reason (a transient store
error during the capacity crunch). `applyReadyAndCommit` had no
snapshot/rollback for this failure path at all (unlike `applyLeaveAndCommit`),
so the uncommitted mutation stayed trusted in the actor's in-memory cache
under `trustCache`, which skips every subsequent reload
(`ensureLoaded(ctx, false)`). The next unrelated successful commit on that
same actor flushed the poisoned cache — including the duplicate seat and the
dropped player — for real.

## Fix

- `hand.Table.DuplicateSeatIDForActor()` (`internal/engine/hand/hand.go`):
  reports the first player ID occupying more than one seat, if any.
- `Actor.commit` (`internal/table/actor.go`) refuses to persist any state that
  fails this check — a hard backstop independent of which handler produced
  the corruption.
- `Actor.ensureLoaded` forces a real reload (bypassing `trustCache`) the
  moment it finds a duplicate seat in the current cache, so the actor
  self-heals on the very next command instead of wedging on the refusal
  forever.
- `applyReadyAndCommit` now takes the same before/rollback snapshot
  (`a.cached` + `a.handID`) as `applyLeaveAndCommit`/`applyJoinAndCommit` on
  any commit failure — closing the specific gap that let this incident's
  mutation survive an unrelated actor's failed write.

## Remediation

The affected table's corrupted state (duplicate seat) was cleared by hand.
All players seated at that table during the incident were credited their
last-known-good stack via `ctech-wallet`'s sandbox credit endpoint, plus a
flat 1,000,000 sandbox-chip goodwill credit each, keyed by an idempotent
`idempotency_key` per player so a retry can't double-credit.

## Still open (infra)

The underlying trigger — an ASG spot-instance rebalance/termination outrunning
replacement capacity, with no lifecycle hook forcing
`tablemanager.DrainAndRelease` to finish before termination — is unchanged by
this fix. This guard stops the *symptom* (corrupted state reaching DynamoDB)
regardless of cause; it does not reduce how often two actors can end up
processing the same table concurrently during an instance churn.

## 2026-09-02 follow-up — structural cache-rollback guard (#51)

The 2026-09-01 fix above closed the specific gap in `applyReadyAndCommit`, but
left the underlying obligation ("snapshot `a.cached`/`a.handID`/`a.activity`
before mutating, restore on any commit failure") as a convention every
mutating handler had to re-implement by hand — exactly the shape that let the
gap exist in the first place. A repo-wide review (`docs/plans/2026-09-02-systematic-review-and-issue-backlog.md`
§3 Issue 23 / GitHub #51) found the same gap, in the same "mutate `a.cached`
in place, then call `a.commit`, with no rollback at all" shape, in most of
`internal/table/actor.go`'s other handlers — `applyActAndCommit` (every
fold/check/call/bet/raise), `handleSitOut`, `handleShowCards`,
`handleRequestRabbitHunt`, `handleRequestExit`, `handleCancelExit`,
`handleRabbitHuntVerifyFailed`, `handleSetRunItTwice`, `handlePostBigBlind`,
`handleSetIdentity`, `handleEscalate`, and the activity-only handlers
(`handleChat`, `handleReaction`, `handlePreselect`) mutating `a.activity`
with no rollback either.

Fix: `Actor.mutate(fn func() error) error` is now the single structural guard
every one of those handlers routes its mutation-and-commit body through. It
snapshots `a.cached`, `a.handID` and `a.activity` before calling `fn`, and
restores all three on any error `fn` returns — a validation rejection, an
engine error partway through, or the final `a.commit` call itself failing —
so a handler can no longer forget the snapshot/restore dance by construction.

This also fixed a **second, subtler bug in the pre-existing convention
itself**: `applyReadyAndCommit`/`applyJoinAndCommit`/`applyLeaveAndCommit`'s
`before := a.cached.ExportState()` / `a.cached = hand.NewTableFromState(before)`
pattern snapshots a *shallow* copy — `ExportState`'s `Players` slice holds the
exact same `*Player` pointers (and, for a removal, the same backing array)
the live table then mutates in place. A field mutation on an already-seated
player (`p.Ready = ...`, `p.Stack = ...`, an in-place slice removal via
`RemovePlayerForActor`) is silently invisible to that "restore" — it aliases
the very object just mutated, so the rollback is a no-op for exactly the kind
of mutation most handlers perform. `Actor.mutate`'s snapshot instead
round-trips the table state through the same `attributevalue.MarshalMap`/
`UnmarshalMap` encoding `CommitAction` uses for the real write, forcing a
genuine deep copy the way an actual DynamoDB reload would. Proven by
`internal/table/cacherollback_test.go` (`TestMutateRestoresCacheHandIDAndActivityOnError`,
`TestHandleJoinRollsBackCacheWhenSettlementIntentFails`,
`TestHandleLeaveRollsBackCacheWhenSettlementIntentFails`).

Complements, not duplicates, PR #126's `handleSafely` panic-recovery guard
(#29): that guard catches a panic mid-handler (which unwinds past any
deferred restore) by dropping the whole cache to force a reload; `mutate`
only ever needs to handle a normal returned error, so it restores the
snapshot in place without a store round trip.
