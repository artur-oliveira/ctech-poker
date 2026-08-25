'use client';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import React, {useState} from 'react';
import {useSessionKeepAlive} from '@/lib/auth/session';
import {RealtimeBridge} from '@/lib/providers/RealtimeBridge';
import {NetworkProvider} from '@/lib/network/NetworkProvider';

export function QueryProvider({children}: { children: React.ReactNode }) {
  // The only component mounted on every route, so it owns the token keep-alive.
  useSessionKeepAlive();
  const [client] = useState(
    () =>
      new QueryClient({
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
      }),
  );
  return (
    <QueryClientProvider client={client}>
      <NetworkProvider>
        <RealtimeBridge/>
        {children}
      </NetworkProvider>
    </QueryClientProvider>
  );
}
