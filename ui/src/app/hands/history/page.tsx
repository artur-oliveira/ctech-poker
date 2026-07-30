'use client';
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {useQuery} from '@tanstack/react-query';
import {ChevronLeft, ChevronRight, Crown, Play, ShieldCheck} from 'lucide-react';
import type {WalletMode} from '@/lib/api/player';
import {getHand} from '@/lib/api/player';
import {getHandHistory} from '@/lib/api/table';
import {PlayingCard} from '@/components/table/PlayingCard';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {BoardSlots} from '@/components/hands/BoardSlots';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {ActionTimeline} from '@/components/hands/ActionTimeline';
import {LoadingRegion, Skeleton, SkeletonList} from '@/components/ui/skeleton';
import {DeckReveal} from '@/components/hands/DeckReveal';
import {PartialDeckProof} from '@/components/hands/PartialDeckProof';
import {HandExportButton} from '@/components/hands/HandExportButton';
import {ShareHandDialog} from '@/components/hands/ShareHandDialog';
import {Button} from '@/components/ui/button';
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
  const mode: WalletMode = params.get('mode') === 'real' ? 'real' : 'sandbox';
  
  const hand = useQuery({
    queryKey: ['hand', mode, handId],
    queryFn: () => getHand(handId, mode),
    enabled: Boolean(handId)
  });
  const history = useQuery({
    queryKey: ['hand-history', tableId, handId],
    queryFn: () => getHandHistory(tableId, handId),
    enabled: Boolean(tableId && handId)
  });
  
  const actions = (history.data?.actions || []).sort((a, b) => (a.timestamp || 0) - (b.timestamp || 0));
  const viewerId = getViewerId();
  const opponentNames = new Map((hand.data?.opponents || []).map(o => [o.player_id, o.name]));
  const resolveName = (playerId: string) => playerName(playerId, viewerId, opponentNames.get(playerId));
  
  if (!tableId || !handId) return <div className="hand-history shell">
    <p className="form-error">Link de mão inválido ou incompleto.</p>
    <Link href="/hands"><ChevronLeft/> Voltar para Minhas Mãos</Link>
  </div>;
  
  // Shaped like the loaded page (tool row, result header, seats, board, timeline)
  // so nothing jumps when the hand arrives.
  if (hand.isLoading) return <div className="hand-history shell">
    <LoadingRegion label="Carregando detalhes da mão…" className="skeleton-panel hand-history-skeleton">
      <Skeleton style={{height: '20px', width: '190px'}}/>
      <Skeleton style={{height: '104px'}}/>
      <Skeleton style={{height: '170px'}}/>
      <Skeleton style={{height: '140px'}}/>
    </LoadingRegion>
  </div>;
  
  if (hand.isError || !hand.data) return <div className="hand-history shell">
    <Link href="/hands"><ChevronLeft/> Voltar para Minhas Mãos</Link>
    <p className="form-error">Não foi possível carregar esta mão. Verifique se você está conectado na conta correta.</p>
  </div>;
  
  const h = hand.data;
  const viewerIsWinner = h.outcome === 'won' || h.outcome === 'tied';
  
  function categoryFor(holeCards?: string[]): string | null {
    if (holeCards?.length !== 2 || h.board?.length !== 5) return null;
    return HAND_CATEGORY_LABELS[bestHandCategory([...holeCards, ...h.board])] || null;
  }
  
  const viewerCategory = categoryFor(h.hole_cards);
  
  return <div className="hand-history shell">
    {/* Export and share are utilities: they belong on the page's tool row beside
        the way out, not stacked down the centre line where the result and the
        winners are what the player came to read. */}
    <div className="hand-history-topbar">
      <Link href="/hands"><ChevronLeft/> Voltar para Minhas Mãos</Link>
      {!history.isLoading && !history.isError &&
          <div className="hand-history-tools">
              <HandExportButton hand={h} actions={actions} viewerId={viewerId}/>
              <ShareHandDialog handId={h.hand_id} outcome={h.outcome} mode={mode}/>
          </div>}
    </div>
    <header className="hand-history-header">
      <OutcomeBadge outcome={h.outcome}/>
      <h1>Detalhes da Mão</h1>
      <p>{formatDate(h.ended_at / 1000)} · Mesa {h.table_id.slice(0, 8)}…</p>
      <span className={`hand-net large ${h.net_change > 0 ? 'gain' : h.net_change < 0 ? 'loss' : 'even'}`}>
        {h.net_change > 0 ? '+' : ''}{h.net_change.toLocaleString('pt-BR')} fichas
      </span>
    </header>

    {!history.isLoading && !history.isError && actions.some(action => action.frame) &&
        <div className="hand-replay-launch">
            <div>
                <Play aria-hidden="true"/>
                <span>
            <b>Reviva esta mão ação por ação</b>
            <small>O replayer abre nesta mesma aba; o botão de voltar traz você de volta para cá.</small>
          </span>
            </div>
            {/* Same tab on purpose: the replayer's own "Voltar para Detalhes da
                Mão" is a back link, and a new tab left the player with two
                windows on the same hand and no way back in either. */}
            <Button render={<Link
              href={`/hands/replay?table_id=${encodeURIComponent(tableId)}&hand_id=${encodeURIComponent(handId)}&mode=${mode}`}/>}>
                Assistir replay <ChevronRight aria-hidden="true"/>
            </Button>
        </div>}
    
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
          <PlayerAvatar name={o.name} avatarUrl={o.avatar_url} size={36}/>
          <b>{o.name || 'Adversário'}</b>
          <div className="hand-history-seat-cards">
            {o.hole_cards?.length
              ? o.hole_cards.map((c, i) => <PlayingCard key={i} card={c} index={i} size="hole"/>)
              : <><span className="board-slot-undealt" aria-hidden="true">
                  <PlayingCard index={0} size="hole"/><PlayingCard index={1} size="hole"/>
                </span>
                <span className="hand-history-seat-hidden">Cartas não reveladas</span></>}
          </div>
          {category && <small className="hand-category">{category}</small>}
        </article>;
      })}
    </section>
    
    <section className="hand-history-board">
      <h2>Board Comunitário</h2>
      <div className="hand-board">
        <BoardSlots board={h.board}/>
      </div>
    </section>
    
    <section className="hand-history-actions">
      <h2>Histórico de Ações</h2>
      {history.isLoading ?
        <SkeletonList label="Carregando histórico de ações…" count={5} height={44} className="skeleton-panel"/> :
        history.isError ? <p className="form-error">Não foi possível carregar a sequência de ações desta mão.</p> :
          <ActionTimeline actions={actions} resolveName={resolveName}/>}
    </section>
    
    <section className="hand-history-fairness">
      <h2><ShieldCheck aria-hidden="true"/> Prova de Integridade (Provably Fair)</h2>
      {h.server_seed && h.commit_hash
        ? <DeckReveal key={h.hand_id} serverSeed={h.server_seed} commitHash={h.commit_hash}/>
        : h.root_commit_hash && h.revealed_card_salts && h.unrevealed_card_hashes
          ? <PartialDeckProof key={h.hand_id} rootCommitHash={h.root_commit_hash}
                              revealed={h.revealed_card_salts} unrevealed={h.unrevealed_card_hashes}/>
          :
          <p className="deck-reveal-status mismatch">Prova de integridade criptográfica indisponível para esta mão.</p>}
    </section>
  </div>;
}

export default function HandHistoryPage() {
  return <TermsGate>
    <main className="app-page">
      <Suspense fallback={<div className="hand-history shell">
        <LoadingRegion label="Carregando mão…" className="skeleton-panel hand-history-skeleton">
          <Skeleton style={{height: '20px', width: '190px'}}/>
          <Skeleton style={{height: '104px'}}/>
          <Skeleton style={{height: '170px'}}/>
        </LoadingRegion>
      </div>}>
        <HandHistoryContent/>
      </Suspense>
    </main>
  </TermsGate>;
}
