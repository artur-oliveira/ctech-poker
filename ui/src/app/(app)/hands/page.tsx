'use client';
import React, {memo, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import Link from 'next/link';
import {useInfiniteQuery, useQuery} from '@tanstack/react-query';
import {measureElement, useWindowVirtualizer} from '@tanstack/react-virtual';
import {
  AlertCircle,
  ArrowRight,
  ChevronRight,
  History,
  Infinity as InfinityIcon,
  LockKeyhole,
  ShieldCheck
} from 'lucide-react';
import type {HandItem, WalletMode} from '@/lib/api/player';
import {getHands, handEndedAtMs} from '@/lib/api/player';
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
import {FilterGroup} from '@/components/FilterGroup';
import {MyHandSharesPanel} from '@/components/hands/MyHandSharesPanel';
import {myRank} from '@/lib/api/gamification';
import {
  ALL_TABLES, filterHands, groupHandsByDay, handTables, type HandsFilter, type HandsRow, loadedTotals,
  NO_FILTER, type OutcomeFilter, shortTableId
} from '@/lib/handsHistory';

function formatDate(endedAtMs: number) {
  return new Date(handEndedAtMs(endedAtMs)).toLocaleString('pt-BR', {
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
  return <Link href={`/hands/history?${historyParams.toString()}`} className={`hand-row static-cards is-${hand.outcome}`}>
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
      <span>{formatDate(hand.ended_at)}</span>
      <span className="hand-row-table" title={hand.table_id}>Mesa {shortTableId(hand.table_id)}</span>
      {/* Blind level of this hand (#75). 0/absent means the record predates the
          field: hide the marker rather than guess a stake. */}
      {Boolean(hand.big_blind) && <span className="hand-row-blinds">
        {(hand.small_blind ?? 0).toLocaleString('pt-BR')}/{hand.big_blind!.toLocaleString('pt-BR')}
      </span>}
      {hand.server_seed
        ? <span className="hand-row-seed" title={hand.server_seed}><ShieldCheck aria-hidden="true"/> seed {truncateSeed(hand.server_seed)}</span>
        : <span className="hand-row-seed is-pending"><LockKeyhole aria-hidden="true"/> seed não revelada</span>}
      <ChevronRight className="hand-row-chevron" aria-hidden="true"/>
    </div>
  </Link>;
});

function VirtualHandsList({rows, mode}: { rows: HandsRow[]; mode: WalletMode }) {
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
    count: rows.length,
    estimateSize: index => rows[index].kind === 'day' ? 52 : 184,
    measureElement: (element, entry, instance) => measureElement(element, entry, instance) || 184,
    overscan: 3,
    scrollMargin
  });

  const virtualRows = virtualizer.getVirtualItems();
  // The pinned bar names whichever day is at the top of the viewport. Sticky
  // positioning cannot live on the rows themselves — each one is placed with a
  // transform, which is exactly what `position: sticky` cannot escape — so one
  // opaque bar outside the transformed rows does the pinning, and the inline
  // day headings scroll underneath it.
  const firstRow = rows[virtualRows[0]?.index ?? 0];
  const pinnedDay = firstRow?.kind === 'day' ? firstRow.label : firstRow?.day;

  // The `role="list"` the flat version carried is gone on purpose: the rows are
  // now interleaved with real `h3` day headings, and a list whose children are
  // half headings is a worse tree than links under headings. The named section
  // keeps the count the old `aria-label` announced.
  const handCount = rows.reduce((total, row) => total + (row.kind === 'hand' ? 1 : 0), 0);

  return <section aria-label={`${handCount} ${handCount === 1 ? 'mão' : 'mãos'} nesta lista, agrupadas por dia`}>
    {pinnedDay && <div className="hands-day-pinned" aria-hidden="true">{pinnedDay}</div>}
    <div
      ref={listRef}
      className="hands-list is-virtualized"
      style={{height: virtualizer.getTotalSize()}}
    >
      {virtualRows.map(virtualRow => {
        const row = rows[virtualRow.index];
        return <div
          key={row.key}
          ref={virtualizer.measureElement}
          className={`hand-row-virtual${row.kind === 'day' ? ' is-day' : ''}`}
          data-index={virtualRow.index}
          style={{transform: `translateY(${virtualRow.start - scrollMargin}px)`}}
        >
          {row.kind === 'day'
            ? <h3 className="hands-day-header">{row.label}
              <small>{row.count} {row.count === 1 ? 'mão' : 'mãos'}</small></h3>
            : <HandRow hand={row.hand} mode={mode}/>}
        </div>;
      })}
    </div>
  </section>;
}

