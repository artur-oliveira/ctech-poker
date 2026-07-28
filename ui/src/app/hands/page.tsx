'use client';
import React, {useEffect, useMemo, useRef, useState} from 'react';
import Link from 'next/link';
import {useInfiniteQuery} from '@tanstack/react-query';
import {Award, BookOpen, ChevronLeft, ChevronRight, Club, History, Sparkles, Trophy} from 'lucide-react';
import {getHands} from '@/lib/api/player';
import {PlayingCard} from '@/components/table/PlayingCard';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';
import {TermsGate} from '@/components/TermsGate';
import {bestHandCategory} from '@/lib/pokerRules';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';
import {Button} from '@/components/ui/button';

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
  const {data, isLoading, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage} = useInfiniteQuery({
    queryKey: ['hands'],
    queryFn: ({pageParam}) => getHands({cursor: pageParam}),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => (page.has_next && page.next_cursor) || undefined
  });
  const [filter, setFilter] = useState<HandFilter>('all');

  // The entrance stagger is per page, not per flattened index: appended rows
  // should cascade like the first batch did instead of all landing on the
  // clamped 400 ms delay.
  const hands = useMemo(
    () => (data?.pages ?? []).flatMap(page => page.data.map((hand, i) => ({hand, delay: Math.min(i, 10) * 40}))),
    [data]
  );
  // Counts only cover what's been fetched so far; "+" keeps that honest
  // instead of presenting a page as the full history.
  const more = hasNextPage ? '+' : '';

  const stats = useMemo(() => {
    if (!hands.length) return null;
    let netSum = 0;
    let winsCount = 0;
    for (const {hand} of hands) {
      netSum += hand.net_change;
      if (hand.outcome === 'won' || hand.outcome === 'tied') winsCount++;
    }
    const winRate = Math.round((winsCount / hands.length) * 100);
    return {totalHands: hands.length, netSum, winsCount, winRate};
  }, [hands]);

  const filteredHands = useMemo(() => {
    if (filter === 'wins') return hands.filter(({hand}) => hand.outcome === 'won' || hand.outcome === 'tied');
    if (filter === 'losses') return hands.filter(({hand}) => hand.outcome === 'lost');
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
    <main className="app-page">
      <nav className="app-nav shell">
        <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
        <div className="header-right">
          <Link href="/guide"><BookOpen/> <span className="header-right-label">Guia</span></Link>
          <Link href="/leaderboard"><Trophy/> <span className="header-right-label">Ranking</span></Link>
          <Link href="/achievements"><Award/> <span className="header-right-label">Conquistas</span></Link>
          <Link href="/hands"><History/> <span className="header-right-label">Mãos</span></Link>
          <ProfileMenu/>
        </div>
      </nav>
      <section className="ranking hands shell">
        <Link href="/lobby"><ChevronLeft/> Lobby</Link>
        <header>
          <History aria-hidden="true"/><small>SEU HISTÓRICO</small>
          <h1>Mãos jogadas</h1>
          <p>Histórico recente das suas mãos com cartas, board comunitário e prova de integridade criptográfica.</p>
        </header>

        {stats && (
          <div className="hands-stats-bar">
            <div className="stat-card">
              <span className="stat-label">Mãos carregadas</span>
              <strong className="stat-value">{stats.totalHands}{more}</strong>
            </div>
            <div className="stat-card">
              <span className="stat-label">Balanço total</span>
              <strong className={`stat-value ${stats.netSum > 0 ? 'gain' : stats.netSum < 0 ? 'loss' : ''}`}>
                {stats.netSum > 0 ? '+' : ''}{stats.netSum.toLocaleString('pt-BR')}
              </strong>
            </div>
            <div className="stat-card">
              <span className="stat-label">Taxa de vitória</span>
              <strong className="stat-value">{stats.winRate}% <small>({stats.winsCount}V
                / {stats.totalHands - stats.winsCount}D)</small></strong>
            </div>
          </div>
        )}

        {!isLoading && !isError && hands.length > 0 && (
          <div className="filter-tabs" role="tablist" aria-label="Filtro de mãos">
            <button
              type="button"
              role="tab"
              aria-selected={filter === 'all'}
              className={`filter-tab${filter === 'all' ? ' active' : ''}`}
              onClick={() => setFilter('all')}
            >
              Todas ({hands.length}{more})
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={filter === 'wins'}
              className={`filter-tab${filter === 'wins' ? ' active' : ''}`}
              onClick={() => setFilter('wins')}
            >
              Vitórias ({stats?.winsCount ?? 0}{more})
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={filter === 'losses'}
              className={`filter-tab${filter === 'losses' ? ' active' : ''}`}
              onClick={() => setFilter('losses')}
            >
              Derrotas ({hands.length - (stats?.winsCount ?? 0)}{more})
            </button>
          </div>
        )}

        {isLoading ? <div className="lobby-empty"><span className="loader"/>Buscando seu histórico de mãos…</div> :
          isError ? <div className="lobby-empty">Não foi possível carregar seu histórico agora.
              <button type="button" className="link-retry" onClick={() => void refetch()}>Tentar novamente</button>
            </div> :
            !hands.length ?
              <div className="lobby-empty">Você ainda não jogou nenhuma mão. As rodadas concluídas aparecerão aqui
                automaticamente.</div> :
              !filteredHands.length ? (
                <div className="lobby-empty">
                  <Sparkles aria-hidden="true"/>
                  <p>Nenhuma mão encontrada neste filtro entre as {hands.length} carregadas.</p>
                  <Button variant="outline" size="sm" onClick={() => setFilter('all')}>Ver todas</Button>
                </div>
              ) : (
                <div className="hands-list">
                  {filteredHands.map(({hand, delay}) => <Link key={hand.hand_id}
                                                              href={`/hands/history?table_id=${hand.table_id}&hand_id=${encodeURIComponent(hand.sk)}`}
                                                              className="hand-row"
                                                              style={{'--delay': `${delay}ms`} as React.CSSProperties}>
                    <div className="hand-row-top">
                      <div className="hand-row-cards">
                        <div className="hand-row-card-group">
                          <small>Suas cartas{handCategoryLabel(hand.hole_cards, hand.board) &&
                              <span
                                  className="hand-category"> · {handCategoryLabel(hand.hole_cards, hand.board)}</span>}</small>
                          <div className="hand-row-card-group-cards">
                            {(hand.hole_cards || []).map((c, idx) => <PlayingCard key={idx} card={c} index={idx}
                                                                                  size="hole" owner="viewer"/>)}
                          </div>
                        </div>
                        <span className="hand-row-sep" aria-hidden="true"/>
                        <div className="hand-row-card-group">
                          <small>Mesa</small>
                          <div className="hand-row-card-group-cards hand-row-board">
                            {Array.from({length: 5}, (_, idx) => hand.board?.[idx]).map((c, idx) => c
                              ? <PlayingCard key={idx} card={c} index={idx} size="board"/>
                              : <span key={idx} className="board-empty-slot"/>)}
                          </div>
                        </div>
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
                      {hand.server_seed &&
                          <span className="hand-row-seed"
                                title={hand.server_seed}>seed {truncateSeed(hand.server_seed)}</span>}
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
      </section>
    </main>
  </TermsGate>;
}
