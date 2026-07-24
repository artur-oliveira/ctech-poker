# Poker table pacing, audio cue, equity UI, bet keybinds, and state-history audit

## Context

The table currently feels mechanical: a 15s turn clock, a 5s gap between hands, snappy 220ms transitions, and no audio
cue for "it's your turn" (easy to miss on a busy table). The user wants a calmer, more legible pace, plus:

- Equity already renders as plain text (`Chance {n}%`) and hand-category already renders (`seat.hand_category` →
  `HAND_CATEGORY_LABELS`) — user wants equity restyled as a color-coded progress bar; hand shown is already there, no
  new work.
- Bet-sizing keyboard shortcuts (arrows, H, A, ctrl-accelerate) — presets for min/½-pot/pot/max already exist as
  buttons; this adds keyboard bindings to the existing `RaiseControl` amount state, no new backend.
- An audit trail of each hand's final state before the table resets for the next hand, in a new DynamoDB table
  (`poker_table_state_history`).
- A real bug: the user saw a fresh joiner sometimes appear to instantly "win" — investigated and confirmed **not** a
  persistence issue (writes to
  `poker_table_state` are already version-conditional). The actual cause is
  `handleJoin` skipping a fresh reload of the lease-holding actor's in-memory cache (`ensureLoaded(ctx, false)` is a
  no-op when `trustCache` is true and
  `cached != nil`). Per-hand reset-to-players/chips is **already** what
  `StartHand()` does on every hand transition — nothing new needed there. User confirmed: fix the join root cause *and*
  keep the audit-history feature (they're complementary, not alternatives).

## Backend (`api/`)

**`internal/table/turntimeout.go`**

- `DefaultTurnTimeout`: 15s → 30s.
- `NextHandDelay`: 5s → 8s.
- `RevealGrace`: 1500ms → 2200ms (covers the slower flop stagger below so the turn clock doesn't visibly start ticking
  before the reveal animation finishes).
- `RunoutStreetDelay`: 2s → 2.6s (calmer all-in runout pacing, same spirit).
- Update `armNextHandTimer`'s doc comment in `actor.go:891` ("5s post-hand countdown" → "8s").

**Join-contamination fix — `internal/table/actor.go:769-772` (`handleJoin`)**

- Change `a.ensureLoaded(ctx, false)` → `a.ensureLoaded(ctx, true)`. Join is the highest-risk moment for a new viewer to
  be folded into a stale in-memory snapshot (leftover `LastOutcome`/board/deadlines from before they existed) — forcing
  a fresh read here costs one extra DynamoDB read per join, same pattern already used before
  turn-timeout/next-hand/runout-step handling.

**State-history audit trail**

- `cdk/lib/dynamodb-stack.ts`: add `'poker_table_state_history'` to the
  `TableName` union; provision it via the existing `table(name, withSortKey, ...)`
  helper: `table('poker_table_state_history', true)` (sort key, no TTL — this is a permanent audit log, no stream
  needed). Attribute names stay the repo's generic `pk`/`sk` (not literally `table_id`/`timestamp`) — `pk` holds the
  table ID, `sk` holds the unix-seconds string — matches every other table here.
- `cdk/bin/poker.ts`: add `tableStateHistoryArn: dynamoStack.tables.get('poker_table_state_history')!.tableArn` next to
  the other `*Arn` wiring (~line 83).
- `cdk/lib/api-stack.ts`: add `tableStateHistoryArn: string` to `ApiStackProps`, destructure it, append to the
  `tableArns` array (~line 102-105) so the EC2 role gets the same `GetItem/PutItem/Query/...` grant as the other tables.
- `api/internal/tablestore/store.go` / `dynamo.go`: add
  `tableStateHistory = "poker_table_state_history"` const, a `history dynamo.Base`
  field on `Store`, wired in `NewStore`. Add:
  ```go
  func (s *Store) SaveTableStateHistory(ctx context.Context, tableID string, unixSeconds int64, state hand.State) error {
      item, err := dynamo.Encode(struct {
          PK    string     `dynamodbav:"pk"`
          SK    string     `dynamodbav:"sk"`
          State hand.State `dynamodbav:"state"`
      }{PK: tableID, SK: fmt.Sprintf("%d", unixSeconds), State: state})
      if err != nil {
          return fmt.Errorf("tablestore: encode history snapshot: %w", err)
      }
      return s.history.PutItem(ctx, item)
  }
  ```
  Plain `PutItem` (no transaction/condition needed — this is an append-only audit copy, not the authoritative item).
- `api/internal/table/actor.go`: the single choke point for "a hand just finished and we're about to reset it" is
  `tryStartHand()` (called from
  `applyReadyAndCommit`, `applyJoinAndCommit`, and `handleNextHand` — all three already have `ctx` in scope). Thread
  `ctx` through:
  ```go
  func (a *Actor) tryStartHand(ctx context.Context) {
      if a.cached.Stage() == hand.WaitingForPlayers || a.cached.Stage() == hand.Complete {
          if a.cached.Stage() == hand.Complete {
              a.saveHandHistorySnapshot(ctx)
          }
          if err := a.cached.StartHand(); err == nil {
              a.handID = newHandID()
          }
      }
  }

  func (a *Actor) saveHandHistorySnapshot(ctx context.Context) {
      if a.store == nil {
          return
      }
      if err := a.store.SaveTableStateHistory(ctx, a.id, timeNowFunc().Unix(), a.cached.ExportState()); err != nil {
          metrics.EmitTableMetric(a.env, "TableStateHistorySaveError", 1, map[string]string{"table_id": a.id})
      }
  }
  ```
  Best-effort: a history-write failure never blocks the hand transition, only emits a metric (matches this package's
  existing no-logger, metrics-only observability convention). Update the 3 call sites to `a.tryStartHand(ctx)`. A
  retry-after-version-conflict re-running `tryStartHand` is harmless: the reloaded cache's stage will no longer be
  `Complete` once another instance has already advanced it, so the guard simply skips the second snapshot.

## Frontend (`ui/`)

**Your-turn sound — already-shipped asset, just needs wiring**

- `public/sounds/your-turn.mp3` already exists on disk but isn't registered.
- `src/lib/sound.ts`: add `'your_turn'` to `SoundName` and `FILES`.
- `src/lib/hooks/useTableRealtime.ts`: `playSoundForTransition` (line 81) gains a `viewerId?: string` param; at the top,
  before the existing priority chain:
  ```ts
  if (viewerId && next.current_player_id === viewerId && previous.current_player_id !== viewerId) {
    playSound('your_turn');
  }
  ```
  Update the call site (line 165) to pass `viewerId` (already in scope in this hook). This can co-fire with another
  transition sound (e.g. someone bets into you) — that's fine, they're different cues.

**Slower, calmer motion — `src/app/globals.css`**

- Board card reveal (flop/turn/river): `board-card-reveal` duration 560ms → 780ms (`:1697`); its stagger delay and the
  matching flip delay both 220ms → 320ms (`:1698`, `:1746-1747`) — keeps the flop's already-correct one-card-at-a-time
  stagger, just paced slower. River's `card-flip-slow` 760ms → 900ms (`:1735`). (Confirmed: flop already deals one card
  at a time via `--deal-index`, and turn/river already add exactly one card each with no stagger needed — this is a
  timing tweak, not a structural fix.)
  Hole-card deal/flip timings are left as-is (user's ask was about board-card pacing, not the initial deal).
- General seat-state transition `.game-seat` 220ms → 380ms (`:1773`).
- `.game-seat.is-turn:after` pulse 1.5s → 2s (`:1788`).

**Equity as a color-coded progress bar**

- No shadcn config exists; `@base-ui/react/progress` is already installed and every `components/ui/*` primitive
  hand-wraps a `@base-ui/react/*` part (`switch.tsx` is the template). Add `src/components/ui/progress.tsx`:
  Root/Track/Indicator wrapper, `Indicator` width set inline from the `value`
  prop (0-100), `cn()` for classes, same shape as `switch.tsx`.
- `src/components/table/Seat.tsx` (`:46-48`): replace the `<small
  className="seat-equity">Chance {chance}%</small>` block with a `<div
  className="seat-equity">` containing the `Progress` bar (labelled, same
  `aria-label` text kept) + the existing numeric label. Indicator color by threshold: `chance <= 20` red, `<= 60`
  yellow, `> 60` green (a tiny
  `equityTone(chance)` helper returning a Tailwind class). Still gated on
  `chance != null && isViewer` — no change to that visibility rule.
- `globals.css`: `.seat-equity` currently inherits the generic `.seat-info small`
  rule (`:1902-1905`) — since it's becoming a `div` (not `<small>`), add a dedicated `.seat-equity` rule (compact
  width/height/rounded track) near that block, plus a one-line compact-tier override inside the existing `@media
  (max-width: 800px), (max-height: 620px) and (orientation: landscape)` block (`:3684`). The two responsive layers that
  already hide
  `.seat-info .seat-equity` in the portrait/vertical-ring mobile mode (`:3384`)
  and the phone font-size block (`:3742`) need no change — equity already degrades away there today for hand-category
  too; keep that existing tradeoff.

