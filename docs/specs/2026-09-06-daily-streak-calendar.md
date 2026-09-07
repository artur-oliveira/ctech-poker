# Daily streak calendar with one-day protection (#293)

**Status:** implemented · **Date:** 2026-09-06 · **Issue:** [#293](https://github.com/artur-oliveira/ctech-poker/issues/293)
(split from #215)

## What changed

The daily sandbox-credit spin was a weighted random draw (5 tiers, 5k–100k) with no memory: one
`poker_daily_reward` row per `(playerID, day)`, TTL 48h, nothing linking one day to the next. It is
now a **30-day streak trail** — deterministic per streak day, with a one-day protection — plus the
calendar the client renders.

### Reward trail (`api/internal/dailyreward/streak.go`)

`trail[30]`, in sandbox chips, indexed by `CycleDayFor(streak) = ((streak-1) % 30) + 1`:

| Days | Values |
|---|---|
| 1–6 | 5k · 7,5k · 10k · 12,5k · 15k · 20k |
| **7** | **50k** (weekly chest) |
| 8–13 | 25k · 27,5k · 30k · 32,5k · 35k · 40k |
| **14** | **100k** |
| 15–20 | 45k · 50k · 55k · 60k · 65k · 70k |
| **21** | **250k** |
| 22–29 | 80k · 90k · 100k · 110k · 120k · 130k · 140k · 150k |
| **30** | **1.000.000** — the grand prize |

**The trail restarts at day 1 after day 30** (product decision, 2026-09-06): the absolute streak
keeps counting (and `best_streak` records it), so a months-long streak stays rewarding without an
unbounded prize curve. `FirstAward` (100k) is unchanged and still overrides day 1 on a player's
very first claim ever.

`pickTier`/`tiers` are deleted. A calendar whose day N advertises a value and then pays a random
one is not a calendar.

### Streak state

One extra item per player in the **existing** `poker_daily_reward` table: `pk = playerID`,
`sk = "streak"`, carrying `current_streak`, `best_streak`, `last_claim_day`,
`protection_available`, `protection_used_day`, `total_claims`. **No TTL** — the day rows expire
after 48h, the streak row is the only durable history of them. No new DynamoDB table, no new GSI.

Progression (`advance`, a pure function so a retry can decline to recompute it):

- gap of 1 calendar day (BRT, the same `cooldownKey` the claim already used) → `current_streak++`
- gap of 2 **and** a protection available → `current_streak++`, protection consumed,
  `protection_used_day` records the day it covered
- anything else (first claim ever, gap ≥ 2 without protection, gap ≥ 3) → reset to 1
- reaching a multiple of `ProtectionGrantEvery` (7) grants one protection

### Write cost

Still **one transaction per player per day**, now with two items instead of one: the create-only
day row plus a plain put of the streak row. The day row's `attribute_not_exists` condition guards
the whole transaction, so a duplicate claim aborts the streak write too — a streak can never
advance twice for one day, which is what makes the wallet-credit retry path safe.

`Store.IsFirstReward`'s `Query` is **gone**: "has this player ever claimed" is now
`streak.TotalClaims == 0`, read from the same `GetItem` the calendar needs. `Service.Status` is one
`GetItem` and serves the cooldown, the streak and all 30 calendar slots — the cooldown endpoint got
cheaper, not more expensive.

### API

Both endpoints keep their paths, their auth and their rate limits. `remaining_time_seconds` (and
`POST`'s `amount`) are byte-for-byte what they were, so a client that reads only those is
unaffected — the streak fields are purely additive, which is the issue's compatibility criterion.

`GET /v1.0/sandbox-credits/` now returns:

```json
{
  "remaining_time_seconds": 0,
  "current_streak": 3, "best_streak": 9, "total_claims": 3,
  "cycle_day": 4, "cycle_length": 30,
  "protection_available": false, "claimed_today": false, "streak_at_risk": true,
  "days": [{"day": 1, "amount": 5000, "milestone": false, "claimed": true, "today": false}, "…30 slots"]
}
```

`POST /v1.0/sandbox-credits/` returns the same envelope with `amount` added, so the client repaints
the trail from the claim response instead of issuing a follow-up `GET`.

`cycle_day` is always the slot the **next** claim lands on: before today's claim it is derived by
running `advance` against today's date, so an unclaimed day renders as the pending slot rather than
as yesterday's position. No separate `/sandbox-credits/calendar` route was added — the issue offered
it as optional and the calendar costs the cooldown read nothing extra.

## Frontend (`ui/`)

- `DailyRewardPanel` is now a **teaser**: flame badge with the streak count, "Dia N de 30", today's
  exact prize, and a button that opens the trail. It reuses `.store-reward`'s existing responsive
  geometry rather than introducing a second layout for the store's first row.
- `DailyStreakDialog` is the expanded reward window: a 760px dialog with the current/best streak and
  protection state, the 30-day grid (7 columns on desktop, 4 below 600px), and the day-30 grand
  prize as a full-width chest rather than a 30th identical cell. The claim happens here.
- Claimed slots are gold (earned value), the claimable day is oxblood (the primary commitment) with
  the only authored motion on the surface — a 2.4s ring breathe, dropped entirely under
  `prefers-reduced-motion`. Every cell carries a `sr-only` sentence naming its day, value and state,
  so the trail is not read by colour.
- `RebuyDialog`'s in-table emergency claim seeds the whole status object into the
  `['dailyReward','cooldown']` cache instead of only the cooldown, so the store's trail never reads
  a half-populated entry.
- `mockRuntime` mirrors the same trail, so `npm run dev:mock` renders the real shape.

## Tests

`api/internal/dailyreward/service_test.go` covers streak advance, protection absorbing exactly one
missed day, reset past that, the weekly protection grant, the 1M finale and its restart, the
`Status` pending/claimed calendar, and — unchanged from before — that a wallet or completion failure
retries with the same amount and idempotency key *without* advancing the streak twice.
`ui/src/app/(app)/store/page.test.tsx` claims from inside the dialog and asserts the grand prize row.
