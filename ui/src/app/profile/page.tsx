'use client';
import {Suspense} from 'react';
import Link from 'next/link';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {Sparkles, Swords, Trophy} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {PlayingCard} from '@/components/table/PlayingCard';
import {getMatchupStats, getProfileShowcase} from '@/lib/api/player';
import {achievementLabel} from '@/lib/achievements';
import {useOptionalSession} from "@/lib/auth/session";
import {AppPage} from '@/components/AppPageChrome';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {PlaystyleBadges} from '@/components/PlaystyleBadges';
import {LoadingRegion, Skeleton} from '@/components/ui/skeleton';
import {PlayerActionsMenu} from '@/components/social/PlayerActionsMenu';
import {getRelationship} from '@/lib/api/social';
import {useSocialActions} from '@/lib/hooks/useSocialActions';
import {SOCIAL_KEYS} from '@/lib/social';

function ProfileContent() {
  const params = useSearchParams();
  const playerID = params.get('id') || '';
  const {authed} = useOptionalSession();
  const showcase = useQuery({
    queryKey: ['profile-showcase', playerID],
    queryFn: () => getProfileShowcase(playerID),
    enabled: Boolean(playerID),
    retry: false
  });
  const matchup = useQuery({
    queryKey: ['profile-matchup', playerID],
    queryFn: () => getMatchupStats(playerID),
    enabled: authed && Boolean(playerID),
    retry: false
  });
  const socialActions = useSocialActions();
  // Fails (400) for the viewer's own profile, which is exactly when no safety
  // or friendship action should be offered — so an absent result hides the menu.
  const relationship = useQuery({
    queryKey: SOCIAL_KEYS.relationship(playerID),
    queryFn: () => getRelationship(playerID),
    enabled: authed && Boolean(playerID),
    retry: false
  });
  
  return <AppPage authed={authed} footer={false}>
    <section className="profile-showcase shell">
      {showcase.isLoading ?
        <LoadingRegion label="Carregando vitrine do jogador…" className="skeleton-panel profile-showcase-skeleton">
          <Skeleton style={{height: '68px', width: '68px', borderRadius: '50%'}}/>
          <Skeleton style={{height: '26px', width: 'min(260px, 70%)'}}/>
          <Skeleton style={{height: '150px'}}/>
          <Skeleton style={{height: '150px'}}/>
        </LoadingRegion> :
        showcase.isError || !showcase.data ? <div className="lobby-empty">
          <Sparkles aria-hidden="true"/>
          <h1>Vitrine indisponível</h1>
          <p>Este perfil não existe ou não foi encontrado.</p>
          <Button render={<Link href="/lobby"/>}>Ir para o Lobby</Button>
        </div> : <>
          <header>
            <PlayerAvatar className="profile-showcase-avatar" name={showcase.data.name}
                          avatarUrl={showcase.data.avatar_url} size={68}/>
            <div>
              <small>VITRINE DO JOGADOR</small>
              <h1>{showcase.data.name || 'Jogador'}</h1>
              {showcase.data.playstyle?.length
                ? <PlaystyleBadges badges={showcase.data.playstyle} className="profile-playstyle-badges"/>
                : null}
            </div>
            {relationship.data && <PlayerActionsMenu target={relationship.data} actions={socialActions}
                                                     surface="profile"/>}
          </header>
          <div className="profile-showcase-content">
            <section>
              <h2><Trophy aria-hidden="true"/> Conquistas em Destaque</h2>
              {showcase.data.featured_achievements.length ? <div className="profile-featured-list">
                {showcase.data.featured_achievements.map(item => <article key={item.key}>
                  <Sparkles aria-hidden="true"/>
                  <div>
                    <b>{achievementLabel(item.key)}</b>
                    <span>{item.count.toLocaleString('pt-BR')} registradas</span>
                  </div>
                </article>)}
              </div> : <p className="profile-showcase-empty">Nenhuma conquista selecionada para exibição.</p>}
            </section>
            <section>
              <h2>Melhor Vitória Recente</h2>
              {showcase.data.best_hand ? <article className="profile-best-hand">
                <div className="profile-best-hand-cards">
                  {(showcase.data.best_hand.hole_cards || []).map((card, index) =>
                    <PlayingCard key={`${card}-${index}`} card={card} index={index} size="hole"/>)}
                </div>
                <b>+{showcase.data.best_hand.net_change.toLocaleString('pt-BR')} fichas</b>
                <span>{new Date(showcase.data.best_hand.ended_at < 1e12 ?
                  showcase.data.best_hand.ended_at * 1000 : showcase.data.best_hand.ended_at).toLocaleDateString('pt-BR')}</span>
              </article> : <p className="profile-showcase-empty">Nenhuma vitória recente registrada nesta vitrine.</p>}
            </section>
            {matchup.data && matchup.data.hands_together > 0 && <section className="profile-matchup">
              <h2><Swords aria-hidden="true"/> Cara a Cara</h2>
              <p>
                Vocês já jogaram {matchup.data.hands_together.toLocaleString('pt-BR')} mãos juntos,
                {' '}você venceu {matchup.data.viewer_wins.toLocaleString('pt-BR')},{' '}
                {showcase.data.name || 'Jogador'} venceu {matchup.data.opponent_wins.toLocaleString('pt-BR')}.
              </p>
            </section>}
          </div>
        </>}
    </section>
  </AppPage>;
}

export default function ProfilePage() {
  return <Suspense fallback={<main className="app-page">
    <section className="profile-showcase shell">
      <LoadingRegion label="Carregando perfil…" className="skeleton-panel profile-showcase-skeleton">
        <Skeleton style={{height: '68px', width: '68px', borderRadius: '50%'}}/>
        <Skeleton style={{height: '26px', width: 'min(260px, 70%)'}}/>
        <Skeleton style={{height: '150px'}}/>
      </LoadingRegion>
    </section>
  </main>}>
    <ProfileContent/>
  </Suspense>;
}
