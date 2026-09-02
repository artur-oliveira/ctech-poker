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
 * `RealtimeBridge`: those pull in `useLobbyRealtime` -> `lib/ws/utils.ts` ->
 * the protobuf-generated `lib/api/proto/poker.ts`, which is ~1MB of JS a text
 * page has no business downloading. Authenticated routes get all three from
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
