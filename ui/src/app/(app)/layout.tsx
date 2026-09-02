import {QueryProvider} from '@/lib/providers/QueryProvider';

/** App shell: lobby, table, hands, achievements, profile, store and every
 * other authenticated/live surface. Owns keep-alive, the offline banner and
 * the single lobby socket via QueryProvider. */
export default function AppLayout({children}: { children: React.ReactNode }) {
  return <QueryProvider>{children}</QueryProvider>;
}
