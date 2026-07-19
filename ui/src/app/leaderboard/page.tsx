'use client'
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {ChevronLeft, Crown} from 'lucide-react';
import {leaderboard} from '@/lib/api/gamification';

export default function Ranking() {
  const {data = []} = useQuery({queryKey: ['leaderboard'], queryFn: leaderboard});
  return <main className="app-page">
    <section className="ranking shell"><Link href="/lobby"><ChevronLeft/> Lobby</Link>
      <header><Crown/><small>GLÓRIA, NÃO SALDO</small><h1>Ranking da comunidade</h1><p>Somente desempenho de jogo.
        Valores monetários nunca aparecem aqui.</p></header>
      <div className="ranking-list">{data.map((e, i) => <article key={e.player_id}>
        <b>{String(i + 1).padStart(2, '0')}</b><span>{e.player_id}<small>{e.hands_played} mãos</small></span><strong>{e.hands_won} vitórias<small>{(e.win_rate * 100).toFixed(1)}%
        win rate</small></strong></article>)}</div>
    </section>
  </main>
}
