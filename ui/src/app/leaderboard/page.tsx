'use client';
import React from 'react';
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

  const topThree = data.slice(0, 3);
  const remaining = data.slice(3);

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
          <Crown aria-hidden="true"/><small>HALL DA FAMA</small>
          <h1>Ranking da comunidade</h1>
          <p>Somente desempenho de jogo.</p>
        </header>
        {isLoading ? <div className="lobby-empty"><span className="loader"/>Buscando o ranking…</div> :
          !data.length ? <div className="lobby-empty">Ninguém jogou ainda. A primeira mesa faz o ranking.</div> :
            <>
              {topThree.length >= 3 && (
                <div className="leaderboard-podium" aria-label="Pódio do ranking">
                  {topThree.map((player, index) => {
                    const rank = index + 1;
                    const isViewer = player.player_id === viewer;
                    return (
                      <article key={player.player_id} className={`podium-card rank-${rank}${isViewer ? ' viewer' : ''}`}>
                        {rank === 1 && <Crown className="podium-crown" aria-hidden="true"/>}
                        <span className="podium-badge">{rank}º Lugar</span>
                        <strong className="podium-name">
                          {playerName(player.player_id, viewer, player.player_name)}
                          {isViewer && ' (Você)'}
                        </strong>
                        <div className="podium-stats">
                          <span><strong>{player.hands_won}</strong> vitórias</span>
                          <span>{player.hands_played} mãos ({(player.win_rate * 100).toFixed(1)}%)</span>
                        </div>
                      </article>
                    );
                  })}
                </div>
              )}

              <div className="ranking-list">
                {(topThree.length >= 3 ? remaining : data).map((e, i) => {
                  const rankNumber = topThree.length >= 3 ? i + 4 : i + 1;
                  return (
                    <article key={e.player_id}
                             className={e.player_id === viewer ? 'viewer' : undefined}
                             style={{'--delay': `${Math.min(i, 10) * 40}ms`} as React.CSSProperties}>
                      <b>{String(rankNumber).padStart(2, '0')}</b>
                      <span>{playerName(e.player_id, viewer, e.player_name)}<small>{e.hands_played} mãos</small></span>
                      <strong>{e.hands_won} vitórias<small>{(e.win_rate * 100).toFixed(1)}% de aproveitamento</small></strong>
                    </article>
                  );
                })}
              </div>
            </>}
      </section>
    </main>
  );
}

