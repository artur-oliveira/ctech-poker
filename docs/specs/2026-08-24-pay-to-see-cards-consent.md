# Pay-to-see-winner-cards: optional consent — Design

## Status

**Implemented 2026-08-24 as option (B)**, after product picked it over (A) and (C). What shipped
differs from the sketch below in one place worth knowing: the live fee moves *table chips*
(`requester.Stack` / `winner.Stack` / `rakeCollected`), not wallet balances, so the refund path is a
plain stack reversal — no `poker_pending_cashouts` entry is involved. The consent window is 8s
(`hand.WinnerCardsConsentWindow`), chosen to fit inside `table.NextHandDelay`'s 12s post-hand
window; `StartHand` refunds anything still unanswered as a backstop.

The rest of this document is the original design. It was written before implementation — the
current non-consensual behavior it describes matches its own specs exactly
(`docs/specs/2026-08-21-pay-to-see-winner-cards-live.md`,
`docs/specs/2026-08-21-pay-to-see-winner-cards-history.md`): any dealt-in opponent pays a fixed
fee and is shown the winner's cards immediately, with no notice to or acceptance from the winner
— deliberately modeled on Rabbit Hunt (`docs/specs/2026-08-21-paid-rabbit-hunt.md`), where the
"secret" being paid for belongs to the table/deck, not to another player.

A player review of a HAR capture expected the winner to be able to accept or decline being paid
to reveal their cards. That expectation conflicts with the shipped design, not a bug in it.
Confirm the desired behavior before building this — it's a product call, not a technical one.

## Why this needs a decision, not just a toggle

The paid Rabbit Hunt precedent doesn't transfer cleanly: the runout cards are house/deck
information nobody "owns," so nobody's consent is meaningfully at stake. Winner cards belong to
another player. If consent should gate this, the fee's whole value proposition changes: the
payer commits chips (and, at showdown time, gets the winner's cards revealed) *before* knowing
whether the winner will agree. Product needs to pick one of:

- **(A) Keep it non-consensual** (current behavior) — reject the report's premise, close as
  working-as-designed, maybe just make the "pay to see" copy clearer in the UI that this is a
  unilateral pull, not a request.
- **(B) Consent required, refund on decline** — payer is charged, request sent to the winner; on
  accept, cards reveal (existing mechanics fire); on decline or timeout, the charge reverses and
  nothing is revealed.
- **(C) Consent required, cards escrow without charging until accept** — no chips move until the
  winner accepts, removing any need for a refund path, at the cost of a genuinely new
  request/accept/timeout state machine before any of the current charge/reveal code runs.

The rest of this doc sketches **(B)**, since it reuses the current charge/reveal step largely
as-is (only wrapping it in a pending gate) rather than needing a new pre-payment escrow. Revisit
if product picks (C).

## Scope

Live tables only (`Table.RequestWinnerCards`, `api/internal/table/actor.go`'s
`handleRequestWinnerCards`). The hand-history variant
(`internal/handreveal`, `POST /players/me/hands/:handId/reveal-winner`) reveals a *past*, already
-archived hand — there is no "the winner" still at the table to ask, so it is out of scope and
should keep its current non-consensual, synchronous behavior regardless of what live tables do.

## Sketch (option B)

### State

`Table` gains a per-hand `pendingWinnerCardsRequest *WinnerCardsRequest`:

```go
type WinnerCardsRequest struct {
    RequesterID string
    WinnerID    string
    Fee         int64
    ExpiresAt   int64 // unix ms; server-set, e.g. now + 20s
}
```

Only one outstanding request per hand — a second `RequestWinnerCards` call while one is pending
returns a "request already pending" error rather than queuing.

### Flow

1. `RequestWinnerCards(requesterID)` — same eligibility checks as today (stage/showdown,
   single-winner invariant, dealt-in, not already voluntarily shown, no double-request) — but
   instead of charging+revealing immediately, debits into hold (or just validates funds without
   moving them, see below) and sets `pendingWinnerCardsRequest`. Broadcasts to the winner's
   `ViewFor` only: an in-band prompt with `RequesterID`/`Fee`/`ExpiresAt`.
2. Winner responds `AcceptWinnerCardsRequest` / `DeclineWinnerCardsRequest`, or the request
   expires.
   - **Accept:** proceed with the existing charge (if not already held) + reveal logic
     unchanged; clear `pendingWinnerCardsRequest`.
   - **Decline / expire:** refund if already charged; clear `pendingWinnerCardsRequest`; notify
     the requester ("declined"/"expired") — no reveal.
3. Whichever happens, `pendingWinnerCardsRequest` is also cleared on hand transition
   (`StartHand`) as a backstop — a request must not survive into the next hand.

### Money ordering

Following this repo's convention (`api/CLAUDE.md`: "debit-then-seat on buy-in,
remove-then-credit on cash-out... keep new money paths in that shape"): debit the requester when
the request is sent (matches today's already-shipped charge point), credit back on
decline/expire. A failed refund credit goes to the same `poker_pending_cashouts` retry table
already used elsewhere in this codebase (`buyin`), not a new bespoke retry mechanism.

### Frontend

- `WinnerCards.tsx` (requester side): button becomes "Pedir para ver a mão de {winner} por {fee}
  fichas", with a pending state ("Aguardando resposta…") while a request is outstanding, and a
  toast on decline/expire (chips refunded).
- New winner-side prompt component, shown only to the winner when `pendingWinnerCardsRequest` on
  their `ViewFor` names them: "{requester} quer pagar {fee} fichas para ver sua mão." with
  Accept/Decline buttons and a visible countdown to `ExpiresAt`.

### Open questions for product — resolved at implementation time

- **Fee visibility to the winner before they decide:** shown. The prompt names the fee and that
  the winner keeps half, because a decision about your own cards with the price hidden is not a
  real choice.
- **Cooldown on decline:** none. The per-hand `winnerCardsPaid` guard already stops a requester
  from paying twice for the same hand, and a request only lives inside the post-hand window.
- **Does the winner see who asked:** yes — `WinnerCardsRequestView.RequesterName`. Anonymising it
  would make the prompt impossible to reason about socially.

## Out of scope

- Hand-history reveal (`internal/handreveal`) — stays as-is per "Scope" above.
- Real-money mode — this feature is sandbox-only today (see the two existing specs); this design
  doesn't change that gate.
