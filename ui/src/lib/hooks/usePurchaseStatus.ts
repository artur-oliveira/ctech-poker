'use client';
import {useRef} from 'react';
import {useQuery, type QueryKey} from '@tanstack/react-query';

// One lifecycle for all three purchase kinds (sandbox credits, premium
// reactions, deck/felt cosmetics). The websocket is the primary confirmation
// path: `sandbox_purchase_update`, `reaction_purchase_update` and
// `cosmetic_purchase_update` all invalidate the roots these keys live under, so
// a delivered frame resolves the dialog on the frame, not on the next poll.
// This poll is the bounded fallback for a dropped or missed frame.
//
// Two of the three used to arm a bare `window.setInterval`, which keeps firing
// in a hidden tab: a pending Pix left open in a background tab spent ~900
// (4s) / ~720 (5s) GETs per hour on a purchase nobody was looking at. Living
// in the query cache fixes that by construction — TanStack's
// `refetchIntervalInBackground` defaults to false, so the interval simply does
// not fetch while the document is hidden, and the global
// `refetchOnWindowFocus` gives exactly one catch-up read when the tab comes
// back (`staleTime` below is what keeps it to one).

/** First fallback gap, while the purchase is fresh. */
export const PURCHASE_POLL_MS = 4000;
/** Ceiling the backoff grows to. */
export const PURCHASE_POLL_MAX_MS = 30_000;
/** The gap doubles once per this many polls already spent. */
export const PURCHASE_POLL_BACKOFF_EVERY = 5;
/** Hard per-purchase deadline, for one the server never resolves and that
 *  carries no Pix window of its own: at most this many fallback polls, ever
 *  (~15 minutes with the backoff below). Past it only the websocket, a manual
 *  re-check or returning to the tab asks again. */
export const PURCHASE_POLL_BUDGET = 40;

// Not a beacon: the app has no metrics sink (`lib/telemetry.ts` is the
// client-*error* sink and a poll is not an error), so this counter exists to be
// asserted in the polls-per-purchase tests and read from the console while
// watching a live purchase.
let polls = 0;

/** How many fallback purchase polls this browser session has spent. */
export function purchasePollCount() {
  return polls;
}

export function resetPurchasePollCount() {
  polls = 0;
}

/** The two statuses that are still waiting on the payment provider. Every
 *  other status — confirmed, expired, failed, refunding, refunded — is final
 *  as far as the poll is concerned. */
export function purchaseActive(status?: string) {
  return status === 'pending' || status === 'processing';
}

/** The gap before the next fallback poll, or `false` to stop polling.
 *
 * Growing gaps, because a payment that has not landed in the first twenty
 * seconds is unlikely to land in the next four: 4s while fresh, then doubling
 * every `PURCHASE_POLL_BACKOFF_EVERY` polls up to a 30s ceiling. A Pix window
 * that has closed (`remainingMs <= 0`) or the spent budget stops it outright.
 *
 * Counting polls rather than elapsed time is deliberate: it is the same
 * quantity the budget is expressed in, it needs no clock read during render,
 * and a tab that spent an hour hidden (where the poll does not fire at all)
 * comes back at the gap it left off on instead of straight at the ceiling. */
export function nextPurchasePollMs(spent: number, remainingMs?: number): number | false {
  if (spent >= PURCHASE_POLL_BUDGET) return false;
  if (remainingMs !== undefined && remainingMs <= 0) return false;
  const steps = Math.floor(Math.max(0, spent) / PURCHASE_POLL_BACKOFF_EVERY);
  return Math.min(PURCHASE_POLL_MAX_MS, PURCHASE_POLL_MS * 2 ** steps);
}

export type TrackedPurchase = { status: string; expires_at?: string };

function remainingMsOf(purchase?: TrackedPurchase) {
  if (!purchase?.expires_at) return undefined;
  const expiry = new Date(purchase.expires_at).getTime();
  return Number.isNaN(expiry) ? undefined : expiry - Date.now();
}

/** Tracks one purchase's live status in the query cache.
 *
 * `purchase` is what the caller already knows locally (the create response, or
 * the row the parent holds); the query only runs while *that* is still active,
 * and the poll stops as soon as the fetched status is not. Callers read the
 * fresher of the two — `data ?? purchase` — exactly as the cosmetic dialog
 * did before this hook existed. */
export function usePurchaseStatus<T extends TrackedPurchase>({queryKey, queryFn, purchase, enabled = true}: {
  queryKey: QueryKey;
  queryFn: () => Promise<T>;
  purchase?: T;
  enabled?: boolean;
}) {
  // Per-purchase poll budget: the module counter above is session-wide, this
  // one drives this purchase's backoff and deadline. Keyed on the query key so
  // a second purchase in the same open dialog starts fresh at the 4s gap.
  const spent = useRef({key: '', count: 0});
  const key = JSON.stringify(queryKey);
  return useQuery<T>({
    queryKey,
    queryFn: () => {
      if (spent.current.key !== key) spent.current = {key, count: 0};
      spent.current.count += 1;
      polls += 1;
      return queryFn();
    },
    enabled: enabled && purchaseActive(purchase?.status),
    // What the caller already knows is what the cache starts with, so opening
    // a dialog on a row the create response (or the parent) just handed us
    // spends no read of its own — the first fallback read is one gap later.
    initialData: purchase,
    // Returning to the tab refetches (global `refetchOnWindowFocus`) — this is
    // what bounds that to a single read instead of one per remount.
    staleTime: PURCHASE_POLL_MS,
    refetchInterval: query => {
      const latest = query.state.data ?? purchase;
      if (!purchaseActive(latest?.status)) return false;
      return nextPurchasePollMs(spent.current.key === key ? spent.current.count : 0, remainingMsOf(latest));
    },
  });
}
