'use client';
import {useEffect, useRef, useState} from 'react';
import {Trophy} from 'lucide-react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {getTodayHighlight} from '@/lib/api/highlights';
import {HAND_CATEGORY_LABELS} from '@/lib/handCategories';
import {bestHandCategory, compareHands} from '@/lib/pokerRules';

const CARD_CODE = /^[2-9TJQKA][CDHS]$/i;

export function highlightWinnerLabel(board?: string[], revealed?: Array<{name?: string; hole_cards: string[]}>) {
  if (board?.length !== 5 || new Set(board.map(card => card.toUpperCase())).size !== 5 ||
    board.some(card => !CARD_CODE.test(card))) return undefined;
  const candidates = (revealed || []).filter(hand => hand.hole_cards.length === 2 &&
    hand.hole_cards.every(card => CARD_CODE.test(card)) &&
    new Set([...board, ...hand.hole_cards].map(card => card.toUpperCase())).size === 7);
  if (candidates.length === 0) return undefined;
  const best = candidates.reduce((winner, hand) =>
    compareHands([...hand.hole_cards, ...board], [...winner.hole_cards, ...board]) > 0 ? hand : winner);
  const tied = candidates.filter(hand =>
    compareHands([...hand.hole_cards, ...board], [...best.hole_cards, ...board]) === 0);
  const names = tied.map(hand => hand.name || 'Jogador').join(' e ');
  const category = HAND_CATEGORY_LABELS[bestHandCategory([...best.hole_cards, ...board])];
  return category ? `${names} — ${category}` : names;
}

// System-detected "biggest pot of the day" for this table — no player action
// required, distinct from the manual, player-initiated hand-share flow.
// Fetched once on mount; re-fetched (via invalidateQueries, not polling) the
// moment a hand this viewer was watching completes, so a bigger pot from
// this table shows up without a page reload.
export function TodayHighlight({tableId, handId, handComplete}: {
  tableId: string;
  handId?: string;
  handComplete: boolean;
}) {
  const queryClient = useQueryClient();
  const {data} = useQuery({
    queryKey: ['highlights', tableId, 'today'],
    queryFn: () => getTodayHighlight(tableId),
  });
  const lastHandId = useRef<string | undefined>(undefined);
  const [expanded, setExpanded] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!handComplete || !handId || handId === lastHandId.current) return;
    lastHandId.current = handId;
    queryClient.invalidateQueries({queryKey: ['highlights', tableId, 'today']});
  }, [handComplete, handId, queryClient, tableId]);

  // Narrow phones shrink the pill down to the trophy icon (see .today-highlight
  // in globals.css) since the label + pot + revealed hand text no longer fit
  // beside the table's utility icons; tapping it re-opens what CSS hid rather
  // than losing the information outright.
  useEffect(() => {
    if (!expanded) return undefined;
    function onPointerDown(e: PointerEvent) {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setExpanded(false);
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setExpanded(false);
    }
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [expanded]);

  if (!data?.pot) return null;
  const revealedText = highlightWinnerLabel(data.board, data.revealed);
  return (
    <div className={`today-highlight-wrap ${expanded ? 'expanded' : ''}`} ref={wrapRef}>
      <button type="button" className="today-highlight" aria-expanded={expanded}
              onClick={() => setExpanded(v => !v)}>
        <Trophy aria-hidden="true"/>
        <span className="today-highlight-label">Maior pote de hoje</span>
        <span className="today-highlight-pot">{data.pot.toLocaleString('pt-BR')}</span>
        {revealedText && <span className="today-highlight-cards">{revealedText}</span>}
      </button>
    </div>
  );
}
