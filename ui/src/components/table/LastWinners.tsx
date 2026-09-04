'use client';
import type {CSSProperties} from 'react';
import {useRef, useState} from 'react';
import Link from 'next/link';
import {Trophy} from 'lucide-react';
import type {HandItem} from '@/lib/api/player';
import {bestFiveCardHand, bestHandCategory} from '@/lib/pokerRules';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';
import {useDismiss} from '@/lib/hooks/useDismiss';
import {useHoverPanel} from '@/lib/hooks/useHoverPanel';
import {PlayingCard} from '@/components/table/PlayingCard';
import {PlayerAvatar} from '@/components/ui/player-avatar';

export type WinnerLogEntry = {
  key: string;
  names: string[];
  avatarUrls: Array<string | undefined>;
  category?: string;
  cards?: string[]
};

// `/players/me/hands` only ever carries the viewer's own perspective, so a
// hand's "winner(s)" are the viewer (outcome won/tied) plus whichever
// opponents the server flagged `won`, so a split pot lists more than one name.
// A `lost` hand never puts the viewer in that list. The winning combo is
// read off whichever winner actually has hole cards on record (the viewer
// always does; an opponent only when the hand went to showdown) combined
// with the board, mirroring the derivation HandOutcomeBanner already does.
export function deriveWinners(items: HandItem[], limit = 5): WinnerLogEntry[] {
  return items.slice(0, limit).map(item => {
    const winners: { name: string; hole?: string[]; avatarUrl?: string }[] = [];
    if (item.outcome !== 'lost') winners.push({name: 'Você', hole: item.hole_cards});
    for (const opp of item.opponents || []) if (opp.won) winners.push({
      name: opp.name || 'Visitante',
      hole: opp.hole_cards,
      avatarUrl: opp.avatar_url
    });
    const board = item.board?.length === 5 ? item.board : undefined;
    const withHole = winners.find(w => w.hole?.length === 2);
    const cards = withHole?.hole && board ? bestFiveCardHand([...withHole.hole, ...board]) : withHole?.hole;
    const category = cards?.length === 5 ? bestHandCategory(cards) : undefined;
    return {
      key: item.hand_id,
      names: winners.map(w => w.name),
      avatarUrls: winners.map(w => w.avatarUrl),
      category,
      cards
    };
  }).filter(entry => entry.names.length > 0);
}

/** Floating toggle mirroring Chat's affordance (bottom-left instead of
 * bottom-right). Shows the last 5 resolved hands at this table, newest first,
 * sourced from the player's own hand-history endpoint rather than live
 * socket state, so it's populated the moment the table loads instead of
 * only after the viewer sits through a fresh resolution. It can be
 * controlled by the table page so opening it closes chat, reactions, or
 * hand rankings instead of stacking multiple mobile overlays. */
export function LastWinners({items, tableId, open: controlledOpen, onOpenChangeAction}: {
  items: HandItem[];
  tableId: string;
  open?: boolean;
  onOpenChangeAction?: (open: boolean) => void;
}) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
  const open = controlledOpen ?? uncontrolledOpen;
  const setOpen = (nextOpen: boolean) => {
    if (controlledOpen === undefined) setUncontrolledOpen(nextOpen);
    onOpenChangeAction?.(nextOpen);
  };
  const winners = deriveWinners(items);
  const asideRef = useRef<HTMLElement>(null);
  useDismiss(asideRef, open, () => setOpen(false));
  const {toggleFromClick, ...hover} = useHoverPanel(setOpen, true, open);
  if (!winners.length) return null;
  return <aside ref={asideRef} className={`last-winners table-aside-skirt ${open ? 'open' : ''}`}
                aria-label="Últimos vencedores da mesa" {...hover}>
    <button type="button" className="last-winners-toggle" aria-expanded={open} aria-controls="last-winners-panel"
            aria-label={open ? 'Fechar últimos vencedores' : 'Ver últimos vencedores'}
            onClick={() => toggleFromClick(open)}>
      <Trophy aria-hidden="true"/>
    </button>
    <div id="last-winners-panel" className="last-winners-panel">
      <h2>Últimos vencedores</h2>
      <ul>
        {winners.map((entry, i) => <li key={entry.key} style={{'--stagger-index': i} as CSSProperties}>
          <Link href={`/hands/replay?table_id=${encodeURIComponent(tableId)}&hand_id=${encodeURIComponent(entry.key)}&mode=sandbox`}
                target="_blank" className="last-winners-row"
                aria-label={`Assistir replay: ${entry.names.join(' e ')}`}>
            <span className="last-winners-cards">
              {entry.cards ? entry.cards.map((card, ci) => <PlayingCard key={card} card={card} index={ci} size="hole"/>) :
                [0, 1].map(ci => <PlayingCard key={ci} index={ci} size="hole"/>)}
            </span>
            <span className="last-winners-info">
              {entry.avatarUrls.map((avatarUrl, wi) => avatarUrl && <PlayerAvatar key={avatarUrl}
                                                                                  name={entry.names[wi]}
                                                                                  avatarUrl={avatarUrl} size={24}
                                                                                  decorative/>)}
              <b>{entry.names.join(' e ')}</b>
              {entry.category && <small>{HAND_CATEGORY_LABELS[entry.category] || entry.category}</small>}
            </span>
          </Link>
        </li>)}
      </ul>
    </div>
  </aside>;
}
