'use client';
import React, {useLayoutEffect, useRef, useState} from 'react';
import {useQuery} from '@tanstack/react-query';
import {measureElement, useWindowVirtualizer} from '@tanstack/react-virtual';
import {Crown, ListChecks, Sparkles} from 'lucide-react';
import type {Entry} from '@/lib/api/gamification';
import {leaderboard, myRank} from '@/lib/api/gamification';
import {getViewerId, playerName} from '@/lib/utils';
import {useOptionalSession} from "@/lib/auth/session";
import {CurrencyModeTabs} from '@/components/CurrencyModeTabs';
import {SkeletonList} from '@/components/ui/skeleton';
import {Button} from '@/components/ui/button';
import {RecoveryState} from '@/components/RecoveryState';
import type {WalletMode} from '@/lib/api/player';
import {AppPage, AppPageBody, AppPageHeader} from '@/components/AppPageChrome';

// The intro stagger reads as motion only while the eye can still follow each
// row landing; past ~10 it is just latency before the list is usable. Rows
// beyond that (and every row scrolled into view later) arrive without delay.
const MAX_STAGGERED_ROWS = 10;

function RankingRow({entry, rank, viewer}: {entry: Entry; rank: number; viewer?: string}) {
  const isViewer = entry.player_id === viewer;
  return <>
    <b>{String(rank).padStart(2, '0')}</b>
    <span>
      {playerName(entry.player_id, viewer, entry.player_name)}{isViewer && ' (Você)'}
      <small>{entry.hands_played} mãos jogadas</small>
    </span>
    <strong>
      {entry.hands_won} vitórias
      <small>{(entry.win_rate * 100).toFixed(1)}% de aproveitamento</small>
    </strong>
  </>;
}

// The community board can run to hundreds of ranked players; render only the
// rows near the viewport, the same window-virtualization the hand history
// list uses, so a 500-row season still scrolls at 60fps.
function VirtualRankingList({entries, startRank, viewer}: {
  entries: Entry[]; startRank: number; viewer?: string
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
        <RankingRow entry={entry} rank={startRank + virtualRow.index} viewer={viewer}/>
      </article>;
    })}
  </div>;
}

export default function Ranking() {
  const [mode, setMode] = useState<WalletMode>('sandbox');
  const {data = [], isLoading, isError, refetch} = useQuery({
    queryKey: ['leaderboard', mode],
    queryFn: () => leaderboard(mode)
  });
  const viewer = getViewerId();
  const {authed} = useOptionalSession();
  const {data: rankInfo} = useQuery({
    queryKey: ['leaderboard-me', mode],
    queryFn: () => myRank(mode),
    enabled: authed
  });

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
          description="Desempenho auditável baseado em vitórias e mãos jogadas nas mesas do CTech Poker."
        />
        <CurrencyModeTabs mode={mode} onChangeAction={setMode}/>

        {authed && rankInfo && (
          <div className="viewer-ranking-card" aria-label="Sua posição atual">
            <Sparkles aria-hidden="true"/>
            {rankInfo.ranked && rankInfo.entry ? (
              <>
                <div>
                  <span>Sua posição no ranking</span>
                  <strong>#{rankInfo.rank} de {rankInfo.total} {rankInfo.total === 1 ? 'jogador' : 'jogadores'}</strong>
                </div>
                <div className="viewer-ranking-stats">
                  <span><b>{rankInfo.entry.hands_won}</b> vitórias</span>
                  <span><b>{(rankInfo.entry.win_rate * 100).toFixed(1)}%</b> de taxa de vitória</span>
                </div>
              </>
            ) : (
              <div>
                <span>Sua posição no ranking</span>
                <strong>Ainda sem ranking</strong>
                <p className="viewer-ranking-hint">Jogue uma mão nesta modalidade para entrar no ranking.</p>
              </div>
            )}
          </div>
        )}

        {isLoading ?
          <SkeletonList label="Buscando o ranking da comunidade…" count={6} height={62}
                        className="ranking-list skeleton-panel"/> :
          !data.length ?
            <div className="lobby-empty">Nenhum jogador pontuou ainda. A primeira mesa inicia o Hall da Fama.</div> :
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
                          <span><strong>{player.hands_won}</strong> vitórias</span>
                          <span>{player.hands_played} mãos ({(player.win_rate * 100).toFixed(1)}%)</span>
                        </div>
                      </article>
                    );
                  })}
                </div>
              )}

              {listEntries.length > 0 &&
                <VirtualRankingList entries={listEntries} startRank={listStartRank} viewer={viewer}/>}
            </>}
      </AppPageBody>
    </AppPage>
  );
}
