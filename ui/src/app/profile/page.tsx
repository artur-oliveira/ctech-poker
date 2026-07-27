'use client';
import {Suspense} from 'react';
import Link from 'next/link';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {ChevronLeft, Club, Sparkles, Trophy} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {PlayingCard} from '@/components/table/PlayingCard';
import {getProfileShowcase} from '@/lib/api/player';
import {achievementLabel} from '@/lib/achievements';

function ProfileContent() {
  const params = useSearchParams();
  const playerID = params.get('id') || '';
  const showcase = useQuery({
    queryKey: ['profile-showcase', playerID],
    queryFn: () => getProfileShowcase(playerID),
    enabled: Boolean(playerID),
    retry: false
  });

  return <main className="app-page profile-showcase-page">
    <nav className="app-nav shell">
      <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
      <Link href="/lobby"><ChevronLeft/> Lobby</Link>
    </nav>
    <section className="profile-showcase shell">
      {showcase.isLoading ? <div className="lobby-empty"><span className="loader"/>Carregando vitrine…</div> :
        showcase.isError || !showcase.data ? <div className="lobby-empty">
          <Sparkles aria-hidden="true"/>
          <h1>Vitrine indisponível</h1>
          <p>Este perfil não existe ou está configurado como privado.</p>
          <Button render={<Link href="/lobby"/>}>Ir ao lobby</Button>
        </div> : <>
          <header>
            <span className="profile-showcase-avatar" aria-hidden="true">
              {(showcase.data.name || '?').slice(0, 2).toUpperCase()}
            </span>
            <div><small>VITRINE DO JOGADOR</small><h1>{showcase.data.name || 'Jogador'}</h1></div>
          </header>
          <div className="profile-showcase-content">
            <section>
              <h2><Trophy aria-hidden="true"/> Conquistas em destaque</h2>
              {showcase.data.featured_achievements.length ? <div className="profile-featured-list">
                {showcase.data.featured_achievements.map(item => <article key={item.key}>
                  <Sparkles aria-hidden="true"/>
                  <div><b>{achievementLabel(item.key)}</b>
                    <span>{item.count.toLocaleString('pt-BR')} registrados</span></div>
                </article>)}
              </div> : <p className="profile-showcase-empty">Nenhuma conquista escolhida.</p>}
            </section>
            <section>
              <h2>Melhor resultado recente</h2>
              {showcase.data.best_hand ? <article className="profile-best-hand">
                <div className="profile-best-hand-cards">
                  {(showcase.data.best_hand.hole_cards || []).map((card, index) =>
                    <PlayingCard key={`${card}-${index}`} card={card} index={index} size="hole"/>)}
                </div>
                <b>+{showcase.data.best_hand.net_change.toLocaleString('pt-BR')} fichas</b>
                <span>{new Date(showcase.data.best_hand.ended_at < 1e12 ?
                  showcase.data.best_hand.ended_at * 1000 : showcase.data.best_hand.ended_at).toLocaleDateString('pt-BR')}</span>
              </article> : <p className="profile-showcase-empty">Ainda não há uma vitória recente para exibir.</p>}
            </section>
          </div>
        </>}
    </section>
  </main>;
}

export default function ProfilePage() {
  return <Suspense fallback={<main className="loading-screen"><span className="loader"/></main>}>
    <ProfileContent/>
  </Suspense>;
}
