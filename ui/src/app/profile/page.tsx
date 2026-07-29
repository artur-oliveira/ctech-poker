'use client';
import {Suspense} from 'react';
import Link from 'next/link';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {Award, BookOpen, ChevronLeft, Club, History, Sparkles, Trophy} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {PlayingCard} from '@/components/table/PlayingCard';
import {getProfileShowcase} from '@/lib/api/player';
import {achievementLabel} from '@/lib/achievements';
import {useOptionalSession} from "@/lib/auth/session";
import {ProfileMenu} from "@/components/lobby/ProfileMenu";
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {PlaystyleBadges} from '@/components/PlaystyleBadges';

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
  
  return <main className="app-page profile-showcase-page">
    <nav className="app-nav shell">
      <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
      {authed ? <div className="header-right">
        <Link href="/guide"><BookOpen/> <span className="header-right-label">Guia</span></Link>
        <Link href="/leaderboard"><Trophy/> <span className="header-right-label">Ranking</span></Link>
        <Link href="/achievements"><Award/> <span className="header-right-label">Conquistas</span></Link>
        <Link href="/hands"><History/> <span className="header-right-label">Mãos</span></Link>
        <ProfileMenu/>
      </div> : <Link href="/lobby"><ChevronLeft/> Lobby</Link>}
    </nav>
    <section className="profile-showcase shell">
      {authed && <Link href="/lobby"><ChevronLeft/> Lobby</Link>}
      {showcase.isLoading ?
        <div className="lobby-empty"><span className="loader"/>Carregando vitrine do jogador…</div> :
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
          </div>
        </>}
    </section>
  </main>;
}

export default function ProfilePage() {
  return <Suspense fallback={<main className="loading-screen"><span className="loader"/>Carregando perfil…</main>}>
    <ProfileContent/>
  </Suspense>;
}
