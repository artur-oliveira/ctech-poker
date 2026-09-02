import {QueryClient} from '@tanstack/react-query';

/** Shared defaults for every QueryClient instance in the app, marketing and
 * app shells alike. Kept in one place so the two providers can't drift. */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30 * 1000,
        refetchOnWindowFocus: true,
        // apiClient already gives safe reads one bounded, jittered retry
        // budget. Retrying here too multiplies three transport attempts
        // into nine requests during the same outage.
        retry: false,
      },
      mutations: {retry: false},
    },
  });
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
