'use client';
import React, {useMemo, useState} from 'react';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
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
  const {data = [], isLoading, isError, refetch} = useQuery({queryKey: ['hands'], queryFn: () => getHands()});
  const [filter, setFilter] = useState<HandFilter>('all');

  const stats = useMemo(() => {
    if (!data.length) return null;
    let netSum = 0;
    let winsCount = 0;
    for (const h of data) {
      netSum += h.net_change;
      if (h.outcome === 'won' || h.outcome === 'tied') winsCount++;
    }
    const winRate = Math.round((winsCount / data.length) * 100);
    return {totalHands: data.length, netSum, winsCount, winRate};
  }, [data]);

  const filteredHands = useMemo(() => {
    if (filter === 'wins') return data.filter(h => h.outcome === 'won' || h.outcome === 'tied');
    if (filter === 'losses') return data.filter(h => h.outcome === 'lost');
    return data;
  }, [data, filter]);

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
              <span className="stat-label">Mãos registradas</span>
              <strong className="stat-value">{stats.totalHands}</strong>
            </div>
            <div className="stat-card">
              <span className="stat-label">Balanço total</span>
              <strong className={`stat-value ${stats.netSum > 0 ? 'gain' : stats.netSum < 0 ? 'loss' : ''}`}>
                {stats.netSum > 0 ? '+' : ''}{stats.netSum.toLocaleString('pt-BR')}
              </strong>
            </div>
            <div className="stat-card">
              <span className="stat-label">Taxa de vitória</span>
              <strong className="stat-value">{stats.winRate}% <small>({stats.winsCount}V / {stats.totalHands - stats.winsCount}D)</small></strong>
            </div>
          </div>
        )}

        {!isLoading && !isError && data.length > 0 && (
          <div className="filter-tabs" role="tablist" aria-label="Filtro de mãos">
            <button
              type="button"
              role="tab"
              aria-selected={filter === 'all'}
              className={`filter-tab${filter === 'all' ? ' active' : ''}`}
              onClick={() => setFilter('all')}
            >
              Todas ({data.length})
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={filter === 'wins'}
              className={`filter-tab${filter === 'wins' ? ' active' : ''}`}
              onClick={() => setFilter('wins')}
            >
              Vitórias ({stats?.winsCount ?? 0})
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={filter === 'losses'}
              className={`filter-tab${filter === 'losses' ? ' active' : ''}`}
              onClick={() => setFilter('losses')}
            >
              Derrotas ({data.length - (stats?.winsCount ?? 0)})
            </button>
          </div>
        )}

        {isLoading ? <div className="lobby-empty"><span className="loader"/>Buscando seu histórico de mãos…</div> :
          isError ? <div className="lobby-empty">Não foi possível carregar seu histórico agora.
              <button type="button" className="link-retry" onClick={() => void refetch()}>Tentar novamente</button>
            </div> :
            !data.length ? <div className="lobby-empty">Você ainda não jogou nenhuma mão. As rodadas concluídas aparecerão aqui automaticamente.</div> :
              !filteredHands.length ? (
                <div className="lobby-empty">
                  <Sparkles aria-hidden="true"/>
                  <p>Nenhuma mão encontrada neste filtro.</p>
                  <Button variant="outline" size="sm" onClick={() => setFilter('all')}>Ver todas</Button>
                </div>
              ) : (
                <div className="hands-list">
                  {filteredHands.map((hand, i) => <Link key={hand.hand_id}
                                               href={`/hands/history?table_id=${hand.table_id}&hand_id=${encodeURIComponent(hand.sk)}`}
                                               className="hand-row"
                                               style={{'--delay': `${Math.min(i, 10) * 40}ms`} as React.CSSProperties}>
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
      </section>
    </main>
  </TermsGate>;
}
