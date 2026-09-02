import {MarketingQueryProvider} from '@/lib/providers/MarketingQueryProvider';

/** Marketing/SEO shell: the landing page, `/poker-rules` and `/guide/*`.
 * Deliberately lightweight — see MarketingQueryProvider for why. */
export default function MarketingLayout({children}: { children: React.ReactNode }) {
  return <MarketingQueryProvider>{children}</MarketingQueryProvider>;
}
