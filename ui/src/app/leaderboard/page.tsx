'use client';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {Award, BookOpen, ChevronLeft, Club, Crown, History, Trophy} from 'lucide-react';
import {leaderboard} from '@/lib/api/gamification';
import {getViewerId, playerName} from '@/lib/utils';
import {ProfileMenu} from "@/components/lobby/ProfileMenu";
import {useOptionalSession} from "@/lib/auth/session";

export default function Ranking() {
  const {data = [], isLoading} = useQuery({queryKey: ['leaderboard'], queryFn: () => leaderboard()});
  const viewer = getViewerId();
  const {authed} = useOptionalSession();

  return (
    <main className="app-page">
      <nav className="app-nav shell">
        <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
        {authed ? <div className="header-right">
          <Link href="/guide"><BookOpen/> <span className="header-right-label">Guia</span></Link>
          <Link href="/leaderboard"><Trophy/> <span className="header-right-label">Ranking</span></Link>
          <Link href="/achievements"><Award/> <span className="header-right-label">Conquistas</span></Link>
          <Link href="/hands"><History/> <span className="header-right-label">Mãos</span></Link>
          <ProfileMenu/>
        </div> : <Link href="/"><ChevronLeft/> Voltar</Link>}
      </nav>
      <section className="ranking shell">
        {authed && <Link href="/lobby"><ChevronLeft/> Lobby</Link>}
        <header>
          <Crown/><small>HALL DA FAMA</small>
          <h1>Ranking da comunidade</h1>
          <p>Somente desempenho de jogo.</p>
        </header>
        {isLoading ? <div className="lobby-empty"><span className="loader"/>Buscando o ranking…</div> :
          !data.length ? <div className="lobby-empty">Ninguém jogou ainda. A primeira mesa faz o ranking.</div> :
            <div className="ranking-list">
              {data.map((e, i) => <article key={e.player_id}
                                           className={e.player_id === viewer ? 'viewer' : undefined}
                                           style={{'--delay': `${Math.min(i, 10) * 40}ms`} as React.CSSProperties}>
                <b>{String(i + 1).padStart(2, '0')}</b><span>{playerName(e.player_id, viewer, e.player_name)}<small>{e.hands_played} mãos</small></span><strong>{e.hands_won} vitórias<small>{(e.win_rate * 100).toFixed(1)}%
                de aproveitamento</small></strong></article>)}
            </div>}
      </section>
    </main>
  );
}
