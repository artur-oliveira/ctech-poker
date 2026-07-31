'use client';
import React, {useEffect, useMemo, useRef, useState} from 'react';
import Link from 'next/link';
import {useInfiniteQuery} from '@tanstack/react-query';
import {AlertCircle, ArrowRight, ChevronRight, History, LockKeyhole, SearchX, ShieldCheck} from 'lucide-react';
import type {WalletMode} from '@/lib/api/player';
import {getHands} from '@/lib/api/player';
import {PlayingCard} from '@/components/table/PlayingCard';
import {BoardSlots} from '@/components/hands/BoardSlots';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {SkeletonList, StatCardsSkeleton} from '@/components/ui/skeleton';
import {TermsGate} from '@/components/TermsGate';
import {bestHandCategory} from '@/lib/pokerRules';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';
import {Button} from '@/components/ui/button';
import {CurrencyModeTabs} from '@/components/CurrencyModeTabs';
import {FilterGroup} from '@/components/FilterGroup';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';

type HandFilter = 'all' | 'wins' | 'losses';

function formatDate(unixSeconds: number) {
  return new Date(unixSeconds * 1000).toLocaleString('pt-BR', {
    day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit'
  });
}

function truncateSeed(hex: string) {
  return `${hex.slice(0, 8)}…`;
}

// Server only sends a category on live table state, never on hand history.
// It's resolvable client-side whenever the full 2 hole + 5 board cards are known.
function handCategoryLabel(holeCards?: string[], board?: string[]): string | null {
  if (holeCards?.length !== 2 || board?.length !== 5) return null;
  return HAND_CATEGORY_LABELS[bestHandCategory([...holeCards, ...board])] || null;
}

