import type {QueryKey} from '@tanstack/react-query';

/**
 * How fresh each family of server state has to be, in one table (#233).
 *
 * Before this, every query in the app shared one default — `staleTime: 30s`
 * plus `refetchOnWindowFocus: true` — and exactly one call site overrode it.
 * Tabbing back after half a minute therefore re-read catalogs, purchase
 * receipts and hand history that nothing had changed, on top of the websocket
 * that already pushes the things that do change.
 *
 * The default is now "do not refetch on focus", because the app's live data
 * arrives over `useLobbyRealtime`/`useTableRealtime` (which invalidate on every
 * relevant push *and* on socket open, so a reconnect is the reconciliation
 * path). Focus refetching is the exception, and every exception is named below.
 */

/** Catalogs and price lists: they change on a deploy, not on a session. Long
 *  staleTime is also what lets Store and Table share one read of the same
 *  cosmetic/reaction catalog instead of one per route. Ownership changes are
 *  explicit invalidations (purchase/refund/websocket), never a clock. */
export const STATIC_QUERY = {
  staleTime: 30 * 60 * 1000,
  refetchOnWindowFocus: false,
} as const;

/** Append-only records — hand history, purchase receipts. A new row arrives
 *  through an invalidation, and re-reading every loaded page on focus is what
 *  the "carregou mais sozinho" report was. */
export const HISTORY_QUERY = {
  staleTime: 5 * 60 * 1000,
  refetchOnWindowFocus: false,
} as const;

/** The player's own live standing — identity, balances, open sessions, the
 *  daily-reward clock. Cheap, single-row, and the most visible thing to be
 *  wrong after a laptop wakes up, so this family keeps focus refetching. */
export const SESSION_QUERY = {
  staleTime: 30 * 1000,
  refetchOnWindowFocus: true,
} as const;

/** Applied by `createQueryClient` through `setQueryDefaults`, so classifying a
 *  family costs no change at its (often many) call sites. Prefix keys: every
 *  query whose key starts with one of these inherits its preset. */
export const QUERY_FRESHNESS: ReadonlyArray<readonly [QueryKey, object]> = [
  [['player', 'me'], SESSION_QUERY],
  [['sessions'], SESSION_QUERY],
  [['dailyReward'], SESSION_QUERY],

  [['achievements', 'catalog'], STATIC_QUERY],
  [['stakes'], STATIC_QUERY],
  [['wallet', 'skus'], STATIC_QUERY],
  [['wallet', 'reaction-catalog'], STATIC_QUERY],
  [['wallet', 'cosmetic-catalog'], STATIC_QUERY],

  [['hands'], HISTORY_QUERY],
  [['wallet', 'sandbox-purchases'], HISTORY_QUERY],
  [['wallet', 'reaction-purchases'], HISTORY_QUERY],
  [['wallet', 'cosmetic-purchases'], HISTORY_QUERY],
];
