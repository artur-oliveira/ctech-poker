# Short-deck sandbox variant (#296) and structured `next_action` / `retry_after_seconds` (#319)

## #319 — `next_action` / `retry_after_seconds` in problem+json

`gopkg.aoctech.app/api-commons` v1.11.0 (already the pinned version) ships the optional RFC 9457 extension members
`next_action` (`retry`, `wait`, `reauthenticate`, `contact_support`) and `retry_after_seconds` on `problem.Problem`,
so no shared-package change was needed; this change only populates them where poker knows the answer:

- `rateLimit` middleware (`internal/api/v1/ratelimit.go`) now answers 429 with problem+json via
  `problem.TooManyRequests(window)`: `next_action=retry`, `retry_after_seconds` = the limiter's fixed window (not a
  per-request remaining time; neither backend tracks that). Replaces the old ad-hoc `{"error":"rate_limit_exceeded"}`.
  Other hand-rolled 429s (`social.go` invitations) are out of scope.
- `problem.Unauthorized` carries `next_action=reauthenticate`; `problem.TableFull` carries `next_action=retry`.
- `walletclient.Error` decodes the same two members; `problem.FromWalletError` propagates them verbatim when present
  and leaves them unset otherwise.

### Frontend: the shared HTTP client now reads and acts on both fields

`ui/src/lib/api/client.ts` (`ApiProblem`/`ApiError`) previously only read the HTTP `Retry-After` *header* — which
none of poker's own responses ever set — and had no notion of `next_action` at all, so the fields above reached the
browser and were silently dropped.

- `ApiProblem` gained typed `next_action`/`retry_after_seconds`; `ApiError.nextAction` is a convenience accessor.
- `retryAfterMsFrom` centralizes the precedence: the HTTP header wins when present (a generic intermediary might set
  it), the problem+json body's `retry_after_seconds` is the fallback — used both by `normalizeApiError` (so any
  caller reading `error.retryAfterMs` gets the server's real number) and by the axios response interceptor's own
  automatic retry on 429/5xx (`httpRetryDelay`), which previously guessed an exponential backoff whenever no header
  was sent and now waits the rate limiter's exact window instead.
- `lib/notify.ts`'s `notifyApiError` (the app-wide error toast) now shows the exact wait time for `retry`/`wait`, and
  a "contact support" message for `contact_support`, only when that adds information the generic status message or
  the server's own `detail` didn't already have — `reauthenticate` needs no separate handling here, since every
  server response setting it is already a 401, which `client.ts`'s interceptor already silently refreshes/redirects
  through its own existing logic (unrelated to `next_action`).
- The table's own WebSocket protocol carries no problem+json (`tablews.go`'s errors are a small closed set —
  `invalid_action`/`unavailable`/resync codes — already handled by `lib/tableResilience.ts`), so there's nothing
  there for `next_action` to plug into; this is HTTP-only by construction.

## #296 — short-deck (6+ hold'em), sandbox only

Isolation is by construction: the standard 52-card path (`deck.ShuffleResult`, `RootCommitHash`, `handeval` tables,
`hand.Table.shuffle`) is untouched.

- `deck`: `Variant` (`Standard`, `ShortDeck`), a separate `ShortDeckShuffleResult` ([36]Card) and
  `NewShortDeckShuffle`, plus `RootCommitHashN` for non-52 decks.
- `handeval/shortdeck`: combinatorial (C(7,5)) evaluator; flush ranks above full house, A-6-7-8-9 is the low straight.
  Ceiling: no perfect-hash table (sandbox-only, low volume) — generate one like `handeval/gen` if it ever matters.
- `hand.Table`: `variant` field, `NewTableWithVariant` (`NewTable` = Standard). `dealCard`, showdown and snapshot
  evaluation go through `Table.best7` / `Table.categoryOf`. A short-deck table leaves `shuffle` nil, so the existing
  `t.shuffle != nil` guards skip fairness proofs.
  Known gaps: no fairness proof/commit hash for short-deck hands; `partialCategory` (opponent hint from revealed
  cards) still uses standard category order.
- `roomstore.Room.Variant` (`""` | `"short_deck"`, written once at creation, no per-hand write). `POST /rooms`
  accepts `variant` and rejects it for real-money rooms and for unknown values; `table.SeedForRoom` also ignores it
  for non-sandbox rooms. `join-or-create` does not expose variants.
- `hand.State` persists `Variant` and `ShortShuffle` (both omitempty, so standard tables' persisted state is
  unchanged), so a mid-hand reload on another instance keeps dealing from the same 36-card shuffle.
