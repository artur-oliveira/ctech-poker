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
