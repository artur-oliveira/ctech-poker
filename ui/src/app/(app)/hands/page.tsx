'use client';
import React, {memo, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import Link from 'next/link';
import {useInfiniteQuery, useQuery, useQueryClient} from '@tanstack/react-query';
import {measureElement, useWindowVirtualizer} from '@tanstack/react-virtual';
import {
  AlertCircle,
  ArrowRight,
  BookmarkPlus,
  ChevronRight,
  FolderHeart,
  History,
  Infinity as InfinityIcon,
  LockKeyhole,
  ShieldCheck
} from 'lucide-react';
import type {HandItem, WalletMode} from '@/lib/api/player';
import {getHands, handEndedAtMs} from '@/lib/api/player';
import {
  getSavedHandFilters, HAND_COLLECTIONS_KEY, listHandCollections, SAVED_HAND_FILTERS_KEY,
  saveSavedHandFilters, type SavedHandFilter
} from '@/lib/api/handMeta';
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
import {LEADERBOARD_STALE_MS, myRank, myRankKey} from '@/lib/api/gamification';
import {
  ALL_TABLES, filterHands, groupHandsByDay, handTables, type HandsFilter, type HandsRow, loadedTotals,
  NO_FILTER, type OutcomeFilter, shortTableId
} from '@/lib/handsHistory';

// The virtual "Marcadas para revisar" collection materializes #349's
// review-marker toggle here, per that issue's own dependency note — it is
// not a name a player can type themselves, so it gets a sentinel value
// distinct from any real collection name.
const REVIEW_COLLECTION = '__review__';

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
    // No focus refetch here: React Query refetches EVERY loaded page, so with
    // 18 pages loaded tabbing back into the window was 18 requests — the
    // "carregou mais sozinho" report. That is now the HISTORY_QUERY preset the
    // `['hands']` key inherits (`lib/queryFreshness.ts`), not a local override.
  });

  const hands = useMemo(
    () => (history.data?.pages ?? []).flatMap(page => page.data),
    [history.data]
  );

  const [filter, setFilter] = useState<HandsFilter>(NO_FILTER);
  const queryClient = useQueryClient();

  // #347: "Filtros" (outcome/table, ad-hoc or saved) and "Coleções" (hands
  // marked individually, from /hands or /hands/history — the same
  // review-marker/collections record #349 writes) are two different lenses
  // over the same loaded pages, never combined.
  const [activeTab, setActiveTab] = useState<'filters' | 'collections'>('filters');
  const savedFilters = useQuery({queryKey: SAVED_HAND_FILTERS_KEY, queryFn: getSavedHandFilters});
  const collections = useQuery({queryKey: HAND_COLLECTIONS_KEY, queryFn: listHandCollections});
  const collectionNames = useMemo(() => {
    const names = new Set<string>();
    let hasReview = false;
    for (const meta of collections.data ?? []) {
      for (const name of meta.collections ?? []) names.add(name);
      if (meta.review_marked) hasReview = true;
    }
    return [...(hasReview ? [REVIEW_COLLECTION] : []), ...[...names].sort()];
  }, [collections.data]);
  const [activeCollection, setActiveCollection] = useState<string | null>(null);
  // Derived during render rather than a useEffect: opening the tab (or the
  // first page of collections arriving) should pick a default synchronously,
  // not one render late.
  if (activeTab === 'collections' && !activeCollection && collectionNames.length) {
    setActiveCollection(collectionNames[0]);
  }
  const collectionHandIds = useMemo(() => {
    if (!activeCollection) return null;
    const ids = new Set<string>();
    for (const meta of collections.data ?? []) {
      if (activeCollection === REVIEW_COLLECTION ? meta.review_marked : meta.collections?.includes(activeCollection)) {
        ids.add(meta.hand_id);
      }
    }
    return ids;
  }, [collections.data, activeCollection]);

  const [savingFilterName, setSavingFilterName] = useState('');
  const [savingFilter, setSavingFilter] = useState(false);
  const [filterSaveError, setFilterSaveError] = useState<string | null>(null);
  async function saveCurrentFilter() {
    const name = savingFilterName.trim();
    if (!name) return;
    setSavingFilter(true);
    setFilterSaveError(null);
    try {
      const next: SavedHandFilter[] = [
        ...(savedFilters.data ?? []).filter(saved => saved.name !== name),
        {name, outcome: filter.outcome, table_id: filter.tableId},
      ];
      const updated = await saveSavedHandFilters(next);
      queryClient.setQueryData(SAVED_HAND_FILTERS_KEY, updated);
      setSavingFilterName('');
    } catch {
      setFilterSaveError('Não foi possível salvar o filtro. Tente novamente.');
    } finally {
      setSavingFilter(false);
    }
  }
  async function deleteSavedFilter(name: string) {
    try {
      const updated = await saveSavedHandFilters((savedFilters.data ?? []).filter(saved => saved.name !== name));
      queryClient.setQueryData(SAVED_HAND_FILTERS_KEY, updated);
    } catch {
      // Leave the list as-is; the player can retry the same removal.
    }
  }

  const visible = useMemo(() => {
    if (activeTab === 'collections') return collectionHandIds ? hands.filter(h => collectionHandIds.has(h.hand_id)) : [];
    return filterHands(hands, filter);
  }, [hands, filter, activeTab, collectionHandIds]);
  const rows = useMemo(() => groupHandsByDay(visible), [visible]);
  const tables = useMemo(() => handTables(hands), [hands]);
  const stats = useMemo(() => loadedTotals(visible), [visible]);

  // Lifetime W/L comes from the leaderboard's own counters, the only totals
  // that describe every hand ever played instead of the pages loaded here.
  // `ranked: false` means "no stats row yet", not an error.
  const lifetime = useQuery({queryKey: myRankKey(mode), queryFn: () => myRank(mode), staleTime: LEADERBOARD_STALE_MS});

  const fetchNextPage = history.fetchNextPage;

  // Auto-load only makes sense on the unfiltered list. Under a filter (or the
  // Coleções tab, always a restrictive subset) the visible list stays short
  // no matter how many pages arrive, so the sentinel never leaves the
  // viewport and every appended page immediately triggers the next one — the
  // whole history downloaded in one cascade. Filtered lists keep the
  // explicit "Carregar mais mãos" button instead.
  const autoLoad = activeTab === 'filters' && filter.outcome === 'all' && filter.tableId === ALL_TABLES;

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
          label="Modo de visualização"
          value={activeTab}
          options={[
            {value: 'filters', label: 'Filtros'},
            {value: 'collections', label: 'Coleções'},
          ] as const}
          onChangeAction={setActiveTab}
        />
        {activeTab === 'filters' ? <>
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
          <div className="hands-saved-filters">
            {(savedFilters.data ?? []).length > 0
              ? <div className="filter-tabs filter-tabs-scroll" role="group" aria-label="Filtros salvos">
                {(savedFilters.data ?? []).map(saved => <span key={saved.name} className="hands-saved-filter">
                  <button type="button" className="filter-tab"
                          onClick={() => setFilter({outcome: saved.outcome as OutcomeFilter, tableId: saved.table_id})}>
                    {saved.name}
                  </button>
                  <button type="button" className="hands-saved-filter-remove" aria-label={`Remover filtro ${saved.name}`}
                          onClick={() => void deleteSavedFilter(saved.name)}>×</button>
                </span>)}
              </div>
              : <p className="hands-saved-filters-empty">Nenhum filtro salvo ainda.</p>}
            <form className="hands-save-filter-form"
                  onSubmit={e => {
                    e.preventDefault();
                    void saveCurrentFilter();
                  }}>
              <label htmlFor="new-saved-filter-name">Salvar filtro atual como</label>
              <input id="new-saved-filter-name" value={savingFilterName} maxLength={40}
                     placeholder="Ex.: Minhas bad beats"
                     onChange={e => setSavingFilterName(e.target.value)}/>
              <Button type="submit" variant="outline" size="sm" disabled={savingFilter || !savingFilterName.trim()}>
                <BookmarkPlus aria-hidden="true"/> Salvar
              </Button>
            </form>
            {filterSaveError && <p className="form-error" role="alert">{filterSaveError}</p>}
          </div>
        </> : (
          collectionNames.length === 0
            ? <p className="hands-saved-filters-empty">
              Nenhuma coleção ainda. Marque uma mão como &quot;para revisar&quot; ou adicione-a a uma coleção pelo
              detalhe da mão.
            </p>
            : <div className="filter-tabs filter-tabs-scroll" role="group" aria-label="Coleções">
              {collectionNames.map(name => <button key={name} type="button"
                                                    className={`filter-tab${activeCollection === name ? ' active' : ''}`}
                                                    aria-pressed={activeCollection === name}
                                                    onClick={() => setActiveCollection(name)}>
                <FolderHeart aria-hidden="true"/> {name === REVIEW_COLLECTION ? 'Marcadas para revisar' : name}
              </button>)}
            </div>
        )}
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
                <strong>{activeTab === 'collections' ? 'Esta coleção está vazia' : 'Nenhuma mão com esse filtro'}</strong>
                <p>As {hands.length} mãos carregadas continuam aqui.
                  {activeTab === 'collections'
                    ? ' Marque uma mão nesta lista ou no detalhe da mão para adicioná-la aqui.'
                    : ' Solte o filtro para vê-las de novo.'}</p>
              </div>
              {activeTab === 'filters' &&
                  <Button variant="outline" size="sm" onClick={() => setFilter(NO_FILTER)}>Limpar filtros</Button>}
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
