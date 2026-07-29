'use client';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {useState} from 'react';
import {useSessionKeepAlive} from '@/lib/auth/session';
import {ApiError} from '@/lib/api/client';

const NON_RETRYABLE = new Set([400, 401, 403, 404, 409]);
const RETRYABLE = new Set([408, 425, 429, 500, 502, 503, 504]);

export function shouldRetryQuery(failureCount: number, error: unknown): boolean {
  if (error instanceof DOMException && error.name === 'AbortError') return false;
  if (error instanceof ApiError) {
    if (error.status && NON_RETRYABLE.has(error.status)) return false;
    if (error.status && !RETRYABLE.has(error.status)) return false;
  }
  return failureCount < 3;
}

export function queryRetryDelay(attempt: number, error: unknown): number {
  if (error instanceof ApiError && error.retryAfterMs !== undefined) return error.retryAfterMs;
  const ceiling = Math.min(8_000, 500 * 2 ** attempt);
  return Math.floor(Math.random() * ceiling);
}

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
            retry: shouldRetryQuery,
            retryDelay: queryRetryDelay,
          },
          mutations: {retry: false},
        },
      }),
  );
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