export default function HandsHistory() {
  const [mode, setMode] = useState<WalletMode>('sandbox');
  const {data, isLoading, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage} = useInfiniteQuery({
    queryKey: ['hands', mode],
    queryFn: ({pageParam}) => getHands({cursor: pageParam, mode}),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => (page.has_next && page.next_cursor) || undefined
  });
  const [filter, setFilter] = useState<HandFilter>('all');

  const hands = useMemo(
    () => (data?.pages ?? []).flatMap(page => page.data),
    [data]
  );
  // Counts only cover what's been fetched so far; "+" keeps that honest
  // instead of presenting a page as the full history.
  const more = hasNextPage ? '+' : '';

  const stats = useMemo(() => {
    if (!hands.length) return null;
    let netSum = 0;
    let winsCount = 0;
    for (const hand of hands) {
      netSum += hand.net_change;
      if (hand.outcome === 'won' || hand.outcome === 'tied') winsCount++;
    }
    const winRate = Math.round((winsCount / hands.length) * 100);
    return {totalHands: hands.length, netSum, winsCount, winRate};
  }, [hands]);

  const filteredHands = useMemo(() => {
    if (filter === 'wins') return hands.filter(hand => hand.outcome === 'won' || hand.outcome === 'tied');
    if (filter === 'losses') return hands.filter(hand => hand.outcome === 'lost');
    return hands;
  }, [hands, filter]);

  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !hasNextPage) return undefined;
    // rootMargin pre-fetches the next page just before the list bottoms out,
    // so scrolling never stops on a spinner.
    const observer = new IntersectionObserver(
      entries => entries[0]?.isIntersecting && void fetchNextPage(),
      {rootMargin: '600px 0px'}
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasNextPage, fetchNextPage, filteredHands.length]);

  return <TermsGate>
    <AppPage authed current="hands">
      <AppPageBody className="ranking hands">
        <AppPageHeader
          icon={History}
          eyebrow="SEU HISTÓRICO"
          title="Mãos jogadas"
          description="Reveja cada mesa, acompanhe seus resultados e abra a prova criptográfica de qualquer mão."
          backHref="/lobby"
        />
        <CurrencyModeTabs mode={mode} onChange={setMode}/>

        {isLoading ? <StatCardsSkeleton label="Somando suas mãos…" count={3}/> : stats && (
          <div className="hands-stats-bar" aria-label="Resumo das mãos carregadas">
            <div className="stat-card">
              <span className="stat-label">Mãos jogadas</span>
              <strong className="stat-value">{stats.totalHands}{more}</strong>
            </div>
            <div className="stat-card">
              <span className="stat-label">Saldo total</span>
              <strong className={`stat-value ${stats.netSum > 0 ? 'gain' : stats.netSum < 0 ? 'loss' : ''}`}>
                {stats.netSum > 0 ? '+' : ''}{stats.netSum.toLocaleString('pt-BR')}
              </strong>
            </div>
            <div className="stat-card">
              <span className="stat-label">% de vitória</span>
              <strong className="stat-value">{stats.winRate}% <small>({stats.winsCount}V
                / {stats.totalHands - stats.winsCount}D)</small></strong>
            </div>
          </div>
        )}

        {!isLoading && !isError && hands.length > 0 && (
          <FilterGroup
            label="Filtro de mãos"
            value={filter}
            options={[
              {value: 'all', label: `Todas (${hands.length}${more})`},
              {value: 'wins', label: `Vitórias (${stats?.winsCount ?? 0}${more})`},
              {value: 'losses', label: `Derrotas (${hands.length - (stats?.winsCount ?? 0)}${more})`}
            ]}
            onChange={setFilter}
          />
        )}

        {isLoading ?
          <SkeletonList label="Reunindo cartas, resultados e provas…" count={4} height={168} className="hands-list"/> :
          isError ? <div className="lobby-empty hands-state hands-state-error">
              <AlertCircle aria-hidden="true"/>
              <div>
                <strong>Seu histórico não abriu desta vez</strong>
                <p>As mãos continuam salvas. Tente carregar novamente.</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => void refetch()}>Tentar novamente</Button>
            </div> :
            !hands.length ?
              <div className="lobby-empty hands-state hands-state-first-hand">
                <div className="hands-empty-cards" aria-hidden="true">
                  <PlayingCard card="back" index={0} size="hole"/>
                  <PlayingCard card="back" index={1} size="hole"/>
                </div>
                <div>
                  <strong>Sua primeira mão começa no lobby</strong>
                  <p>Quando uma rodada terminar, cartas, resultado e prova aparecerão aqui.</p>
                </div>
                <Button render={<Link href="/lobby"/>} variant="outline" size="sm">
                  Encontrar uma mesa <ArrowRight aria-hidden="true"/>
                </Button>
              </div> :
              !filteredHands.length ? (
                <div className="lobby-empty hands-state">
                  <SearchX aria-hidden="true"/>
                  <div>
                    <strong>Nada por aqui neste filtro</strong>
                    <p>Nenhuma das {hands.length} mãos carregadas corresponde a esta seleção.</p>
                  </div>
                  <Button variant="outline" size="sm" onClick={() => setFilter('all')}>Ver todas</Button>
                </div>
              ) : (
                <div className="hands-list">
                  {filteredHands.map(hand => <Link key={hand.hand_id}
                                                              href={`/hands/history?table_id=${hand.table_id}&hand_id=${encodeURIComponent(hand.hand_id)}&mode=${mode}`}
                                                              className={`hand-row is-${hand.outcome}`}>
                    {/* Fixed grid tracks, not flow: every row has at most two hole
                        cards, five board slots, one category and one result, so
                        each of those gets its own column and lands at the same x
                        in every row — including the rows with no hand name. */}
                    <div className="hand-row-top">
                      <div className="hand-row-card-group">
                        <small>Suas cartas</small>
                        <div className="hand-row-card-group-cards">
                          {(hand.hole_cards || []).map((c, idx) => <PlayingCard key={idx} card={c} index={idx}
                                                                                size="hole" owner="viewer"/>)}
                        </div>
                      </div>
                      <span className="hand-row-sep" aria-hidden="true"/>
                      <div className="hand-row-card-group">
                        <small>Mesa</small>
                        <div className="hand-row-card-group-cards hand-row-board">
                          <BoardSlots board={hand.board}/>
                        </div>
                      </div>
                      <div className="hand-row-category">
                        {handCategoryLabel(hand.hole_cards, hand.board)
                          ? <span className="hand-category">{handCategoryLabel(hand.hole_cards, hand.board)}</span>
                          : <span className="hand-category is-unknown" aria-hidden="true">—</span>}
                      </div>
                      <div className="hand-row-result">
                        <OutcomeBadge outcome={hand.outcome}/>
                        <span
                          className={`hand-net ${hand.net_change > 0 ? 'gain' : hand.net_change < 0 ? 'loss' : 'even'}`}>
                          {hand.net_change > 0 ? '+' : ''}{hand.net_change.toLocaleString('pt-BR')}
                        </span>
                      </div>
                    </div>
                    <div className="hand-row-bottom">
                      <span>{formatDate(hand.ended_at / 1000)}</span>
                      {/* A hand can be proven without a full seed reveal (partial
                          proof) — saying so keeps the row's third column filled
                          and is honest about which proof this hand carries. */}
                      {hand.server_seed
                        ? <span className="hand-row-seed"
                                title={hand.server_seed}><ShieldCheck aria-hidden="true"/> seed {truncateSeed(hand.server_seed)}</span>
                        : <span className="hand-row-seed is-pending"><LockKeyhole aria-hidden="true"/> seed não revelada</span>}
                      <ChevronRight className="hand-row-chevron" aria-hidden="true"/>
                    </div>
                  </Link>)}
                </div>
              )}

        {hasNextPage && !isLoading && !isError && (
          <div className="hands-more" ref={sentinel}>
            {/* The observer above auto-loads on scroll; the button is the
                keyboard/reduced-input path and the recovery after a failure. */}
            <Button variant="outline" size="sm" disabled={isFetchingNextPage} onClick={() => void fetchNextPage()}>
              {isFetchingNextPage ? 'Carregando mais mãos…' : 'Carregar mais mãos'}
            </Button>
            <span
              className={`hands-more-track${isFetchingNextPage ? ' is-loading' : ''}`}
              role="status"
              aria-label={isFetchingNextPage ? 'Carregando mais mãos' : ''}
            />
          </div>
        )}
      </AppPageBody>
    </AppPage>
  </TermsGate>;
}
