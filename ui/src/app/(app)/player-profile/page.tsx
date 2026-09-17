'use client';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {ChevronRight, ShoppingBag, UserRound} from 'lucide-react';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {TermsGate} from '@/components/TermsGate';
import {Button} from '@/components/ui/button';
import {LoadingRegion, Skeleton} from '@/components/ui/skeleton';
import {chipsExact, moneyExact} from '@/lib/chips';
import {getMe, type PlayerProfile} from '@/lib/api/player';
import {REAL_MONEY_UI_ENABLED} from '@/lib/capabilities';
import {IdentitySection} from './IdentitySection';
import {ShowcaseSection} from './ShowcaseSection';
import {TableSection} from './TableSection';

function BalancesSection({me}: {me: PlayerProfile}) {
  return <section className="player-profile-section" aria-labelledby="player-profile-balances-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-balances-title">Seus saldos</h2>
      <p>O que você tem para colocar na mesa.</p>
    </header>
    <dl className="player-profile-balances">
      <div><dt>Fichas</dt><dd>{chipsExact(me.sandbox_balance ?? 0)}</dd></div>
      {REAL_MONEY_UI_ENABLED && <div><dt>Dinheiro real</dt><dd>{moneyExact(me.game_balance)}</dd></div>}
    </dl>
    <Button type="button" variant="ghost" className="profile-wallet-link" render={<Link href="/store"/>}>
      <ShoppingBag aria-hidden="true"/> <span><b>Loja</b><small>Reações, baralhos, feltros e fichas</small></span>
      <ChevronRight aria-hidden="true"/>
    </Button>
  </section>;
}

/**
 * The player's own profile, as a route rather than a popover and a dialog.
 * Name, photo, cosmetics and the whole public showcase used to be split across
 * a 360px menu and a 448px modal, which is what made each of them terse; here
 * they are one page with room to explain itself, and the menu keeps only the
 * shortcuts.
 */
export default function PlayerProfilePage() {
  const {data: me, isLoading} = useQuery({queryKey: ['player', 'me'], queryFn: getMe});

  return <TermsGate>
    <AppPage authed>
      <AppPageBody className="player-profile">
        <AppPageHeader
          icon={UserRound}
          eyebrow="SUA CONTA"
          title="Seu perfil"
          description="Nome, foto, baralho e tudo que a sua vitrine mostra para os outros jogadores."
        />
        {me
          ? <>
            <IdentitySection key={`identity:${me.name ?? ''}:${me.avatar_url ?? ''}`} me={me}/>
            <ShowcaseSection
              key={`showcase:${me.showcase_public}:${me.playstyle_public}:${me.table_public}:${(me.featured_achievements || []).join(',')}:${JSON.stringify(me.showcase_layout || {})}`}
              me={me}/>
            <TableSection me={me}/>
            <BalancesSection me={me}/>
          </>
          : <LoadingRegion label={isLoading ? 'Carregando seu perfil…' : 'Preparando seu perfil…'}
                           className="skeleton-panel player-profile-skeleton">
            <Skeleton style={{height: '96px', width: '96px', borderRadius: '50%'}}/>
            <Skeleton style={{height: '26px', width: 'min(280px, 70%)'}}/>
            <Skeleton style={{height: '180px'}}/>
            <Skeleton style={{height: '140px'}}/>
          </LoadingRegion>}
      </AppPageBody>
    </AppPage>
  </TermsGate>;
}
