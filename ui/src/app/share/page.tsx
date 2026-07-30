'use client';
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {Club, ShieldCheck} from 'lucide-react';
import {getHandShare} from '@/lib/api/handShares';
import {PlayingCard} from '@/components/table/PlayingCard';
import {HandReplayer} from '@/components/hands/HandReplayer';
import {Button} from '@/components/ui/button';
import {LoadingRegion, Skeleton} from '@/components/ui/skeleton';
import type {HandItem} from '@/lib/api/player';

function SharedHandContent() {
  const token = useSearchParams().get('id') || '';
  const share = useQuery({
    queryKey: ['hand-share', token], queryFn: () => getHandShare(token), enabled: Boolean(token), retry: false
  });
  
  if (!token || share.isError) return <section className="public-hand unavailable">
    <h1>Link indisponível</h1>
    <p>Esta mão compartilhada pode ter sido revogada ou ter expirado.</p>
    <Button render={<Link href="/"/>}>Conhecer o CTech Poker</Button>
  </section>;
  
  if (share.isLoading || !share.data) return <section className="public-hand">
    <LoadingRegion label="Carregando mão compartilhada…" className="skeleton-panel">
      <Skeleton style={{height: '26px', width: 'min(240px, 70%)'}}/>
      <Skeleton style={{height: '104px'}}/>
      <Skeleton style={{height: '170px'}}/>
    </LoadingRegion>
  </section>;
  
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
      <span>{item.kind === 'bad_beat' ? 'Bad Beat Compartilhado' : 'Mão Compartilhada'}</span>
      <h1>{item.net_change >= 0 ? '+' : ''}{item.net_change.toLocaleString('pt-BR')} fichas</h1>
      <p><ShieldCheck aria-hidden="true"/> Jogadores anonimizados · expira
        em {new Date(item.expires_at).toLocaleDateString('pt-BR')}</p>
    </header>
    <div className="public-hand-cards">
      <article>
        <b>Herói</b>
        <div>{item.hero_cards?.length ? item.hero_cards.map((card, index) =>
            <PlayingCard key={card} card={card} index={index} size="hole" owner="viewer"/>) :
          <span>Cartas ocultas</span>}</div>
      </article>
      <article>
        <b>Board Comunitário</b>
        <div>{(item.board || []).map((card, index) =>
          <PlayingCard key={`${card}-${index}`} card={card} index={index} size="board"/>)}</div>
      </article>
    </div>
    {(item.actions || []).some(action => action.frame) &&
        <HandReplayer hand={hand} actions={item.actions || []} viewerId="hero"/>}
    <footer className="public-hand-footer">
      <p>Gostou dessa jogada? Venha jogar Texas Hold&apos;em no CTech Poker!</p>
      <Button render={<Link href="/"/>}>Jogar no CTech Poker</Button>
    </footer>
  </section>;
}

export default function SharedHandPage() {
  return <main className="public-hand-page">
    <nav><Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link></nav>
    <Suspense fallback={<section className="public-hand">
      <LoadingRegion label="Carregando…" className="skeleton-panel">
        <Skeleton style={{height: '26px', width: 'min(240px, 70%)'}}/>
        <Skeleton style={{height: '104px'}}/>
      </LoadingRegion>
    </section>}>
      <SharedHandContent/>
    </Suspense>
  </main>;
}
