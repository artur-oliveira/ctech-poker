'use client';
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {Club, ShieldCheck} from 'lucide-react';
import {getHandShare} from '@/lib/api/handShares';
import {PlayingCard} from '@/components/table/PlayingCard';
import {HandReplayer} from '@/components/hands/HandReplayer';
import type {HandItem} from '@/lib/api/player';

function SharedHandContent() {
  const token = useSearchParams().get('id') || '';
  const share = useQuery({
    queryKey: ['hand-share', token], queryFn: () => getHandShare(token), enabled: Boolean(token), retry: false
  });
  if (!token || share.isError) return <section className="public-hand unavailable">
    <h1>Este link não está disponível</h1><p>Ele pode ter expirado ou sido revogado.</p><Link href="/">Conhecer o CTech Poker</Link>
  </section>;
  if (share.isLoading || !share.data) return <section className="public-hand loading"><span className="loader"/>Carregando mão…</section>;
  const item = share.data;
  const hand: HandItem = {
    pk: '', sk: '', table_id: 'shared', hand_id: item.token, outcome: item.outcome,
    net_change: item.net_change, ended_at: item.ended_at, board: item.board, hole_cards: item.hero_cards,
    opponents: (item.opponents || []).map((opponent, index) => ({
      player_id: `player_${index + 1}`, name: opponent.alias, hole_cards: opponent.hole_cards, won: opponent.won
    }))
  };
  return <section className="public-hand">
    <header>
      <span>{item.kind === 'bad_beat' ? 'Bad beat compartilhado' : 'Mão compartilhada'}</span>
      <h1>{item.net_change >= 0 ? '+' : ''}{item.net_change.toLocaleString('pt-BR')} fichas</h1>
      <p><ShieldCheck aria-hidden="true"/> Jogadores anonimizados · expira em {new Date(item.expires_at).toLocaleDateString('pt-BR')}</p>
    </header>
    <div className="public-hand-cards">
      <article><b>Herói</b><div>{item.hero_cards?.length ? item.hero_cards.map((card, index) =>
        <PlayingCard key={card} card={card} index={index} size="hole" owner="viewer"/>) : <span>Cartas ocultas</span>}</div></article>
      <article><b>Board</b><div>{(item.board || []).map((card, index) =>
        <PlayingCard key={`${card}-${index}`} card={card} index={index} size="board"/>)}</div></article>
    </div>
    {(item.actions || []).some(action => action.frame) &&
      <HandReplayer hand={hand} actions={item.actions || []} viewerId="hero"/>}
  </section>;
}

export default function SharedHandPage() {
  return <main className="public-hand-page">
    <nav><Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link></nav>
    <Suspense fallback={<section className="public-hand loading"><span className="loader"/></section>}>
      <SharedHandContent/>
    </Suspense>
  </main>;
}
