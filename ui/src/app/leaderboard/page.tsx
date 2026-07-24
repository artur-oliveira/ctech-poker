'use client';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {ChevronLeft, Crown} from 'lucide-react';
import {leaderboard} from '@/lib/api/gamification';
import {getViewerId, playerName} from '@/lib/utils';

export default function Ranking() {
  const {data = [], isLoading} = useQuery({queryKey: ['leaderboard'], queryFn: leaderboard});
  const viewer = getViewerId();
  return <main className="app-page">
    <section className="ranking shell"><Link href="/lobby"><ChevronLeft/> Lobby</Link>
      <header><Crown/><small>GLÓRIA, NÃO SALDO</small><h1>Ranking da comunidade</h1><p>Somente desempenho de jogo.
        Valores monetários nunca aparecem aqui.</p></header>
      {isLoading ? <div className="lobby-empty"><span className="loader"/>Buscando o ranking…</div> :
        !data.length ? <div className="lobby-empty">Ninguém jogou ainda. A primeira mesa faz o ranking.</div> :
          <div className="ranking-list">{data.map((e, i) => <article key={e.player_id}
                                                                     className={e.player_id === viewer ? 'viewer' : undefined}
                                                                     style={{'--delay': `${Math.min(i, 10) * 40}ms`} as React.CSSProperties}>
            <b>{String(i + 1).padStart(2, '0')}</b><span>{playerName(e.player_id, viewer)}<small>{e.hands_played} mãos</small></span><strong>{e.hands_won} vitórias<small>{(e.win_rate * 100).toFixed(1)}%
            de aproveitamento</small></strong></article>)}</div>}
    </section>
  </main>;
}
