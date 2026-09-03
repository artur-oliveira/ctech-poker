import type {QueryClient, QueryKey} from '@tanstack/react-query';

// A completed hand's player-visible projections — the last-winners strip's hand
// history (`['hands', tableId]`) and the "maior pote de hoje" highlight
// (`['highlights', tableId, 'today']`) — are written by a gamification pipeline
// the server detaches and only runs *after* it has already broadcast the
// `complete` snapshot. A single invalidate fired the moment that snapshot
// arrives therefore races ahead of the write and the finished hand shows up a
// whole hand late (or only on the 30s stale timer). Re-invalidating a few
// times with a widening backoff catches the write shortly after it lands
// without polling.
export const SETTLE_REFETCH_DELAYS_MS: readonly number[] = [1500, 4000, 9000];

/** Invalidate `queryKey` immediately, then again after each backoff delay.
 *  Returns a cleanup that cancels the still-pending invalidations — call it
 *  from an effect cleanup so an unmount or a newer hand doesn't leave stale
 *  timers running. */
export function invalidateAfterSettle(
  queryClient: QueryClient,
  queryKey: QueryKey,
  delays: readonly number[] = SETTLE_REFETCH_DELAYS_MS,
): () => void {
  void queryClient.invalidateQueries({queryKey});
  const timers = delays.map(delay =>
    setTimeout(() => void queryClient.invalidateQueries({queryKey}), delay));
  return () => timers.forEach(timer => clearTimeout(timer));
}
