'use client';
import React from 'react';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {Award, BookOpen, ChevronLeft, Club, Crown, History, Sparkles, Trophy} from 'lucide-react';
import {leaderboard} from '@/lib/api/gamification';
import {getViewerId, playerName} from '@/lib/utils';
import {ProfileMenu} from "@/components/lobby/ProfileMenu";
import {useOptionalSession} from "@/lib/auth/session";

export default function Ranking() {
  const {data = [], isLoading, isError, refetch} = useQuery({queryKey: ['leaderboard'], queryFn: () => leaderboard()});
  const viewer = getViewerId();
  const {authed} = useOptionalSession();

  const topThree = data.slice(0, 3);
  const remaining = data.slice(3);
  const viewerEntry = data.find(p => p.player_id === viewer);

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
          <p>Desempenho auditável baseado em vitórias e mãos jogadas nas mesas do CTech Poker.</p>
        </header>

        {viewerEntry && (
          <div className="viewer-ranking-card" aria-label="Sua posição atual">
            <Sparkles aria-hidden="true"/>
            <div>
              <span>Sua posição no ranking</span>
              <strong>#{data.findIndex(p => p.player_id === viewer) + 1} de {data.length} jogadore{data.length === 1 ? 'r' : 's'}</strong>
            </div>
            <div className="viewer-ranking-stats">
              <span><b>{viewerEntry.hands_won}</b> vitórias</span>
              <span><b>{(viewerEntry.win_rate * 100).toFixed(1)}%</b> de taxa de vitória</span>
            </div>
          </div>
        )}

        {isLoading ? <div className="lobby-empty"><span className="loader"/>Buscando o ranking da comunidade…</div> :
          isError ? <div className="lobby-empty">Não foi possível carregar o ranking agora.
              <button type="button" className="link-retry" onClick={() => void refetch()}>Tentar novamente</button>
            </div> :
            !data.length ? <div className="lobby-empty">Nenhum jogador pontuou ainda. A primeira mesa inicia o Hall da Fama.</div> :
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
                    const isViewer = e.player_id === viewer;
                    return (
                      <article key={e.player_id}
                               className={isViewer ? 'viewer' : undefined}
                               style={{'--delay': `${Math.min(i, 10) * 40}ms`} as React.CSSProperties}>
                        <b>{String(rankNumber).padStart(2, '0')}</b>
                        <span>{playerName(e.player_id, viewer, e.player_name)}{isViewer && ' (Você)'}<small>{e.hands_played} mãos jogadas</small></span>
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
