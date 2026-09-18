'use client';
import React, {useLayoutEffect, useRef, useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {measureElement, useWindowVirtualizer} from '@tanstack/react-virtual';
import {Crown, ListChecks, Sparkles} from 'lucide-react';
import type {BoardScope, Entry, LeaderboardMetric, LeaderboardPeriod} from '@/lib/api/gamification';
import {
  LEADERBOARD_STALE_MS,
  MIN_HANDS_FOR_WIN_RATE,
  leaderboard,
  leaderboardKey,
  myRank,
  myRankKey
} from '@/lib/api/gamification';
import {getViewerId, playerName} from '@/lib/utils';
import {useOptionalSession} from "@/lib/auth/session";
import {CurrencyModeTabs} from '@/components/CurrencyModeTabs';
import {SkeletonList} from '@/components/ui/skeleton';
import {Button} from '@/components/ui/button';
import {RecoveryState} from '@/components/RecoveryState';
import type {WalletMode} from '@/lib/api/player';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';
import {FilterGroup} from '@/components/FilterGroup';

// The intro stagger reads as motion only while the eye can still follow each
// row landing; past ~10 it is just latency before the list is usable. Rows
// beyond that (and every row scrolled into view later) arrive without delay.
const MAX_STAGGERED_ROWS = 10;

const PERIODS = [
  {value: 'month', label: 'Este mês'},
  {value: 'all', label: 'Geral'}
] as const satisfies readonly {value: LeaderboardPeriod; label: string}[];

const METRICS = [
  {value: 'hands_won', label: 'Vitórias'},
  {value: 'hands_played', label: 'Mãos jogadas'},
  {value: 'win_rate', label: 'Aproveitamento'}
] as const satisfies readonly {value: LeaderboardMetric; label: string}[];

const wins = (entry: Entry) => `${entry.hands_won} ${entry.hands_won === 1 ? 'vitória' : 'vitórias'}`;
const played = (entry: Entry) => `${entry.hands_played} ${entry.hands_played === 1 ? 'mão' : 'mãos'}`;
const rate = (entry: Entry) => `${(entry.win_rate * 100).toFixed(1)}% de aproveitamento`;

// The number the board is sorted by leads each row; the other two follow. A
// list ordered by one metric while emphasizing another reads as unsorted.
function figures(entry: Entry, metric: LeaderboardMetric) {
  switch (metric) {
    case 'hands_played':
      return {lead: played(entry), support: wins(entry), aside: rate(entry)};
    case 'win_rate':
      return {lead: rate(entry), support: wins(entry), aside: `${played(entry)} jogadas`};
    default:
      return {lead: wins(entry), support: rate(entry), aside: `${played(entry)} jogadas`};
  }
}

// The month label comes from São Paulo's clock because the server buckets it
// there (leaderboard/period.go): in UTC the label would flip three hours early
// on the last night of the month.
const monthLabel = () => new Intl.DateTimeFormat('pt-BR', {month: 'long', timeZone: 'America/Sao_Paulo'})
  .format(new Date());

// An empty board means something different per filter, and the message is the
// only place the player learns which: a brand-new month, a floor they have not
// crossed yet, or a game nobody has played at all.
function emptyBoardMessage(period: LeaderboardPeriod, metric: LeaderboardMetric, month: string) {
  if (metric === 'win_rate') {
    return `Ninguém completou ${MIN_HANDS_FOR_WIN_RATE} mãos ${period === 'month' ? `em ${month}` : 'ainda'}, o mínimo para o ranking de aproveitamento.`;
  }
  return period === 'month'
    ? `Nenhuma mão contabilizada em ${month} ainda. A primeira mesa do mês abre o ranking.`
    : 'Nenhum jogador pontuou ainda. A primeira mesa inicia o Hall da Fama.';
}

function RankingRow({entry, rank, viewer, metric}: {
  entry: Entry; rank: number; viewer?: string; metric: LeaderboardMetric
}) {
  const isViewer = entry.player_id === viewer;
  const {lead, support, aside} = figures(entry, metric);
  return <>
    <b>{String(rank).padStart(2, '0')}</b>
    <span>
      {playerName(entry.player_id, viewer, entry.player_name)}{isViewer && ' (Você)'}
      <small>{aside}</small>
    </span>
    <strong>
      {lead}
      <small>{support}</small>
    </strong>
  </>;
}

// The community board can run to hundreds of ranked players; render only the
// rows near the viewport, the same window-virtualization the hand history
// list uses, so a 500-row season still scrolls at 60fps.
function VirtualRankingList({entries, startRank, viewer, metric}: {
  entries: Entry[]; startRank: number; viewer?: string; metric: LeaderboardMetric
}) {
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
    count: entries.length,
    estimateSize: () => 74,
    measureElement: (element, entry, instance) => measureElement(element, entry, instance) || 74,
    overscan: 6,
    scrollMargin
  });

  return <div
    ref={listRef}
    className="ranking-list is-virtualized"
    role="list"
    aria-label={`Ranking, posições ${startRank} em diante`}
    style={{height: virtualizer.getTotalSize()}}
  >
    {virtualizer.getVirtualItems().map(virtualRow => {
      const entry = entries[virtualRow.index];
      const isViewer = entry.player_id === viewer;
      return <article
        key={entry.player_id}
        ref={virtualizer.measureElement}
        data-index={virtualRow.index}
        role="listitem"
        aria-posinset={startRank + virtualRow.index}
        aria-setsize={entries.length}
        className={isViewer ? 'ranking-row-virtual viewer' : 'ranking-row-virtual'}
        style={{
          transform: `translateY(${virtualRow.start - scrollMargin}px)`,
          '--delay': `${Math.min(virtualRow.index, MAX_STAGGERED_ROWS) * 40}ms`
        } as React.CSSProperties}
      >
        <RankingRow entry={entry} rank={startRank + virtualRow.index} viewer={viewer} metric={metric}/>
      </article>;
    })}
  </div>;
}

