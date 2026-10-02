# Paid reveals on the table (protocol 12)

Status: implemented on `feat/paid-reveals-on-table`.

Moves both post-hand paid reveals off floating asides and onto the table elements they reveal, and
reshapes their backend so the new UI is honest about what is bought.

## 1. Pre-flop board glow

An empty board readies all three flop slots (`.board-slot.is-next`), because the flop lands three
cards at once. From the flop on, only the next slot glows (unchanged).

## 2. Rabbit hunt: per card, small blind, on the board

Same eligibility as before: sandbox table, hand complete, won without showdown, board < 5, deck
commitment published, viewer dealt in.

- **Price:** one small blind (`Table.RabbitHuntCardFee`, the current hand's escalated SB) per card.
  The snapshot carries it as `rabbit_hunt_fee` only while the hunt is on offer (0 on real-money
  tables, which also hides the UI there).
- **Request:** `request_rabbit_hunt` + `board_slot` (0-4). Purchases are tracked per player as a
  slot bitmask (`Table.rabbitHuntSlots`, persisted as `State.RabbitHuntSlots`). A slot already dealt,
  out of range or already bought is rejected without charging. A frame **without** `board_slot`
  (pre-12 clients, CLI `/rabbit all`) buys every slot not yet bought, one SB each.
- **Snapshot:** the proof is published to a viewer only once they bought at least one card, and it
  reveals only the bought slots: `revealed_card_salts` gains those deck positions, every other
  position (including unbought runout cards) stays in `unrevealed_card_hashes`.
  `shuffle_server_seed_hex` is never sent for a hand won without showdown (it would reveal every
  card), which `fairnessProofFor` already guaranteed and a test now pins. New `rabbit_cards`
  (`map<int32,string>`, slot → card) tells the client which slot each card belongs to;
  `runout_cards` keeps the same cards in slot order for older clients.
- **Refund:** `rabbit_hunt_verify_failed` + `board_slot` refunds that one card's SB; without a slot
  it refunds every bought card (legacy).
- **Ledger:** unchanged in kind — the fee is debited from the table stack and leaves play, sandbox
  only, exactly as the BB fee did.
- **Client:** `useRabbitHunt` drives each undealt slot through offer → pending → verifying →
  revealed | failed. Verification stays in the browser: the partial-deck proof must rebuild
  `root_commit_hash` **and** the salted card at that slot's deck index must equal
  `rabbit_cards[slot]`. A failure shows "Devolvido" on the slot and reports the refund for that card.
  A revealed card keeps a dashed halo + rabbit mark: it is what would have come, not the board.

## 3. Winner's cards: click the cards, batched consent

Price stays the big blind; the fee still only splits winner/rake on accept.

- **Requester:** the uncontested winner's face-down cards become the button (price chip with an Eye
  under them). After asking, the same spot shows an hourglass + countdown.
- **Batch:** `Table.pendingWinnerCards` is now a slice (`State.PendingWinnerCardsBatch`). A request
  made while a batch is open joins it under the **same** `ExpiresAt` (never extended). Each
  requester pays individually. Accept pays the winner half of every fee and reveals to every
  requester in the batch; a late requester after an accept opens a fresh batch. Decline or timeout
  refunds every requester and sets `winnerCardsClosed`: no further requests that hand (cards stop
  being clickable; `winner_cards_closed` on the snapshot). One request per player per hand still
  holds (`winnerCardsAsked`). `StartHand` refunds any open batch and reopens.
- **Timer:** `armWinnerCardsTimer` keys on `handID#ExpiresAt`, so joiners never re-arm it.
- **Visibility:** `winner_cards_requests` is viewer-scoped — the winner sees the whole batch, each
  requester only their own entry, everyone else nothing. `pending_winner_cards` (field 35) is kept
  populated with the first visible entry for pre-12 clients. Note: `winner_cards_closed` is sent to
  every viewer, so a bystander can infer that *someone* was refused (not who); this is inherent to
  "the cards stop being clickable".
- **Winner prompt:** one prompt anchored above the winner's own seat: up to three requester avatars
  then `+N`, names, total they would receive (Σ ⌊fee/2⌋), countdown, Recusar / Mostrar.

## Compatibility

- `TableProtocolVersion` 11 → 12. The web client gates both new controls on `protocol_version >= 12`
  (an older server keeps them dark rather than half-working).
- Old clients against a new server: `request_rabbit_hunt` without a slot buys the rest of the
  runout (SB per card) and `runout_cards` still carries it; `pending_winner_cards` still drives the
  old prompt, and its accept answers the whole batch.
- Persisted state: `State.RabbitHuntPaid` and `State.PendingWinnerCards` are read-only legacy
  fields, upgraded on load (`legacyRabbitHuntSlots`, `legacyWinnerCardsBatch`) and never written
  again. A state written by a new instance and read by an old one during a rolling deploy loses
  the new fields (purchases invisible, a pending batch's fees not refunded by the old StartHand) —
  accepted for the deploy window.

## Not changed / follow-ups

- `FairnessProofsForActor` (match history) still reveals every undealt rabbit slot for free, as it
  did before this change.
- The client derives the deck index from the count of `dealt_in` seats; a dealt-in player who left
  before the purchase shifts it and makes verification fail (refund), as before.
