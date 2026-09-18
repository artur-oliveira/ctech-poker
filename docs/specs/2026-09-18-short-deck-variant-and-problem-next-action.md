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

### Frontend gap — BLOCKING before short-deck is exposed to real users

`#296` is `module:backend`; no frontend work was in scope, so none was attempted. But an audit of `ui/` (grepping for
`Flush`/`FullHouse`/`HandCategory`/`categoryOf`/`variant` across `src/`) found the gap is **wider than "the opponent
category hint,"** and it is a display-correctness bug, not a missing feature, if short-deck ships without a frontend
fix:

- `ui/src/lib/pokerRules.ts` is a **second, independent hand evaluator**, hardcoded to standard Texas Hold'em rules:
  `HAND_RANK_INDEX`/`HAND_RANKINGS` order full house above flush, and `bestFiveCardHand`'s straight detection has no
  A-6-7-8-9 case. It takes raw hole+board cards and recomputes category/best-5-cards/comparisons from scratch — it
  never reads a server-sent category, and nothing anywhere in `ui/` threads a room or hand variant into it (the only
  other "variant" in the codebase, `useDeckVariant`/`DeckVariantId`, is the unrelated cosmetic card-back skin).
- Every call site of `bestFiveCardHand` / `bestHandCategory` / `compareHands` inherits this on a short-deck table:
  - `src/lib/tableOutcome.ts` (`bestFiveCardHand`) — picks/orders the 5 cards shown in the live post-hand outcome
    banner. On a short-deck flush-vs-full-house hand it would show the wrong 5-card combination as "the winning hand."
  - `src/components/table/HandOutcome.tsx` (`bestHandCategory`, `wasDecidedByKicker`) — the post-hand banner's
    category label and "won by kicker" framing.
  - `src/components/table/LastWinners.tsx` (`bestFiveCardHand`, `bestHandCategory`) — the last-winners strip.
  - `src/components/table/TodayHighlight.tsx` (`bestHandCategory`, `compareHands`) — "maior pote de hoje": both the
    category label and the cross-hand comparison used to pick the highlighted hand would rank a short-deck flush
    below a full house, backwards from what the server actually paid out.
  - `src/app/(app)/hands/page.tsx` and `src/app/(app)/hands/history/page.tsx` (`bestHandCategory`) — hand history
    list/detail category labels, recomputed client-side from cards rather than reading the server's category.
  - `src/lib/equityTrainer.ts` — a standalone offline practice tool with no live table/room behind it; standard-only
    is arguably correct here and it is **not** part of this gap.
- **What is already correct:** `src/components/table/Seat.tsx` renders the *live*, mid-hand per-seat category
  (`seat.hand_category`) straight from the server-sent label (`hand.Table.categoryOf` on the backend) and does not
  recompute it — so the in-hand seat hint is fine. The gap is specifically the post-hand/history/highlight surfaces
  that re-derive category and "best hand" from raw cards instead of trusting the server.
- Also missing entirely: the frontend has no `variant` field anywhere for a room/table/hand (confirmed by grep), no
  UI for choosing short-deck at room creation, and no messaging that ranks 2-6 don't exist in a short-deck table's
  deck.

**This is an explicit blocker before short-deck is exposed to real users**, not a nice-to-have: a table that pays out
correctly on the backend but shows the wrong winning hand/category to players is worse than not offering the variant
at all. Recommended follow-up (not done here, out of `#296`'s backend scope): either (a) make the above five call
sites prefer a server-sent category/board-cards field when present and fall back to `pokerRules.ts` only for
standard hands, or (b) thread a `variant` through `pokerRules.ts`'s ranking table and straight detection so it can
answer correctly for both rule sets. Do not enable short-deck room creation in the UI until one of these lands.
