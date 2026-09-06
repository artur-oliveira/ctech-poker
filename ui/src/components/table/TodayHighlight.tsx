'use client';
import {useEffect, useRef, useState} from 'react';
import {Trophy} from 'lucide-react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {getTodayHighlight, type TableHighlight} from '@/lib/api/highlights';
import {HAND_CATEGORY_LABELS} from '@/lib/handCategories';
import {bestHandCategory, compareHands} from '@/lib/pokerRules';
import {invalidateAfterSettle} from '@/lib/settleRefetch';

const CARD_CODE = /^[2-9TJQKA][CDHS]$/i;

// KNOWN LIMITATION: this names whoever holds the single best raw hand among
// `revealed`, not whoever actually won the largest share of `data.pot`. On a
// multi-way all-in with a side pot, the best hand at the table is only ever
// eligible for the pot layer it covers (see `contestedPots`/`playerPotBreakdown`
// in lib/tableOutcome.ts, which HandOutcome and the live standings already get
// right) — a short-stacked flush can win a small main pot while a worse hand
// takes a much bigger side pot between the two deeper stacks. `TableHighlight`
// (lib/api/highlights.ts) has no per-player payout on `revealed` to attribute
// the caption correctly here; fixing it needs that field added on the
// api/highlights.Store side first.
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
export function TodayHighlight({tableId, handId, handComplete, handPot}: {
  tableId: string;
  handId?: string;
  handComplete: boolean;
  /** The settled hand's contested pot (`highlightPot`). The server only
   *  overwrites today's row when a hand beats it, so a hand that cannot beat
   *  the pot already on display needs no read at all. */
  handPot?: number;
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
    if (!handComplete || !handId || handId === lastHandId.current) return undefined;
    lastHandId.current = handId;
    // The highlight row is written by the server's detached post-hand
    // pipeline, after this `complete` frame was already sent — hence the
    // backoff, so a bigger pot isn't shown a hand late. It stops as soon as
    // the answer can no longer change: either this hand *is* the highlight,
    // or the row on display already holds a pot this hand could not beat
    // (`RecordHand` only overwrites on a strictly bigger pot), in which case
    // not a single read is spent (#229).
    return invalidateAfterSettle<TableHighlight>(queryClient, ['highlights', tableId, 'today'], {
      settled: current => current?.hand_id === handId ||
        (handPot !== undefined && current !== undefined && current.pot >= handPot),
    });
  }, [handComplete, handId, handPot, queryClient, tableId]);

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
