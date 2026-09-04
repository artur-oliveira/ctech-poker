import type {QueryClient, QueryKey} from '@tanstack/react-query';

// A completed hand's player-visible projections — the last-winners strip's hand
// history (`['hands', tableId]`) and the "maior pote de hoje" highlight
// (`['highlights', tableId, 'today']`) — are written by a gamification pipeline
// the server detaches and only runs *after* it has already broadcast the
// `complete` snapshot. A single invalidate fired the moment that snapshot
// arrives therefore races ahead of the write and the finished hand shows up a
// whole hand late (or only on the 30s stale timer).
//
// Re-invalidating with a widening backoff catches the write shortly after it
// lands without polling — but it must stop the moment the projection is there.
// Fired unconditionally, the two sequences cost every seated player eight reads
// per hand (72 at a full 9-max table) even when the very first read already
// carried the settled hand. `settled` is what turns the backoff into a retry:
// it is asked before every read, so a projection that already landed costs zero
// reads and one that lands on the first try costs exactly one.
//
// Each entry is the gap *after* the previous attempt, not an offset from the
// settle: the cache is re-read between attempts, so the attempts are chained
// rather than armed all at once. Three gaps, so at most four reads, the last
// of them ~14.5s after the hand settled.
export const SETTLE_REFETCH_DELAYS_MS: readonly number[] = [1500, 4000, 9000];

// Post-hand read budget, per browser session. Not a beacon: the app has no
// metrics sink (`lib/telemetry.ts` is the client-*error* sink and this is not
// an error), so the counter exists to be asserted in the request-budget tests
// and read from the console while watching a live table.
let reads = 0;

/** How many post-hand reads this session has spent, across every table and
 *  resource. `resetSettleRefetchReads()` is the test seam. */
export function settleRefetchReads() {
  return reads;
}

export function resetSettleRefetchReads() {
  reads = 0;
}

/** Invalidate `queryKey` until its projection for the settled hand shows up.
 *
 * `settled` reads the query's own cache entry and answers "is the projection
 * there?". It is consulted before the first read too, so a hand whose
 * projection already arrived (over the socket, or from an earlier read) costs
 * nothing at all. Without it the sequence keeps its old unconditional shape.
 *
 * The last entry of `delays` is the deadline: after it, the sequence gives up
 * and leaves the query to its normal stale timer.
 *
 * Returns a cleanup that cancels whatever is still pending — call it from an
 * effect cleanup so an unmount or a newer hand doesn't leave stale timers
 * running. */
export function invalidateAfterSettle<T>(
  queryClient: QueryClient,
  queryKey: QueryKey,
  {settled, delays = SETTLE_REFETCH_DELAYS_MS}: {
    settled?: (data: T | undefined) => boolean;
    delays?: readonly number[];
  } = {},
): () => void {
  let cancelled = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function attempt(index: number) {
    if (cancelled || settled?.(queryClient.getQueryData<T>(queryKey))) return;
    reads += 1;
    void queryClient.invalidateQueries({queryKey});
    if (index >= delays.length) return;
    timer = setTimeout(() => attempt(index + 1), delays[index]);
  }

  attempt(0);
  return () => {
    cancelled = true;
    if (timer) clearTimeout(timer);
  };
}
