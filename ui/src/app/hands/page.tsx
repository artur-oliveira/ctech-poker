'use client';
import React, {memo, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import Link from 'next/link';
import {useInfiniteQuery} from '@tanstack/react-query';
import {measureElement, useWindowVirtualizer} from '@tanstack/react-virtual';
import {
  AlertCircle,
  ArrowRight,
  ChevronRight,
  History,
  LockKeyhole,
  ShieldCheck
} from 'lucide-react';
import type {HandItem, WalletMode} from '@/lib/api/player';
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
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';

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

const HandRow = memo(function HandRow({hand, mode}: { hand: HandItem; mode: WalletMode }) {
  const historyParams = new URLSearchParams({
    table_id: hand.table_id,
    hand_id: hand.hand_id,
    mode
  });
  const category = handCategoryLabel(hand.hole_cards, hand.board);
  return <Link href={`/hands/history?${historyParams.toString()}`} className={`hand-row is-${hand.outcome}`}>
    <div className="hand-row-top">
      <div className="hand-row-card-group">
        <small>Suas cartas</small>
        <div className="hand-row-card-group-cards">
          {(hand.hole_cards || []).map((card, index) => <PlayingCard key={index} card={card} index={index}
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
        {category
          ? <span className="hand-category">{category}</span>
          : <span className="hand-category is-unknown" aria-hidden="true">—</span>}
      </div>
      <div className="hand-row-result">
        <OutcomeBadge outcome={hand.outcome}/>
        <span className={`hand-net ${hand.net_change > 0 ? 'gain' : hand.net_change < 0 ? 'loss' : 'even'}`}>
          {hand.net_change > 0 ? '+' : ''}{hand.net_change.toLocaleString('pt-BR')}
        </span>
      </div>
    </div>
    <div className="hand-row-bottom">
      <span>{formatDate(hand.ended_at / 1000)}</span>
      <span className="hand-row-table" title={hand.table_id}>Mesa {hand.table_id}</span>
      {hand.server_seed
        ? <span className="hand-row-seed" title={hand.server_seed}><ShieldCheck aria-hidden="true"/> seed {truncateSeed(hand.server_seed)}</span>
        : <span className="hand-row-seed is-pending"><LockKeyhole aria-hidden="true"/> seed não revelada</span>}
      <ChevronRight className="hand-row-chevron" aria-hidden="true"/>
    </div>
  </Link>;
});

function VirtualHandsList({hands, mode}: { hands: HandItem[]; mode: WalletMode }) {
  const listRef = useRef<HTMLDivElement>(null);
  const [scrollMargin, setScrollMargin] = useState(0);

  useLayoutEffect(() => {
    const updateScrollMargin = () => {
      const top = listRef.current?.getBoundingClientRect().top;
      setScrollMargin(top === undefined ? 0 : top + window.scrollY);
    };
    updateScrollMargin();
    window.addEventListener('resize', updateScrollMargin, {passive: true});
    return () => window.removeEventListener('resize', updateScrollMargin);
  }, []);

  const virtualizer = useWindowVirtualizer({
    count: hands.length,
    estimateSize: () => 184,
    measureElement: (element, entry, instance) => measureElement(element, entry, instance) || 184,
    overscan: 3,
    scrollMargin
  });

  return <div
    ref={listRef}
    className="hands-list is-virtualized"
    role="list"
    aria-label={`${hands.length} mãos carregadas`}
    style={{height: virtualizer.getTotalSize()}}
  >
    {virtualizer.getVirtualItems().map(virtualHand => {
      const hand = hands[virtualHand.index];
      return <div
        key={hand.hand_id}
        ref={virtualizer.measureElement}
        className="hand-row-virtual"
        data-index={virtualHand.index}
        role="listitem"
        aria-posinset={virtualHand.index + 1}
        aria-setsize={hands.length}
        style={{transform: `translateY(${virtualHand.start - scrollMargin}px)`}}
      >
        <HandRow hand={hand} mode={mode}/>
      </div>;
    })}
  </div>;
}

export default function HandsHistory() {
  const [mode, setMode] = useState<WalletMode>('sandbox');
  const history = useInfiniteQuery({
    queryKey: ['hands', mode],
    queryFn: ({pageParam}) => getHands({cursor: pageParam, mode}),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => (page.has_next && page.next_cursor) || undefined
  });

  const hands = useMemo(
    () => (history.data?.pages ?? []).flatMap(page => page.data),
    [history.data]
  );

  const stats = useMemo(() => {
    if (!hands.length) return null;
    let netSum = 0;
    let wins = 0;
    let ties = 0;
    let losses = 0;
    for (const hand of hands) {
      netSum += hand.net_change;
      if (hand.outcome === 'won') wins++;
      else if (hand.outcome === 'tied') ties++;
      else losses++;
    }
    return {total: hands.length, netSum, wins, ties, losses, winRate: Math.round((wins / hands.length) * 100)};
  }, [hands]);

  const fetchNextPage = history.fetchNextPage;

  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !history.hasNextPage) return undefined;
    const observer = new IntersectionObserver(
      entries => entries[0]?.isIntersecting && void fetchNextPage(),
      {rootMargin: '600px 0px'}
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [fetchNextPage, hands.length, history.hasNextPage]);

  return <TermsGate><AppPage authed current="hands">
    <AppPageBody className="ranking hands">
      <AppPageHeader
        icon={History}
        eyebrow="HISTÓRICO"
        title="Minhas mãos"
        description="Reveja suas jogadas e confira a prova criptográfica calculada no navegador."
      />
      <CurrencyModeTabs mode={mode} onChangeAction={setMode}/>

      {history.isLoading ? <StatCardsSkeleton label="Somando as mãos carregadas…" count={3}/> : stats && (
        <div className="hands-stats-bar" aria-label="Resumo das mãos carregadas">
          <div className="stat-card">
            <span className="stat-label">Mãos carregadas</span>
            <strong className="stat-value">{stats.total}</strong>
          </div>
          <div className="stat-card">
            <span className="stat-label">Saldo carregado</span>
            <strong className={`stat-value ${stats.netSum > 0 ? 'gain' : stats.netSum < 0 ? 'loss' : ''}`}>
              {stats.netSum > 0 ? '+' : ''}{stats.netSum.toLocaleString('pt-BR')}
            </strong>
          </div>
          <div className="stat-card">
            <span className="stat-label">Vitórias carregadas</span>
            <strong className="stat-value">{stats.winRate}% <small>({stats.wins}V · {stats.ties}E · {stats.losses}D)</small></strong>
          </div>
        </div>
      )}

      {history.isLoading ?
        <SkeletonList label="Reunindo cartas, resultados e provas…" count={4} height={168} className="hands-list"/> :
        history.isError ? <div className="lobby-empty hands-state hands-state-error">
            <AlertCircle aria-hidden="true"/>
            <div>
              <strong>Seu histórico não abriu desta vez</strong>
              <p>As mãos continuam salvas. Tente carregar novamente.</p>
            </div>
            <Button variant="outline" size="sm"
                    onClick={() => void history.refetch()}>Tentar novamente</Button>
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
            (
              <VirtualHandsList hands={hands} mode={mode}/>
            )}

      {history.hasNextPage && !history.isLoading && !history.isError && (
        <div className="hands-more" ref={sentinel}>
          <Button variant="outline" size="sm" disabled={history.isFetchingNextPage}
                  onClick={() => void history.fetchNextPage()}>
            {history.isFetchingNextPage ? 'Carregando mais mãos…' : 'Carregar mais mãos'}
          </Button>
          <span className={`hands-more-track${history.isFetchingNextPage ? ' is-loading' : ''}`}
                role="status" aria-label={history.isFetchingNextPage ? 'Carregando mais mãos' : ''}/>
        </div>
      )}
    </AppPageBody>
  </AppPage></TermsGate>;
}