**Bet-sizing keyboard shortcuts — `src/components/table/ActionBar.tsx`, inside `RaiseControl`'s existing keydown
effect (`:65-76`)**

- Extend the same `window.addEventListener('keydown', onKey)` effect (don't add a second listener):
    - `h` → `setAmount(presets[1].value)` (½ pot, mirrors the existing "½ pote" button).
    - `a` → `setAmount(maxRaise); onRaise(maxRaise)` (sets max **and** submits immediately, per spec — the only shortcut
      that auto-submits).
    - `ArrowDown` → `setAmount(minRaise)`; `ArrowUp` → `setAmount(maxRaise)` (jump, not increment, per spec).
    - `ArrowLeft` / `ArrowRight` → `setAmount(amount ∓/± raiseStep)`, clamped to
      `[minRaise, maxRaise]` (reuses the same `raiseStep` already driving the slider's `step` and the preset-snapping
      `snap()` helper).
    - Ctrl held during `ArrowLeft`/`ArrowRight` → step × 3 instead of × 1 (a named
      `FAST_STEP_MULTIPLIER = 3` constant — user's ask gave a 2-4x example range, picking the middle; trivial to
      retune).
- `isPlainKey` (`:36-38`) rejects `event.ctrlKey` outright, which is correct for the single-letter shortcuts
  (f/c/p/r/h/a) but wrong for the new arrow keys. Add a second guard `isBetAdjustKey` (same as `isPlainKey` minus the
  `ctrlKey`
  exclusion) used only for `ArrowLeft`/`ArrowRight`/`ArrowUp`/`ArrowDown`.
  `event.preventDefault()` on all of these (matches the existing `r` handler) so a focused native `<input type=range>`
  doesn't also apply its own default step-by-arrow-key behavior on top of ours.
- No new mobile/touch UI needed — presets/½-pot/max buttons already exist and are already touch-friendly; these are
  desktop keyboard shortcuts layered on top of what's already there.

## Verification

- `api`: `go test ./internal/table/... ./internal/tablestore/... -race` — extend the existing table/actor tests with one
  case asserting `tryStartHand` writes exactly one history item when transitioning out of `Complete`, and one asserting
  `handleJoin` reflects a concurrently-committed change from a second
  `Store`-backed actor instance (regression test for the join fix). DynamoDB Local via `docker-compose.test.yml` already
  covers this pattern.
- `cdk`: `npm test` (Jest/CDK assertions) after adding the new table + IAM grant.
- `ui`: `eslint src --max-warnings 0` and `next build` (the project's only gate, per `ui/CLAUDE.md`) must both stay
  clean.
- Manual: run the app (`run` skill / dev servers), open a table with two browser profiles, confirm: 30s turn clock, 8s
  between hands, your-turn sound fires only for the player whose turn it is, flop/turn/river feel visibly slower and
  still one-card-at-a-time, equity bar shows correct color at each threshold, arrow/H/A/ctrl+arrow keys move the raise
  slider as specified, and a fresh join mid-Complete-stage never shows a stale outcome. Check phone portrait width and
  tablet landscape width for the equity bar and action bar layout (existing responsive tiers should already cover both).