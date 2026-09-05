# One purchase lifecycle, and a poll that stops in a hidden tab (#227)

Three things can be bought: sandbox credits (`PurchaseModal`), premium reactions
(`ReactionPurchaseDialog`) and deck/felt cosmetics (`CosmeticPurchaseDialog`). Two of them armed a
bare `window.setInterval` — 5s and 4s — which keeps firing while the tab is hidden: a pending Pix
left open in a background tab spent roughly 720 and 900 GETs per hour on a purchase nobody was
looking at. The cosmetic dialog already read its status through React Query, which is why it did
not have the problem; it did, however, keep polling forever after the fetched status turned
`confirmed`, because its `enabled` looked at the *created* row and its `refetchInterval` was a flat
number.

## The shared hook

`lib/hooks/usePurchaseStatus.ts` is now the single lifecycle for all three:

- **The websocket is primary.** Every status query key lives under `['wallet', …]`
  (`sandboxPurchaseKey`, `reactionPurchaseKey`) or `COSMETIC_PURCHASE_QUERY_ROOT`, which is exactly
  what `sandbox_purchase_update` / `reaction_purchase_update` / `cosmetic_purchase_update` already
  invalidate in `useLobbyRealtime`. A delivered frame resolves the dialog on the frame.
- **The poll is the bounded fallback.** Living in the query cache is what pauses it in a hidden
  tab: TanStack's `refetchIntervalInBackground` defaults to `false`, so the interval simply does not
  fetch while the document is hidden, and the app-wide `refetchOnWindowFocus` gives **one** catch-up
  read when the tab comes back — one, not a backlog, because the hook pins `staleTime` to the base
  gap.
- **Backoff.** `nextPurchasePollMs(spent, remainingMs)`: 4s while fresh, doubling every 5 polls up
  to a 30s ceiling. It is driven by polls *spent*, not elapsed time — the same unit the budget is
  in, no clock read during render, and a tab that spent an hour hidden (polling nothing) comes back
  at the gap it left off on instead of at the ceiling.
- **Deadline.** `false` — stop — as soon as the latest known status is no longer `pending`/
  `processing`, or the purchase's own Pix window has closed, or `PURCHASE_POLL_BUDGET` (40) polls
  have been spent. So a pending purchase costs at most 40 fallback reads ever, against ~900/hour
  before.
- **No read at open.** `initialData` is what the caller already knows (the create response, or the
  row the store page holds), so opening a dialog spends nothing; the first fallback read is one gap
  later.

`purchasePollCount()` (with `resetPurchasePollCount()` as the test seam) counts the fallback reads
the session actually spent. Deliberately not a beacon: `lib/telemetry.ts` is the client-*error*
sink and a poll is not an error, so the counter exists to be asserted in the tests and read from
the console while watching a live purchase.

## What the player sees

Nothing new, and nothing sooner or later than before while the tab is in front of them. The
recovery affordance is unchanged in both dialogs — a failed status read still shows "Não foi
possível atualizar a confirmação" with a manual re-check — and it is now wired to the query's own
`refetch`/`isFetching`, so the button reports "Verificando…" while the check is in flight in the
reaction dialog too, matching the credits modal.

Deliberately *not* added: a "we stopped checking" state. Every path that can outlive the poll
already ends in an honest UI of its own — a Pix that expires renders the expired branch with
"Gerar novo Pix", and a `processing` fichas debit says the window can be closed and the purchase
will appear in the history. Inventing a new banner for the deadline would be a fourth lifecycle,
which is what this change is removing.

## Tests

- `lib/hooks/usePurchaseStatus.test.tsx`: the pure backoff/deadline function; no read at open; the
  4s base gap; nothing while `focusManager` says the tab is hidden and exactly one read on coming
  back; stopping on a non-waiting status, on a closed Pix window and on a failed read that recovers.
  It runs against `createQueryClient()`, so the assertions are made against the defaults the app
  actually ships.
- `PurchaseModal.test.tsx` and `reactionStore.test.tsx` each assert the hidden-tab budget through
  the real dialog, so the two former `setInterval` sites cannot regress independently of the hook.
