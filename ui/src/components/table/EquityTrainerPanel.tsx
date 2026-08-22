'use client';
import {useRef, useState} from 'react';
import {Lightbulb} from 'lucide-react';
import type {SeatView} from '@/lib/api/table';
import {explainEquity, matchingCards} from '@/lib/equityTrainer';
import {useDismiss} from '@/lib/hooks/useDismiss';
import {HAND_CATEGORY_LABELS} from '@/lib/utils';
import {PlayingCard} from '@/components/table/PlayingCard';

const STAGE_LABELS: Record<string, string> = {
  preflop: 'Pré-flop', flop: 'Flop', turn: 'Turn', river: 'River', showdown: 'Showdown', complete: 'Showdown'
};

/** Sandbox-only teaching overlay built entirely on the viewer's own already-
 * authorized equity/hand_category (same fields Seat.tsx already renders as a
 * bare number) — no opponent simulation, no server change. Mirrors
 * LastWinners' fixed toggle/panel vocabulary and controlled open contract so
 * it slots into the same activeTablePanel exclusivity as chat/reactions/
 * winners/rankings. */
export function EquityTrainerPanel({seat, isViewer, currencyMode, board, stage, handId, handComplete, isTurn,
                                     open: controlledOpen, onOpenChangeAction}: {
  seat: SeatView;
  isViewer: boolean;
  currencyMode?: string;
  board: string[];
  stage: string;
  handId?: string;
  handComplete: boolean;
  // False while it's the viewer's own turn to act. The panel withholds its
  // explanation during that window so it can never function as a live
  // solver assisting an in-progress decision (see docs/specs/2026-08-21-
  // sandbox-equity-trainer.md).
  isTurn: boolean;
  open?: boolean;
  onOpenChangeAction?: (open: boolean) => void;
}) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
  const open = controlledOpen ?? uncontrolledOpen;
  const setOpen = (nextOpen: boolean) => {
    if (controlledOpen === undefined) setUncontrolledOpen(nextOpen);
    onOpenChangeAction?.(nextOpen);
  };
  const asideRef = useRef<HTMLElement>(null);
  useDismiss(asideRef, open, () => setOpen(false));

  const chance = seat.equity == null ? null : Math.round(seat.equity * 100);
  // Per-street equity recap for the post-showdown "how it moved" section:
  // one entry per street, updated in place while the street is current and
  // appended when it advances. Local-only and reset per hand — nothing here
  // is server state. Follows the same "adjust state during render" pattern
  // Seat.tsx uses for its own per-hand reset (handIdSeen), rather than an
  // effect, since this is deriving state from props, not synchronizing with
  // an external system.
  const [history, setHistory] = useState<{ stage: string; chance: number }[]>(() =>
    chance == null ? [] : [{stage, chance}]);
  const [tracked, setTracked] = useState({handId, stage, chance});
  if (handId !== tracked.handId) {
    setTracked({handId, stage, chance});
    setHistory(chance == null ? [] : [{stage, chance}]);
  } else if (chance != null && (stage !== tracked.stage || chance !== tracked.chance)) {
    setTracked({handId, stage, chance});
    setHistory(previous => previous[previous.length - 1]?.stage === stage ?
      [...previous.slice(0, -1), {stage, chance}] : [...previous, {stage, chance}]);
  }

  if (!isViewer || currencyMode !== 'sandbox') return null;

  const category = seat.hand_category;
  const cards = matchingCards(category, seat.hole_cards ?? [], board);
  const reason = explainEquity(category, board);

  return <aside ref={asideRef} className={`equity-trainer ${open ? 'open' : ''}`} aria-label="Treinador">
    <button type="button" className="equity-trainer-toggle" aria-expanded={open} aria-controls="equity-trainer-panel"
            aria-label={open ? 'Fechar treinador' : 'Ver treinador'}
            disabled={isTurn} title={isTurn ? 'Disponível depois da sua decisão' : undefined}
            onClick={() => setOpen(!open)}>
      <Lightbulb aria-hidden="true"/>
    </button>
    <div id="equity-trainer-panel" className="equity-trainer-panel">
      <h2>Treinador</h2>
      {isTurn ? <p className="equity-trainer-waiting">
        Disponível depois da sua decisão, para nunca funcionar como uma dica em tempo real.
      </p> : <>
        {category && <p className="equity-trainer-category">{HAND_CATEGORY_LABELS[category] || category}</p>}
        {cards.length > 0 && <div className="equity-trainer-cards">
          {cards.map((card, i) => <PlayingCard key={`${i}-${card}`} card={card} index={i} size="hole"/>)}
        </div>}
        {chance != null && <p className="equity-trainer-chance">Chance atual: <b>{chance}%</b></p>}
        <p className="equity-trainer-reason">{reason}</p>
        {handComplete && history.length > 1 && <dl className="equity-trainer-history">
          {history.map(entry => <div key={entry.stage}>
            <dt>{STAGE_LABELS[entry.stage] || entry.stage}</dt>
            <dd>{entry.chance}%</dd>
          </div>)}
        </dl>}
      </>}
    </div>
  </aside>;
}
