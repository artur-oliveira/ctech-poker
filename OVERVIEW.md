# ctech-poker — Product / Functional Spec

## 1. Purpose

A real-time, multi-table Texas Hold'em poker product. Two currency modes:
**sandbox** (play money, no relation to real funds) and **real** (backed by `ctech-wallet`
balance). MVP ships sandbox-complete; real-money mode ships only once its two hard
prerequisites are met — see § 10.

## 2. Rooms

- **Public rooms**: listed in a lobby, anyone can join up to table capacity.
- **Private rooms**: created with a shareable code/link, not listed.
- Room config at creation: stakes (small/big blind), max seats (2–9), sandbox or real,
  buy-in min/max (as a multiple of big blind, standard poker convention).

## 3. Game rules — Texas Hold'em (must be implemented exactly, this is the hard part)

### 3.1 Hand lifecycle
`WAITING_FOR_PLAYERS → PRE_FLOP → FLOP → TURN → RIVER → SHOWDOWN → HAND_COMPLETE`, then
dealer button rotates one seat clockwise and the next hand begins automatically if ≥2 players
have chips.

### 3.2 Per-player states
`ACTIVE`, `FOLDED`, `ALL_IN`, `SITTING_OUT` (voluntary), `DISCONNECTED` (see § 6 — distinct
from sitting out; has a reconnect grace window).

### 3.3 Betting rules (the actual hard edge cases — do not hand-wave these)
- **Blinds**: small blind and big blind posted automatically pre-flop by the two seats after
  the dealer button; heads-up (2 players) is a special case — dealer posts small blind.
- **Minimum raise**: must be at least the size of the previous bet/raise in the same round
  (e.g. bet 100, min raise to 200 — i.e. a raise of at least 100 more).
- **Short all-in does not reopen the action**: if a player goes all-in for *less* than a full
  minimum raise, players who have already acted in that round may call or fold but **may not
  re-raise** — the betting round is not "reopened" by a sub-minimum raise. This is the single
  most commonly-implemented-wrong rule in amateur poker engines; it needs an explicit unit
  test per scenario, not just "make it work for the happy path."
- **Betting round ends** when all active (non-folded, non-all-in) players have acted and all
  outstanding bets are equal.
- **Side pots**: when players go all-in at different stack sizes, split into pot layers.
  Algorithm: sort all-in contribution amounts ascending; each layer's pot is
  `(layer_amount − previous_layer_amount) × number_of_contributors_at_or_above_this_layer`;
  a player is only eligible to win layers up to their own contribution level. This must be
  implemented as a named, independently unit-tested function (`ComputeSidePots`) with test
  cases covering 2-way and 3+-way simultaneous all-ins at different amounts — this is the
  #1 place real-money poker engines have historically had payout bugs.
- **Showdown**: best 5-card hand from 7 (2 hole + 5 board) wins each pot layer it's eligible
  for; ties split the pot layer evenly (odd chip to the player closest to the button, standard
  convention); reveal order follows standard convention (last aggressor shows first, or first
  active player left of the button if no bet on the river).

### 3.4 Hand evaluation
Standard 10-category ranking (high card → royal flush), full 7-card evaluation with kicker
comparison. This is a solved problem (well-known algorithms/lookup-table approaches exist) —
**do not hand-roll a naive evaluator**; use a vetted approach and put it under a large table
of known hand-vs-hand comparisons as a regression test, since a silent mis-ranking is the kind
of bug that only surfaces as "a player is quietly being paid wrong" in production.

### 3.5 Fairness
- Server-authoritative shuffle using a CSPRNG (never `math/rand` unseeded or seeded
  predictably).
- **Commit-reveal for provable fairness** (suggested, not in original brief): before each hand,
  the server commits to a hash of the shuffled deck + a server seed; after the hand, it
  reveals the seed so the shuffle is independently verifiable. This matters specifically
  because this is *real-money* poker — "trust us, the shuffle was fair" is a weak position
  the first time a player disputes a bad beat.

## 4. Resilience & real-time

- **Disconnect handling**: a `DISCONNECTED` player gets a grace window (e.g. 30–60s) during
  which their hand is auto-folded on their next action deadline but their seat is held; beyond
  the grace window (or after N consecutive disconnected hands), auto-sit-out.
- **Server-authoritative state**: the client never computes game outcomes; it renders what the
  server pushes and optimistically previews the local player's own action before
  confirmation. On any disconnect/reconnect, the client re-syncs from a full authoritative
  state snapshot, not from replaying missed deltas (deltas are for the happy path only).
