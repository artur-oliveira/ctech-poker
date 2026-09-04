import {QueryProvider} from '@/lib/providers/QueryProvider';
// The renderer aggregate is route-scoped here (#84); public routes only load base.css.
// app.css follows it to preserve the established authenticated-shell cascade.
import '../renderer.css';
import './app.css';

/** App shell: lobby, table, hands, achievements, profile, store and every
 * other authenticated/live surface. Owns keep-alive, the offline banner and
 * the single lobby socket via QueryProvider. */
export default function AppLayout({children}: { children: React.ReactNode }) {
  return <QueryProvider>{children}</QueryProvider>;
}
