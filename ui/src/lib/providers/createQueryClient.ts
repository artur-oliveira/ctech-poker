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
