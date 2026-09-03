import {QueryProvider} from '@/lib/providers/QueryProvider';
// The authenticated shell's own stylesheet (#84): the (marketing) group, the error
// boundaries and /unavailable are laid out by globals.css alone and never load it.
import './app.css';

/** App shell: lobby, table, hands, achievements, profile, store and every
 * other authenticated/live surface. Owns keep-alive, the offline banner and
 * the single lobby socket via QueryProvider. */
export default function AppLayout({children}: { children: React.ReactNode }) {
  return <QueryProvider>{children}</QueryProvider>;
}