export default function HandsHistory() {
  const [mode, setMode] = useState<WalletMode>('sandbox');
  const history = useInfiniteQuery({
    queryKey: ['hands', mode],
    queryFn: ({pageParam}) => getHands({cursor: pageParam, mode}),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => (page.has_next && page.next_cursor) || undefined,
    // React Query refetches EVERY loaded page on window focus. With 18 pages
    // loaded that is 18 requests for tabbing back into the window, which is
    // what the "carregou mais sozinho" report was.
    refetchOnWindowFocus: false
  });

  const hands = useMemo(
    () => (history.data?.pages ?? []).flatMap(page => page.data),
    [history.data]
  );

  const [filter, setFilter] = useState<HandsFilter>(NO_FILTER);
  const visible = useMemo(() => filterHands(hands, filter), [hands, filter]);
  const rows = useMemo(() => groupHandsByDay(visible), [visible]);
  const tables = useMemo(() => handTables(hands), [hands]);
  const stats = useMemo(() => loadedTotals(visible), [visible]);

  // Lifetime W/L comes from the leaderboard's own counters, the only totals
  // that describe every hand ever played instead of the pages loaded here.
  // `ranked: false` means "no stats row yet", not an error.
  const lifetime = useQuery({queryKey: ['leaderboard', 'me', mode], queryFn: () => myRank(mode)});

  const fetchNextPage = history.fetchNextPage;

  // Auto-load only makes sense on the unfiltered list. Under a filter the
  // visible list stays short no matter how many pages arrive, so the sentinel
  // never leaves the viewport and every appended page immediately triggers the
  // next one — the whole history downloaded in one cascade. Filtered lists keep
  // the explicit "Carregar mais mãos" button instead.
  const autoLoad = filter.outcome === 'all' && filter.tableId === ALL_TABLES;

  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !history.hasNextPage || !autoLoad) return undefined;
    const observer = new IntersectionObserver(
      entries => entries[0]?.isIntersecting && void fetchNextPage(),
      {rootMargin: '600px 0px'}
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [autoLoad, fetchNextPage, hands.length, history.hasNextPage]);

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
        <div className="hands-totals">
          <div className="hands-stats-bar" aria-label="Resumo das mãos nesta lista">
            <div className="stat-card">
              <span className="stat-label">Mãos nesta lista</span>
              <strong className="stat-value">{stats.total}</strong>
            </div>
            <div className="stat-card">
              <span className="stat-label">Saldo nesta lista</span>
              <strong className={`stat-value ${stats.netSum > 0 ? 'gain' : stats.netSum < 0 ? 'loss' : ''}`}>
                {stats.netSum > 0 ? '+' : ''}{stats.netSum.toLocaleString('pt-BR')}
              </strong>
            </div>
            <div className="stat-card">
              <span className="stat-label">Vitórias nesta lista</span>
              <strong className="stat-value">{stats.winRate}% <small>({stats.wins}V · {stats.ties}E · {stats.losses}D)</small></strong>
            </div>
          </div>
          {/* The bar above only ever describes the pages already loaded (and
              the active filter). This strip is the honest lifetime number, so
              a heavy player is not left reading "carregadas" as their record. */}
          {lifetime.data?.ranked && lifetime.data.entry && <p className="hands-lifetime" role="note">
            <InfinityIcon aria-hidden="true"/>
            <span>Desde o início: <b>{lifetime.data.entry.hands_played.toLocaleString('pt-BR')} mãos</b>,
              {' '}<b>{lifetime.data.entry.hands_won.toLocaleString('pt-BR')} vitórias</b>
              {' '}({Math.round(lifetime.data.entry.win_rate * 100)}%)</span>
          </p>}
        </div>
      )}

      {!history.isLoading && !history.isError && hands.length > 0 && <div className="hands-filters">
        <FilterGroup
          label="Filtro por resultado"
          value={filter.outcome}
          options={[
            {value: 'all', label: `Todas (${hands.length})`},
            {value: 'won', label: 'Só vitórias'},
            {value: 'lost', label: 'Só derrotas'},
            {value: 'tied', label: 'Só empates'}
          ] as const}
          onChangeAction={(outcome: OutcomeFilter) => setFilter(current => ({...current, outcome}))}
        />
        {tables.length > 1 && <FilterGroup
          label="Filtro por mesa"
          className="filter-tabs-scroll"
          value={filter.tableId}
          options={[
            {value: ALL_TABLES, label: 'Todas as mesas'},
            ...tables.map(table => ({
              value: table.tableId,
              // Head+tail of the id so the "(count)" suffix stays visible; a CSS
              // trailing clip would eat it. Full id in the tooltip.
              label: `Mesa ${shortTableId(table.tableId)} (${table.count})`,
              title: table.tableId
            }))
          ]}
          onChangeAction={(tableId: string) => setFilter(current => ({...current, tableId}))}
        />}
      </div>}

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
            !visible.length ? <div className="lobby-empty hands-state">
              <div>
                <strong>Nenhuma mão com esse filtro</strong>
                <p>As {hands.length} mãos carregadas continuam aqui. Solte o filtro para vê-las de novo.</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => setFilter(NO_FILTER)}>Limpar filtros</Button>
            </div> : (
              <VirtualHandsList rows={rows} mode={mode}/>
            )}

      {history.hasNextPage && !history.isLoading && !history.isError && visible.length > 0 && (
        <div className="hands-more" ref={sentinel}>
          <Button variant="outline" size="sm" disabled={history.isFetchingNextPage}
                  onClick={() => void history.fetchNextPage()}>
            {history.isFetchingNextPage ? 'Carregando mais mãos…' : 'Carregar mais mãos'}
          </Button>
          <span className={`hands-more-track${history.isFetchingNextPage ? ' is-loading' : ''}`}
                role="status" aria-label={history.isFetchingNextPage ? 'Carregando mais mãos' : ''}/>
        </div>
      )}

      <MyHandSharesPanel/>
    </AppPageBody>
  </AppPage></TermsGate>;
}
