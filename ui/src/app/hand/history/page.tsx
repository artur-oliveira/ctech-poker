'use client';
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {ChevronLeft, Crown, ShieldCheck} from 'lucide-react';
import {getHand} from '@/lib/api/player';
import {getHandHistory} from '@/lib/api/table';
import {PlayingCard} from '@/components/table/PlayingCard';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {ActionTimeline} from '@/components/hands/ActionTimeline';
import {DeckReveal} from '@/components/hands/DeckReveal';
import {TermsGate} from '@/components/TermsGate';
import {getViewerId, HAND_CATEGORY_LABELS, playerName} from '@/lib/utils';
import {bestHandCategory} from '@/lib/pokerRules';

function formatDate(unixSeconds: number) {
  return new Date(unixSeconds * 1000).toLocaleString('pt-BR', {
    day: '2-digit', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit'
  });
}

function HandHistoryContent() {
  const params = useSearchParams();
  const tableId = params.get('table_id') || '';
  const handId = params.get('hand_id') || '';

  const hand = useQuery({queryKey: ['hand', handId], queryFn: () => getHand(handId), enabled: Boolean(handId)});
  const history = useQuery({
    queryKey: ['hand-history', tableId, handId],
    queryFn: () => getHandHistory(tableId, handId),
    enabled: Boolean(tableId && handId)
  });

  const viewerId = getViewerId();
  const opponentNames = new Map((hand.data?.opponents || []).map(o => [o.player_id, o.name]));
  const resolveName = (playerId: string) => playerName(playerId, viewerId, opponentNames.get(playerId));

  if (!tableId || !handId) return <div className="hand-history shell">
    <p className="form-error">Link inválido: faltam table_id ou hand_id.</p>
    <Link href="/hands"><ChevronLeft/> Minhas mãos</Link>
  </div>;

  if (hand.isLoading) return <div className="loading-screen"><span className="loader"/>Carregando a mão…</div>;

  if (hand.isError || !hand.data) return <div className="hand-history shell">
    <Link href="/hands"><ChevronLeft/> Minhas mãos</Link>
    <p className="form-error">Não foi possível carregar esta mão. Ela pode não pertencer à sua conta.</p>
  </div>;

  const h = hand.data;
  // A tie still means the viewer took a share of the pot, same as an
  // outright win — only an outright loss leaves the viewer with no crown.
  const viewerIsWinner = h.outcome === 'won' || h.outcome === 'tied';

  // Server only sends a category on live table state, never on hand
  // history — resolvable client-side whenever a player's hole cards are
  // known (revealed at showdown) and the board ran out in full.
  function categoryFor(holeCards?: string[]): string | null {
    if (holeCards?.length !== 2 || h.board?.length !== 5) return null;
    return HAND_CATEGORY_LABELS[bestHandCategory([...holeCards, ...h.board])] || null;
  }

  const viewerCategory = categoryFor(h.hole_cards);

  return <div className="hand-history shell">
    <Link href="/hands"><ChevronLeft/> Minhas mãos</Link>
    <header className="hand-history-header">
      <OutcomeBadge outcome={h.outcome}/>
      <h1>Detalhes da mão</h1>
      <p>{formatDate(h.ended_at)} · Mesa {h.table_id.slice(0, 8)}…</p>
      <span className={`hand-net large ${h.net_change > 0 ? 'gain' : h.net_change < 0 ? 'loss' : 'even'}`}>
        {h.net_change > 0 ? '+' : ''}{h.net_change.toLocaleString('pt-BR')} fichas
      </span>
    </header>

    <section className="hand-history-players">
      <article className={`hand-history-seat viewer${viewerIsWinner ? ' is-winner' : ''}`}>
        {viewerIsWinner && <Crown aria-hidden="true" className="winner-crown"/>}
        <b>Você</b>
        <div className="hand-history-seat-cards">
          {(h.hole_cards || []).map((c, i) => <PlayingCard key={i} card={c} index={i} size="hole" owner="viewer"/>)}
        </div>
        {viewerCategory && <small className="hand-category">{viewerCategory}</small>}
      </article>
      {(h.opponents || []).map(o => {
        const category = categoryFor(o.hole_cards);
        return <article key={o.player_id} className={`hand-history-seat${o.won ? ' is-winner' : ''}`}>
          {o.won && <Crown aria-hidden="true" className="winner-crown"/>}
          <b>{o.name || 'Adversário'}</b>
          <div className="hand-history-seat-cards">
            {o.hole_cards?.length ? o.hole_cards.map((c, i) => <PlayingCard key={i} card={c} index={i} size="hole"/>) :
              <span className="hand-history-seat-hidden">Cartas não reveladas</span>}
          </div>
          {category && <small className="hand-category">{category}</small>}
        </article>;
      })}
    </section>

    <section className="hand-history-board">
      <h2>Board</h2>
      <div className="hand-board">
        {Array.from({length: 5}, (_, i) => h.board?.[i]).map((c, i) => c
          ? <PlayingCard key={i} card={c} index={i} size="board"/>
          : <span key={i} className="board-empty-slot large"/>)}
      </div>
    </section>

    <section className="hand-history-actions">
      <h2>Histórico de ações</h2>
      {history.isLoading ? <div className="lobby-empty"><span className="loader"/>Carregando ações…</div> :
        history.isError ? <p className="form-error">Não foi possível carregar o histórico de ações.</p> :
          <ActionTimeline actions={history.data?.actions || []} resolveName={resolveName}/>}
    </section>

    <section className="hand-history-fairness">
      <h2><ShieldCheck aria-hidden="true"/> Prova de integridade</h2>
      {h.server_seed && h.commit_hash
        ? <DeckReveal key={h.hand_id} serverSeed={h.server_seed} commitHash={h.commit_hash}/>
        : <p className="deck-reveal-status mismatch">Prova de integridade indisponível para esta mão.</p>}
    </section>
  </div>;
}

export default function HandHistoryPage() {
  return <TermsGate>
    <main className="app-page">
      <Suspense fallback={<div className="loading-screen"><span className="loader"/></div>}>
        <HandHistoryContent/>
      </Suspense>
    </main>
  </TermsGate>;
}