- Tests also cover a full 9-handed hand (`TestShortDeckNineHandedTablePlaysAFullHandWithoutExhaustingTheDeck`) and a
  9-way all-in run-it-twice hand (`TestShortDeckNineWayRunItTwiceFitsInTheReducedDeck`, 34 of 36 cards consumed,
  mirroring the standard-deck `TestRunItTwiceNineWayPreflopFitsInTheCommittedDeck`) — both assert every dealt card
  (board, board-two, and every participant's hole cards) has rank >= Six.
- Not done: the spec-approval step (issue checklist item 1).

### Frontend — the ranking-correctness blocker is now fixed, plus the room-creation selector

An audit of `ui/` (grepping for `Flush`/`FullHouse`/`HandCategory`/`categoryOf`/`variant` across `src/`) found the
gap was wider than "the opponent category hint": `ui/src/lib/pokerRules.ts` is a second, independent hand evaluator,
hardcoded to standard Texas Hold'em rules (full house ranked above flush, no A-6-7-8-9 straight), and every
post-hand/history/highlight surface recomputed hand strength from raw cards through it with no notion of variant at
all — a display-correctness bug, not a missing feature, on a short-deck table.

**Decision: fix in the client, not a new backend evaluator endpoint.** A `POST /hand-eval/best`-style endpoint
(hole+board+variant in, category/score out) was considered — it would make the backend the single source of truth
for any future consumer, at the cost of a network round trip. Rejected for the two things it would land in the
critical path:
- `HandOutcome`/`LastWinners`/`tableOutcome.ts`'s live post-hand banner would need one round trip mid-render for
  every showdown — directly against PRODUCT.md's "Speed is part of trust: input, dealing, betting, and reconnecting
  must feel immediate," and the exact area (`docs/2026-09-04-table-render-budget.md`'s render/latency work) this repo
  has already spent real effort tightening.
- `TodayHighlight`'s `bestShownLabel` reduces over N candidate hands to find the best one — O(N) sequential network
  calls in a `reduce`, not O(1).

The client already ports a second copy of the standard evaluator (documented in `bestFiveCardHand`'s own comment:
"the server only ever sends a category label... never the resolved 5-card hand itself"), so extending it with a
`variant` parameter is the same accepted trade-off doubled, not a new one — pure, deterministic, ported line-for-line
from the tested `handeval/shortdeck` Go package, with its own differential test.

**What shipped:**
- `pokerRules.ts` gained `HandVariant = 'standard' | 'short_deck'`, threaded (default `'standard'`, so every existing
  call site is byte-for-byte unaffected) through `bestFiveCardHand`/`bestHandCategory`/`compareHands`/
  `wasDecidedByKicker`: a short-deck rank table (flush above full house) and A-6-7-8-9 straight detection/ordering,
  mirroring `handeval/shortdeck` exactly.
- **`HandOutcome.tsx`'s real bug, independent of variant support:** `categoryFor` recomputed a category from raw
  cards *unconditionally* whenever exactly 5 were resolved, ignoring the server-sent `handCategory`/`opponentCategory`/
  `beatenCategory` fallback every call site already threaded in — so the server's own correct (variant-aware) label
  was discarded even before this change. Fixed to prefer the server label and only fall back to a local recompute
  (now variant-aware too, via a new `HandOutcomeState.variant`) when the server didn't send one.
- `tableOutcome.ts`'s `buildHandOutcome`/`resolvedHand` (which *must* recompute — the server never sends the
  resolved 5-card hand) now takes a `variant` parameter, sourced in `page.tsx` from `room.variant` and threaded
  through `useTableOutcome`.
- `TodayHighlight` takes a `variant` prop (same `room.variant` source) for its cross-player `compareHands`/
  `bestHandCategory` calls (`highlightWinnerLabel`/`bestShownLabel`/`madeHandOf`).
- `LastWinners.tsx` and the two `/hands` pages read **the hand's own recorded variant**, not the room's current one:
  backend `hand.HandOutcome` gained a `Variant` field (`deck.Variant.Label()`, same reasoning as the existing
  `SmallBlind`/`BigBlind` capture — a room's variant is immutable for its lifetime, but the room record a
  much-later history reader fetches may no longer exist), propagated into `sessionlog.HandItem.Variant`
  (`internal/app`'s `handItemForWithAvatars`) and the frontend `HandItem.variant` type. Both are `omitempty`, so a
  standard hand's persisted/wire shape is unchanged.
- `roomstore.Room.Variant`/`POST /rooms` already existed (see above); `CreateRoomDialog.tsx` now has a "Short-deck
  (6+)" checkbox, shown only in sandbox mode, cleared automatically when switching to real money, and documented in
  the in-app guide (`(marketing)/guide/basics`).
- **What was already correct and untouched:** `Seat.tsx`'s live in-hand category hint renders the server-sent
  `seat.hand_category` directly, never recomputing — no bug there.
- **Still not done:** `partialCategory` (the backend's opponent-hint helper for 0-2 revealed hole cards) still uses
  standard category order server-side (documented above); `lib/equityTrainer.ts` is a standalone offline practice
  tool with no live table/room behind it, deliberately left standard-only.
