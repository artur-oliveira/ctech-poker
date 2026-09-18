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
- Not done: frontend rendering and the spec-approval step (issue checklist item 1).