- **Idempotent actions**: a double-submitted "call" (double-click, retry after a dropped ack)
  must not be applied twice — every player action carries a client-generated action id, the
  server de-dupes on `(table_id, hand_id, seat, action_id)`.
- **Crash recovery**: table state must be durably checkpointed after every action (not just
  in memory) so a table can resume — possibly on a different server process — without losing
  the hand in progress. See ARCHITECTURE.md § 3.

## 5. Wallet integration & sandbox isolation

- **Real mode**: joining a real-money table with a buy-in **reserves** funds from the
  player's `ctech-wallet` balance into a table-stake hold, it does not just "check balance and
  hope." Cashing out returns the remaining stack. This must be built as a **hold/capture**
  pattern (reserve on buy-in, capture/release on cash-out), the same pattern real payment
  processors use for exactly this reason — a crash between "chips awarded" and "balance
  debited" must never lose or duplicate funds.
- **Sandbox mode**: uses a completely separate play-money ledger, owned entirely by
  `ctech-poker`, with **no code path** that can convert sandbox chips into real wallet balance
  or vice versa. This separation must be enforced at the data-model level (different tables /
  a `currency_mode` field that every wallet-interaction code path checks and rejects mixing
  on), not just by convention.
- **As of the current `ctech-wallet` audit**: the sandbox credit/debit surface already exists
  and is implemented/tested — safe to build the sandbox mode against today. The real-money
  hold/capture pattern above has **no corresponding wallet endpoint yet** (only sandbox
  routes exist on the wallet side today) and `ctech-wallet`'s DynamoDB tables are currently
  hard-capped at 5 RCU/WCU, which would not survive real poker table traffic. **Real-money
  mode is blocked on both of these being fixed on the `ctech-wallet` side first** — do not
  schedule real-money-mode engineering until they are.

## 6. Frontend

- SPA using `ctech-oauth-client` for auth, matching the rest of the company's frontends.
- Card assets: SVGs provided externally; animations (deal, flip, flop reveal, chip-to-pot,
  pot-to-winner) built in CSS/JS — no animation library dependency needed for this, it's a
  well-bounded set of transitions (YAGNI on a full animation framework).
- Lobby (table list, filters by stakes/mode), table view, buy-in/cash-out flow, basic in-table
  chat.

## 7. MVP scope (as specified by the business)

- Public/private room creation and joining.
- Full Texas Hold'em rules engine: blinds, betting rounds, side pots, showdown, hand
  evaluation, dealer rotation.
- Resilient real-time updates (disconnect/reconnect, crash-recoverable table state).
- Gamified frontend with card animations (SVGs provided externally).
- Resilient wallet integration.
- Sandbox and real modes.

## 8. Suggested features (not in the original brief — flagged as suggestions)

1. **Rake/monetization mechanism.** The brief doesn't say how `ctech-poker` makes money. The
   standard model is a rake (a small % of each real-money pot, capped at a max per hand) taken
   into a house account. Without this designed in from the start, real-money mode has no
   revenue model — flagging this as an open product question, not assuming an answer.
2. **Hand history log**, queryable per player, independent of the live table state — needed
   for dispute resolution ("I got a bad beat, prove the shuffle was fair") and is cheap to add
   if commit-reveal (§ 3.5) is already in place.
3. **Provable-fair shuffle (commit-reveal)** — see § 3.5.
4. **Chat moderation** (basic profanity filter + report/mute) — real-money player-vs-player
   chat without any moderation is a support-ticket generator from week one.
5. **Table themes / cosmetics** — post-MVP delighter, explicitly not a priority now.

## 9. Explicitly out of scope for MVP
- Tournaments (multi-table, blind escalation, prize pools) — cash-game tables only for MVP.
- Spectator mode.
- Run-it-twice / rabbit hunting.
- Mobile native apps (responsive web only).

## 10. P0 non-technical risk — read before building real-money mode

Real-money online poker is in a **legally ambiguous position under Brazilian gambling
regulation** (Brazil's 2023–24 legal-betting framework (Law 14.790) covers fixed-odds sports
betting and some online games explicitly, and it does not obviously and cleanly cover
peer-to-peer poker; the space is actively being litigated/regulated). This is a business/legal
risk, not an engineering one, and it is **bigger than any technical risk in this document**.
Do not treat "sandbox mode first, real-money mode later" as purely a technical phasing
decision — get a real legal opinion on real-money poker's legality/licensing requirements in
Brazil before committing engineering time to real-money mode specifically. Sandbox-only poker
carries none of this risk and can proceed regardless.
