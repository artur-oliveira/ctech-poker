'use client';
import {QueryClientProvider} from '@tanstack/react-query';
import React, {useState} from 'react';
import {useSessionKeepAlive} from '@/lib/auth/session';
import {RealtimeBridge} from '@/lib/providers/RealtimeBridge';
import {NetworkProvider} from '@/lib/network/NetworkProvider';
import {createQueryClient} from '@/lib/providers/createQueryClient';

/**
 * The full provider for the `(app)` route group: lobby, table, hands,
 * achievements and every other authenticated/live surface. Owns the token
 * keep-alive, the offline/unavailable banner and the single per-session lobby
 * socket (`RealtimeBridge`) — all mounted exactly once per app session, never
 * from the `(marketing)` group. See `lib/providers/MarketingQueryProvider.tsx`
 * for the lightweight counterpart used by static SEO pages.
 */
export function QueryProvider({children}: { children: React.ReactNode }) {
  // The only component mounted on every app-shell route, so it owns the token keep-alive.
  useSessionKeepAlive();
  const [client] = useState(createQueryClient);
  return (
    <QueryClientProvider client={client}>
      <NetworkProvider>
        <RealtimeBridge/>
        {children}
      </NetworkProvider>
    </QueryClientProvider>
  );
}