export default function Ranking() {
  const [mode, setMode] = useState<WalletMode>('sandbox');
  // The month is the default board: a lifetime ranking a new player can never
  // reach is a scoreboard, not a competition. "Geral" is one tab away.
  const [period, setPeriod] = useState<LeaderboardPeriod>('month');
  const [metric, setMetric] = useState<LeaderboardMetric>('hands_won');
  const scope: BoardScope = {period, metric};
  const {data = [], isLoading, isError, refetch} = useQuery({
    queryKey: leaderboardKey(mode, scope),
    queryFn: () => leaderboard(mode, undefined, scope),
    staleTime: LEADERBOARD_STALE_MS
  });
  const viewer = getViewerId();
  const {authed} = useOptionalSession();
  const {data: rankInfo} = useQuery({
    queryKey: myRankKey(mode, scope),
    queryFn: () => myRank(mode, scope),
    staleTime: LEADERBOARD_STALE_MS,
    enabled: authed
  });
  const month = monthLabel();
  const periodLabel = period === 'month' ? `em ${month}` : 'desde o primeiro dia';

  const topThree = data.slice(0, 3);
  const hasPodium = topThree.length >= 3;
  const listEntries = hasPodium ? data.slice(3) : data;
  const listStartRank = hasPodium ? 4 : 1;

  // A board that failed to load is a trust moment, not a stray error line: it
  // gets the shared recovery composition with its own h1, in place of the
  // page heading, so the landmark and heading still survive the state.
  if (isError) {
    return <AppPage authed={authed} current="leaderboard">
      <AppPageBody className="ranking">
        <RecoveryState
          nested
          title="Não foi possível carregar o ranking agora"
          description="O Hall da Fama está calculado no servidor e continua intacto. Tente novamente em instantes."
          action={<Button type="button" onClick={() => void refetch()}><ListChecks aria-hidden="true"/> Tentar novamente</Button>}
        />
      </AppPageBody>
    </AppPage>;
  }

  return (
    <AppPage authed={authed} current="leaderboard">
      <AppPageBody className="ranking">
        <AppPageHeader
          icon={Crown}
          eyebrow="HALL DA FAMA"
          title="Ranking da comunidade"
          description={period === 'month'
            ? `Desempenho auditável nas mesas do CTech Poker, contado a partir da primeira mão de ${month}.`
            : 'Desempenho auditável nas mesas do CTech Poker, somando todas as mãos já jogadas.'}
        />
        <div className="ranking-filters">
          <CurrencyModeTabs mode={mode} onChangeAction={setMode} showLabel/>
          <FilterGroup label="Período" value={period} options={PERIODS} onChangeAction={setPeriod} showLabel/>
          <FilterGroup label="Ordenar por" value={metric} options={METRICS} onChangeAction={setMetric} showLabel/>
        </div>

        {authed && rankInfo && (
          <div className="viewer-ranking-card" aria-label="Sua posição atual">
            <Sparkles aria-hidden="true"/>
            {rankInfo.ranked && rankInfo.entry ? (
              <>
                <div>
                  <span>Sua posição {periodLabel}</span>
                  <strong>#{rankInfo.rank} de {rankInfo.total} {rankInfo.total === 1 ? 'jogador' : 'jogadores'}</strong>
                </div>
                <div className="viewer-ranking-stats">
                  <span><b>{rankInfo.entry.hands_won}</b> vitórias</span>
                  <span><b>{rankInfo.entry.hands_played}</b> mãos</span>
                  <span><b>{(rankInfo.entry.win_rate * 100).toFixed(1)}%</b> de aproveitamento</span>
                </div>
              </>
            ) : (
              <div>
                <span>Sua posição {periodLabel}</span>
                <strong>Ainda sem ranking</strong>
                {/* Absence on this board has two different causes, and the
                    player can only act on the one they are actually in: the
                    win_rate board has a minimum-hands floor, every other board
                    just needs a first hand in the period. */}
                <p className="viewer-ranking-hint">{metric === 'win_rate'
                  ? `O ranking de aproveitamento começa em ${MIN_HANDS_FOR_WIN_RATE} mãos ${periodLabel} — ${
                    rankInfo.entry ? `você tem ${rankInfo.entry.hands_played}.` : 'nenhuma até agora.'}`
                  : `Jogue uma mão nesta modalidade ${periodLabel} para entrar no ranking.`}</p>
              </div>
            )}
          </div>
        )}

        {isLoading ?
          <SkeletonList label="Buscando o ranking da comunidade…" count={6} height={62}
                        className="ranking-list skeleton-panel"/> :
          !data.length ?
            <div className="lobby-empty">{emptyBoardMessage(period, metric, month)}</div> :
            <>
              {hasPodium && (
                <div className="leaderboard-podium" aria-label="Pódio do ranking">
                  {topThree.map((player, index) => {
                    const rank = index + 1;
                    const isViewer = player.player_id === viewer;
                    return (
                      <article key={player.player_id}
                               className={`podium-card rank-${rank}${isViewer ? ' viewer' : ''}`}>
                        {rank === 1 && <Crown className="podium-crown" aria-hidden="true"/>}
                        <span className="podium-badge">{rank}º Lugar</span>
                        <strong className="podium-name">
                          {playerName(player.player_id, viewer, player.player_name)}
                          {isViewer && ' (Você)'}
                        </strong>
                        <div className="podium-stats">
                          <span><strong>{figures(player, metric).lead}</strong></span>
                          <span>{figures(player, metric).support}</span>
                        </div>
                      </article>
                    );
                  })}
                </div>
              )}

              {listEntries.length > 0 &&
                <VirtualRankingList entries={listEntries} startRank={listStartRank} viewer={viewer} metric={metric}/>}
            </>}
      </AppPageBody>
    </AppPage>
  );
}
