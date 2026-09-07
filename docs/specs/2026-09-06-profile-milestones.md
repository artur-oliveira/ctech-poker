# Profile milestones on the public showcase (#330)

**Status:** implemented · **Date:** 2026-09-06 ·
**Issue:** [#330](https://github.com/artur-oliveira/ctech-poker/issues/330) (split from #215)

## Problem

`achievements.Catalog` is entirely skill- and hand-event-based (all-in, showdown, streaks). Nothing
in it says *how long* a player has been here or *how much* they have played, so the public showcase
read as a preferences form rather than a persistent identity. `PlayerProfile.CreatedAt` existed in
the store but carried `json:"-"` and never left the API.

## What changed

### `player.Milestones` (`api/internal/player/milestones.go`)

A pure function over three counters that **already exist** — no new table, no new GSI, and nothing
added to the per-hand pipeline (#204's budget is untouched):

| Key | Category | Earned at | Derived from |
|---|---|---|---|
| `veteran_1y` / `veteran_3y` | `tenure` | 365 / 1095 days of account | `PlayerProfile.CreatedAt` |
| `hands_1k` / `hands_10k` / `hands_100k` | `volume` | 1k / 10k / 100k lifetime hands | the `hands_played` aggregate `achievements` has materialized since #198 |
| `top100` / `top10` | `ranking` | current sandbox rank ≤ 100 / ≤ 10 | `leaderboard.Service.MyRank` |

Only the highest tier in each category is returned, and each mark carries the **figure it was
earned with** (`value`: days, hands, or the rank itself) so the client can show "43.700 mãos
jogadas" rather than only the threshold that was crossed. An unranked player (`MyRank` returns
`(nil, nil)`) earns no ranking mark — rank 0 is "no rank", never "better than first".

**`top100_peak` was implemented as current rank, not peak — deliberately.** A peak needs a durable
high-water mark written whenever a rank improves, and the only place to put that write is the
per-hand path #198/#217 spent real effort taking writes *out* of. Current rank costs one Valkey
rank-mirror read (#202), falling back to the existing `gsi_hands_won` COUNT — no new index either
way, which is what the issue asked for.

### `GET /players/:playerId/showcase`

Two additive fields, both under the `ShowcasePublic` gate the whole response already sits behind —
a private profile still 404s, so no new gate was needed:

- `member_since` — `PlayerProfile.CreatedAt`, RFC3339. The showcase is the one place it is public.
- `milestones` — `[{key, category, value}]`, `[]` when none are earned.

Cost per showcase view: unchanged except **one rank lookup**, and the `hands_played` figure comes
from the `ListAchievements` call the handler already made for its featured-achievement counts. A
rank lookup that errors is logged and dropped — a missing badge is not worth failing a profile over.

`leaderboard.Service` reaches the handler through `RegisterPlayers`' existing variadic `extras`
switch (behind a `leaderboardRanker` interface with the single `MyRank` method), so no call site
changed shape. A typed-nil `*leaderboard.Service` — what the narrower test wiring passes — is
rejected in that switch, since a typed nil boxed in an interface is not `nil` and `MyRank` would
deref its store.

## Frontend

`ProfileMilestones` renders under the player's name on `/profile`: a "Jogando desde <mês de ano>"
line plus a rail of gold medal chips (earned value — the Three Materials Rule), each showing its
label and its real figure. Categories are told apart by icon and copy, never by hue. A key this
client has no copy for is skipped rather than rendered as a raw slug, so a server ahead of the
client degrades quietly. The showcase header is now top-aligned: a vertically centred avatar
drifted below the name once the identity block grew.

## Tests

`internal/player/milestones_test.go` covers one mark per category plus the tier boundaries, the
unranked case, and a missing `CreatedAt` (which must not read as the zero time and make everyone a
three-year veteran). `internal/api/v1/player_test.go`'s `TestShowcaseExposesMemberSinceAndMilestones`
covers the wired response, the private-profile 404, a failed rank lookup, and no leaderboard wired.
`ui/src/app/(app)/profile/page.test.tsx` covers the rail, the unknown-key skip, and the
render-nothing case.
