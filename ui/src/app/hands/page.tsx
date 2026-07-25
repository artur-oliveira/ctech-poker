'use client';
import Link from 'next/link';
import {useQuery} from '@tanstack/react-query';
import {BookOpen, ChevronLeft, ChevronRight, Club, History, Trophy} from 'lucide-react';
import {getHands} from '@/lib/api/player';
import {PlayingCard} from '@/components/table/PlayingCard';
import {OutcomeBadge} from '@/components/hands/OutcomeBadge';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';
import {TermsGate} from '@/components/TermsGate';
import {bestHandCategory} from '@/lib/pokerRules';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';
import React from "react";

function formatDate(unixSeconds: number) {
  return new Date(unixSeconds * 1000).toLocaleString('pt-BR', {
    day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit'
  });
}

function truncateSeed(hex: string) {
  return `${hex.slice(0, 8)}…`;
}

// Server only sends a category on live table state, never on hand history —
// resolvable client-side whenever the full 2 hole + 5 board cards are known.
function handCategoryLabel(holeCards?: string[], board?: string[]): string | null {
  if (holeCards?.length !== 2 || board?.length !== 5) return null;
  return HAND_CATEGORY_LABELS[bestHandCategory([...holeCards, ...board])] || null;
}

export default function HandsHistory() {
  const {data = [], isLoading, isError, refetch} = useQuery({queryKey: ['hands'], queryFn: getHands});

  return <TermsGate>
    <main className="app-page">
      <nav className="app-nav shell">
        <Link href="/" className="brand"><span className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
        <div className="header-right">
          <Link href="/guide"><BookOpen/> <span className="header-right-label">Guia</span></Link>
          <Link href="/leaderboard"><Trophy/> <span className="header-right-label">Ranking</span></Link>
          <ProfileMenu/>
        </div>
      </nav>
      <section className="ranking hands shell">
        <Link href="/lobby"><ChevronLeft/> Lobby</Link>
        <header>
          <History aria-hidden="true"/><small>SEU HISTÓRICO</small>
          <h1>Mãos jogadas</h1>
          <p>As últimas 50 mãos que você jogou, com suas cartas, o board e a prova de integridade de cada baralho.</p>
        </header>
        {isLoading ? <div className="lobby-empty"><span className="loader"/>Buscando suas mãos…</div> :
          isError ? <div className="lobby-empty">Não foi possível carregar seu histórico agora.
              <button type="button" className="link-retry" onClick={() => void refetch()}>Tentar novamente</button>
            </div> :
            !data.length ? <div className="lobby-empty">Você ainda não jogou nenhuma mão. Elas aparecem aqui assim que
                uma mesa termina.</div> :
              <div className="hands-list">
                {data.map((hand, i) => <Link key={hand.hand_id}
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
              </div>}
      </section>
    </main>
  </TermsGate>;
}
