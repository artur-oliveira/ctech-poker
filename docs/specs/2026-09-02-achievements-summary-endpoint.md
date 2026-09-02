# Achievement summary endpoint — `GET /players/me/achievements/summary`

## Problem (issue #71)

`GET /players/me/achievements` returns a `Page<PlayerAchievementProgress>` (DynamoDB-cursor
paginated, 100/page). The frontend (`lib/api/achievements.ts`) never follows the cursor, so a
player who has touched more distinct achievement keys than one page holds gets a completion %
that understates their real progress, a `next-star` computed from a partial set, and a secret
achievement past its first tier that never surfaces if its key happens to land on page 2. The
showcase featured-achievement picker has the identical bug. Frontend issue #79 tracks fixing the
client to consume this new endpoint; this change is backend-only.

## Fix

Added `GET /v1.0/players/me/achievements/summary` (JWT, `?mode=sandbox|real`) — a single,
non-paginated response covering the whole (bounded, ~dozens of entries) achievement catalog:

```jsonc
{
  "mode": "sandbox",
  "totals": {"revealed": 41, "unlocked": 12, "completed": 3, "stars": 27, "max_stars": 205},
  "achievements": [
    {
      "key": "wins", "metric": "hand_won", "tiers": [{"stars":1,"threshold":1}, ...],
      "progress": 15, "stars": 2, "unlocked": true, "completed": false,
      "next_target": 100, "max_target": 10000
    },
    ...
  ]
}
```

- **Every** catalog achievement is present except a secret one the player hasn't reached the first
  tier of yet — the same reveal gate `Store.ListAchievements` already applies to the paginated
  endpoint, so a secret achievement past its first tier is now always included regardless of where
  its key would have landed on a page.
- `progress`/`stars`/`unlocked`/`completed`/`next_target`/`max_target` are derived per achievement
  from the full tier ladder, so completion % and "next star" are computed from real, complete data.
- `totals` are catalog-wide roll-ups (stars earned vs. max possible, counts unlocked/completed),
  computed server-side so the client cannot understate them from a partial fetch.
- No `unlocked_at` timestamp: `poker_achievement_progress` rows only ever store a running counter
  (`Store.Increment`/`IncrementStreak`), never a timestamp of when a tier was first crossed — that
  data doesn't exist to serve. Deferred; would need a write at tier-crossing time in
  `Service.RecordHand`'s `TierCrossed` branch if ever wanted.

## Implementation

- `achievements.Store.AllAchievements(ctx, playerID, mode)` (`store.go`) does the "single batched
  read" the issue asked for: it calls the existing `ListAchievements` in a loop, following
  DynamoDB's `LastEvaluatedKey` internally, capped at `allProgressMaxPages` (20) pages of
  `allProgressPageSize` (200) each — bounded, never an unbounded fetch loop, and generous enough
  that a real player's row count (well under a thousand) always finishes on page 1.
- `achievements.BuildSummary(mode, progress)` (new `summary.go`) folds those progress rows over
  `Catalog` into the `Summary`/`AchievementState` shape above — pure, no I/O, unit-tested directly
  in `summary_test.go`.
- `playerHandlers.achievementsSummary` (`api/v1/player.go`) is the thin handler: derive `playerID`
  from the JWT (never trust a client-supplied id, per this repo's IDOR convention), call
  `AllAchievements`, hand the rows to `BuildSummary`, return JSON. Route registered alongside the
  existing `/players/me/achievements` under the same auth group.
- `playerAchievementStore` interface gained `AllAchievements` alongside `ListAchievements`; the one
  other implementer is the real `*achievements.Store` used by `router.go`'s wiring — no other call
  site to update.

## Not done here (frontend, #79)

`ui/src/lib/api/achievements.ts`, `achievements/page.tsx`, and `ProfileShowcaseDialog.tsx` still
build their progress map from the paginated endpoint. Consuming the new summary endpoint (and
sharing one hook between the page and the showcase dialog) is #79's scope, not this change's.
