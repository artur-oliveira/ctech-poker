'use client';
import {QueryClientProvider} from '@tanstack/react-query';
import React from 'react';
import {getQueryClient} from '@/lib/providers/createQueryClient';

/**
 * The lightweight provider for the (marketing) route group: the landing page,
 * `/poker-rules` and `/guide/*`. These pages are static, indexable and
 * logged-out-friendly, so they get plain TanStack Query for `useOptionalSession`
 * and the nav's social-unread badge — nothing else.
 *
 * Deliberately does NOT mount `useSessionKeepAlive`, `NetworkProvider` or
 * `RealtimeBridge`: none of them has anything to do on a logged-out text page.
 * (The bridge used to also drag in the full protobuf codec; since #228 it
 * decodes with `lib/ws/lobbyCodec.ts` and the table half of `poker.proto` is
 * loaded by the table route alone.) Authenticated routes get all three from
 * `QueryProvider` via the `(app)` group layout instead. See issue #80.
 *
 * It does, however, share the one browser QueryClient with `QueryProvider`
 * (via `getQueryClient`) so the `['player','me']` / wallet / social-unread
 * cache stays warm when an authenticated player crosses the route-group
 * boundary between `(app)` and `/guide` — otherwise the header flashed a `?`
 * avatar and a zeroed balance on every such navigation.
 */
export function MarketingQueryProvider({children}: {children: React.ReactNode}) {
  return <QueryClientProvider client={getQueryClient()}>{children}</QueryClientProvider>;
}
