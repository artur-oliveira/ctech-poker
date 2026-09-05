import {QueryClient} from '@tanstack/react-query';
import {QUERY_FRESHNESS} from '@/lib/queryFreshness';

/** Shared defaults for every QueryClient instance in the app, marketing and
 * app shells alike. Kept in one place so the two providers can't drift. */
export function createQueryClient(): QueryClient {
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30 * 1000,
        // Live data arrives over the websocket, and both realtime hooks
        // invalidate what they touch — on every push and on socket open, which
        // is the reconnect path. A global focus refetch on top of that re-read
        // catalogs, receipts and history that nothing had changed (#233).
        // Focus refetching is opt-in per family; see `lib/queryFreshness.ts`.
        refetchOnWindowFocus: false,
        // apiClient already gives safe reads one bounded, jittered retry
        // budget. Retrying here too multiplies three transport attempts
        // into nine requests during the same outage.
        retry: false,
      },
      mutations: {retry: false},
    },
  });
  // One classification for the whole app, applied by key prefix so a family's
  // freshness does not have to be repeated at each of its call sites.
  for (const [queryKey, options] of QUERY_FRESHNESS) client.setQueryDefaults(queryKey, options);
  return client;
}

let browserQueryClient: QueryClient | undefined;

/**
 * One QueryClient per browser session, shared by BOTH route-group providers
 * (`QueryProvider` for `(app)`, `MarketingQueryProvider` for `(marketing)`).
 *
 * Next.js keeps a single `<html>`/`<body>` mounted across route-group
 * boundaries, but each group layout mounts its own provider. Before #80
 * everything lived under one provider, so `['player','me']`, the wallet
 * balance and the social-unread count stayed warm across navigation. After the
 * split, walking from the lobby into `/guide` (or `/guide` into `/leaderboard`)
 * remounted the provider with an empty cache: the header briefly rendered the
 * `?` avatar placeholder and a zeroed balance pill before `getMe` re-resolved
 * — very visible under 3G throttling. Sharing the instance restores the warm
 * cache while keeping the two providers' *other* responsibilities separate.
 *
 * On the server every request still gets a fresh client (no shared state
 * across requests).
 */
export function getQueryClient(): QueryClient {
  if (typeof window === 'undefined') return createQueryClient();
  if (!browserQueryClient) browserQueryClient = createQueryClient();
  return browserQueryClient;
}
