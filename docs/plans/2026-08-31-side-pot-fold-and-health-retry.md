# Side-pot fold handling + resilient /health probe + table_theme wiring — 2026-08-31

Three independent correctness fixes, reported together from a `game.har` session.

## 1. A folded player's bet must not create a side pot (`api/`)

### Symptom

When a player bet chips and then folded, `sidepots.ComputeSidePots` still drew a
pot-layer boundary at that player's contribution amount, producing a visible
"pote lateral" (side pot) that no live all-in player had created. The engine
then either raked it as a contested pot or — per the old
`TestOrphanedSidePotLayerIsRefundedNotDropped` — refunded the folded chips back
to the folded players.

### Root cause

`ComputeSidePots` took only `{PlayerID, Amount}` and treated **every** distinct
contribution level as a side-pot split point. Fold state was applied by the
callers *after the fact* (filtering `Eligible`), so a folded player's partial
bet still fractured the pot.

### Fix

`sidepots.Contribution` gains a `Folded bool`. `ComputeSidePots` now:

- Draws layer boundaries **only** at the contribution levels of players still in
  the hand. A folded player's amount adds no boundary.
- Computes each layer's amount as the sum, over *all* contributors (folded
  included), of the chips they put into that band — so folded "dead money" is
  absorbed into the layers live players contest.
- Keeps a boundary for folded money that sits entirely **above** every live
  contribution, only so it can be classified:
  - exactly one contributor in a band → `PotLayer.Uncalled = true`, returned to
    that player (a genuine uncalled bet — folded or not);
  - two-plus folded contributors, no live eligible → dead money, rolled down
    into the nearest contested layer (the main pot).

`hand.runShowdown` and `hand.potViews` pass `Folded` through; `runShowdown`
keys the refund branch off `layer.Uncalled` instead of `len(Eligible) == 1`;
`potViews` folds `Uncalled` layers into the main pot for display so the shown
total still matches the chips in the middle.

### Behaviour change

`TestOrphanedSidePotLayerIsRefundedNotDropped` is replaced by
`TestFoldedPlayersMatchedMoneyRollsIntoContestedPot`: when two players bet and
called each other into a side pot and then **both fold**, that money is now dead
money for the remaining live players, **not** a refund to the folders. This
matches real poker ("a player who folds forfeits any interest in the pot") and
was an explicit product decision.

Rake is unchanged for genuinely-called fold wins (the called portion is still a
contested, rakeable pot); only truly-uncalled money stops being raked, which is
correct.

### Not changed (deliberately)

The heads-up over-shove (all-in 10k vs all-in 50k): the 40k excess is already
returned to the shover at pot resolution via the `Uncalled` layer
(`TestHeadsUpOverShoveExcessIsFlaggedUncalled` /
`TestUncalledAllInExcessIsNotCountedAsAWin`). Making `internal/engine/betting`
cap the commitment at the table *before* it leaves the stack is a larger
betting-engine change and is left as a follow-up.

### Tests

- `internal/engine/sidepots`: `TestFoldedPartialBetsDoNotCreateASidePot`,
  `TestFoldedMidStackContributionIsAbsorbed`,
  `TestFoldedPlayerUncalledExcessIsReturned`,
  `TestHeadsUpOverShoveExcessIsFlaggedUncalled`.
- `internal/engine/hand`: `TestFoldedPlayersMatchedMoneyRollsIntoContestedPot`.

## 2. `/health` probe retries before declaring the API offline (`ui/`)

### Symptom

A single timed-out (or CORS-shaped-failed) `GET /v1.0/health` immediately
published `status: 'unavailable'`, blacking out the app on one slow response.

### Fix

`checkApiLiveness` (`src/lib/network/liveness.ts`) now retries the probe
`HEALTH_PROBE_ATTEMPTS` (3) times with a `HEALTH_PROBE_RETRY_MS` (600 ms)
backoff between attempts. The outage is published **only** after every attempt
fails; the snapshot stays `'checking'` (or its prior value) while retries are in
flight. If `navigator.onLine` flips to false between attempts, it stops
immediately and publishes `reason: 'offline'`.

Worst-case time to declare an outage: `3 × HTTP_TIMEOUT_MS + 2 × 600 ms ≈ 10.2 s`.
The `NetworkProvider` outage poll loop and `livenessPollDelay` backoff are
unchanged — they now retry a *probe that already retried*.

### Tests

`src/lib/network/liveness.test.ts`: retries-then-fails, no outage while retries
pending, recovers on a later retry, stops on mid-retry offline, per-attempt
3 s abort across all retries.

## 3. `POST /players/me {"table_theme": …}` was silently ignored (`api/`)

### Symptom

Changing the felt from the client (`TablePreferencesDialog` →
`updateMe({table_theme})`) did nothing: the value never persisted and the
response body carried no `table_theme` field.

### Root cause

The premium-cosmetics overhaul (`docs/specs/2026-08-21-premium-cosmetics-overhaul.md`
Part 5) landed `player.Service.SetTableTheme`, the store method, the catalog
entry and validation — but not the HTTP layer.
`internal/api/v1/player.go`'s `UpdatePlayerRequest` had no `TableTheme` field,
`updateMe` had no branch for it, and `playerResponse` didn't emit it. The
frontend side (`ui/src/lib/api/player.ts`, `tablePreferences.ts`) was fully
wired — only the server dropped it.

### Fix

- `UpdatePlayerRequest` gains `TableTheme *string \`json:"table_theme"\``.
- `updateMe` calls `h.players.SetTableTheme`, mapping `ErrInvalidTableTheme` →
  400 and `ErrCosmeticNotOwned` → 400 (a premium felt the player has not
  bought).
- `playerResponse` emits `"table_theme": profile.EffectiveTableTheme()`.
- Same `ErrCosmeticNotOwned` → 400 mapping added to the existing `deck_variant`
  branch, which previously returned 500 for an unowned premium deck.

### Tests

`internal/api/v1`: `TestUpdateMeSetsTableThemeAndEchoesIt` (free `classic`
persists and round-trips; unknown id → 400).
