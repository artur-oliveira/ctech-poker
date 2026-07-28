'use client';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {useState} from 'react';
import {useSessionKeepAlive} from '@/lib/auth/session';

export function QueryProvider({children}: { children: React.ReactNode }) {
  // The only component mounted on every route, so it owns the token keep-alive.
  useSessionKeepAlive();
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 60 * 1000,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
