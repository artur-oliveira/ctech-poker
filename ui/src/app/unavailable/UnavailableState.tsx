'use client';

import Link from 'next/link';
import {NetworkProvider, UnavailableScreen, Button} from '@aoctech/ui';
import {House} from 'lucide-react';
import {PokerLogo} from '@/components/PokerLogo';
import {HTTP_TIMEOUT_MS} from '@/lib/network/liveness';
import {USE_MOCK} from '@/lib/mockConfig';
import {networkLabels} from '@/lib/network/NetworkProvider';

const RETURN_AFTER_OUTAGE = 'poker:return-after-outage';

function returnDestination() {
  try {
    const saved = window.sessionStorage.getItem(RETURN_AFTER_OUTAGE);
    window.sessionStorage.removeItem(RETURN_AFTER_OUTAGE);
    return saved || '/lobby';
  } catch {
    // Keep the safe lobby destination when storage is unavailable.
    return '/lobby';
  }
}

/**
 * The `(app)` layout's probe is not mounted on this ungrouped route, so the
 * package's own `NetworkProvider` owns the poll here (backoff, visibility,
 * online/offline) and `UnavailableScreen` sends the player back to the
 * interrupted route the moment the API answers, whether anything was tapped or not.
 */
export function UnavailableState() {
  return <main className="system-state">
    <nav>
      <Link href="/" className="brand"><span className="brand-mark"><PokerLogo priority/></span>CTech <b>Poker</b></Link>
    </nav>
    <section data-ctech-bridge="">
      <NetworkProvider
        url={`${(process.env.NEXT_PUBLIC_API_URL || '').replace(/\/$/, '')}/v1.0/health`}
        timeoutMs={HTTP_TIMEOUT_MS}
        fetch={USE_MOCK ? () => Promise.resolve({ok: true} as Response) : undefined}
        banner={false}
        locale="pt-BR"
        labels={{...networkLabels, retry: 'Tentar novamente'}}
      >
        <UnavailableScreen
          locale="pt-BR"
          labels={{...networkLabels, retry: 'Tentar novamente'}}
          title="A mesa volta em breve."
          description="O servidor não respondeu, mas suas fichas e seu histórico continuam seguros."
          onRecovered={() => window.location.replace(returnDestination())}
        />
      </NetworkProvider>
      <div className="system-state-actions">
        <Button size="lg" variant="outline" render={<Link href="/"/>}><House/> Ir para o início</Button>
      </div>
    </section>
    <footer>Suas fichas e seu histórico permanecem seguros.</footer>
  </main>;
}
