# Achievement Seasons — Design

## Summary

`api/internal/achievements/catalog.go` defines a flat, permanent list of 40+ `Achievement`s
(`Catalog []Achievement`), each with a `Key`, a `Metric`, and star `Tiers`. There is no notion of a
time-boxed achievement today — confirmed by grep, zero hits for "season" anywhere in `api/` or `ui/`.
This adds **seasonal achievements**: catalog entries that can only earn progress inside an explicit
start/end window, alongside the permanent ones, using the exact same progress-tracking and
tier-unlock machinery (`achievements.Service.RecordHand`, `Store.Increment`, `TierCrossed`) — no
parallel system, no new DynamoDB table.

## Where

- `api/internal/achievements/catalog.go` — add a `Season` field to `Achievement`.
- `api/internal/achievements/service.go` — gate progress increments on the season window inside
  `RecordHand`'s `bump`/`bumpBy` helpers (lines 53-63), plus one new explicit trigger call per new
  seasonal achievement (the same pattern every existing key already uses — e.g. `KeyBluff` is bumped
  at `service.go:76`).
- `ui/src/lib/achievements.ts` — one new entry per seasonal key in `DESCRIPTIONS`/`EXAMPLES` (already
  the pattern for every key), plus a new `SEASONAL` lookup mirroring the existing `MODE_ONLY` map
  (lines 52-55) so the achievements page can badge it "sazonal" without an API contract change.
- `ui/src/app/achievements/page.tsx` and `ui/src/app/guide/achievements/page.tsx` — display tweak
  only (badge + end-date), not read in full for this spec since the change is additive to whatever
  list rendering already exists there.

## Design

### Catalog: a `Season` field, not a second catalog

```go
// catalog.go
type Season struct {
    ID    string    // e.g. "halloween-2026" — informational, not used as a storage key (see below)
    Start time.Time
    End   time.Time // exclusive
}

func (s *Season) ActiveAt(t time.Time) bool {
    return s == nil || (!t.Before(s.Start) && t.Before(s.End))
}

type Achievement struct {
    Key    string
    Metric string
    Tiers  []Tier
    Secret bool
    Season *Season `json:"season,omitempty"` // nil = permanent, matches every existing entry
}
```

A seasonal achievement gets its own `Key` constant per occurrence (e.g.
`KeyHalloweenBluffer2026 = "halloween_bluffer_2026"`), the same way every other achievement already
has a unique key. **Next year's edition is a brand-new catalog entry with a new key and a new
`Season`**, not a reused key with a reset counter — this is the simplest correct design: DynamoDB
progress rows are keyed `pk=playerID, sk=mode#key` (`store.go:33`), so a new key already gets a fresh
counter for free, with zero changes to `Store`. The alternative (one recurring key, reset yearly) would
need a season-ID storage-key suffix and a migration step every rotation; skipped, YAGNI — nothing here
needs a single key to persist across years, and past occurrences staying visible in a player's history
(see below) is actually the more desirable behavior, not a limitation to work around.

### Gating progress: one check, inside the shared `bump`/`bumpBy` helpers

`RecordHand` (`service.go:48-82`) defines `bump`/`bumpBy`/`streak` closures that every one of the ~40
explicit trigger call sites in the function goes through. Adding the season check **inside those three
closures** (not at each of the ~40 call sites) means every future seasonal achievement is
automatically window-gated with no per-call-site code:

```go
bumpBy := func(playerID, key string, by int) error {
    if achievement, ok := achievementForKey(key); ok && !achievement.Season.ActiveAt(time.Now()) {
        return nil // outside the window: no progress, no unlock, not an error
    }
    previous, current, err := s.store.Increment(ctx, playerID, mode, key, by)
    // ... unchanged
}
```

(`achievementForKey` already exists in `store.go:184-191` as a package-private helper; it's called
from `Store.ListAchievements` today — reuse it here too rather than duplicating the Catalog scan.)
Same one-line guard added to `streak`'s body. `time.Now()` is used directly, consistent with the rest
of `RecordHand` being called synchronously off the real hand-completion event, not a replayed batch.

### New seasonal achievement = same steps as any new achievement, plus a window

To ship e.g. "Halloween Bluffer" (win 5 hands without showdown during a one-week window):

1. Add `KeyHalloweenBluffer2026` + a `Catalog` entry with `Season: &Season{ID: "halloween-2026", Start: ..., End: ...}`.
2. Add one explicit `bump(id, KeyHalloweenBluffer2026)` call next to the existing `bump(id, KeyBluff)`
   call it piggybacks on (`service.go:76`) — same trigger condition, second key.
3. Add a `DESCRIPTIONS`/`EXAMPLES` entry in `ui/src/lib/achievements.ts`, plus a `SEASONAL` entry:

```ts
// ui/src/lib/achievements.ts, mirroring MODE_ONLY (lines 52-55)
const SEASONAL: Record<string, {label: string; endsAt: string}> = {
  halloween_bluffer_2026: {label: 'Halloween 2026', endsAt: '2026-11-01'}
};
export function achievementSeason(key: string) { return SEASONAL[key]; }
```

No server response shape change: the client already has to hand-curate label/description/example per
key (there is no server-driven achievement metadata endpoint — `PlayerAchievementProgress` is just
`{key, count}`, `store.go:151-154`), so a client-side seasonal flag costs nothing new and needs no API
version bump.

### After the window closes

Nothing deletes the catalog entry or the player's earned tiers — `Catalog` keeps every past season's
entry forever (same as every permanent achievement never being removed), so a player who unlocked
"Halloween Bluffer 2026" keeps seeing it (with its stars) on their achievements page indefinitely; the
season check only blocks *new* progress after `End`. `ListAchievements` needs no change: it already
returns every progress row that exists for the player regardless of catalog freshness.

### UI: badge, not a new page

`achievementSeason(key)` returning non-undefined is enough for the achievements page/toast to render a
small "sazonal · encerra em 01/11" badge next to the existing star display — additive to whatever
`AchievementCard`/`AchievementToast` already renders per key, no new route, no new component tree.

## Testing

- `catalog_test.go` (new cases in the existing `cards_test.go`/`service_test.go` files, not a new
  file — matches how `achievements` already organizes its tests): `Season.ActiveAt` boundary cases
  (before start, exactly at start, exactly at end (exclusive), after end, `nil` season always active).
- `service_test.go`: a seasonal achievement's `bump` is a no-op (no `Increment` call, no `TierUnlock`)
  when `RecordHand` runs outside its window, and behaves exactly like a permanent achievement inside
  it — reuse the existing table-driven test shape in that file rather than a new test harness.
- `ui/src/lib/achievements.test.ts` (if it exists — verify before assuming; if not, add alongside
  whatever tests already cover `achievementDescription`): `achievementSeason` returns the badge data
  for a seasonal key and `undefined` for a permanent one.

## Out of scope

- Any UI features from `ui/CLAUDE.md`'s "Not built" list — not touched, not relevant here.
- A server-driven achievement metadata/catalog endpoint — the client already hand-curates per-key
  copy; introducing one would be a much bigger change unrelated to seasons and isn't needed to ship
  this.
- Automatic season scheduling/rotation tooling (e.g. a cron that flips `Start`/`End` or auto-generates
  next year's key) — each season is a small, deliberate code change (new key, new dates, new
  description) the same way every existing achievement was added by hand; building automation for
  something that happens a few times a year is premature.
- Retroactively crediting progress made during the window if the code deploys late — out of scope;
  `ActiveAt` uses wall-clock `time.Now()` at the moment the hand completes, same as every other
  real-time counter in this service.
