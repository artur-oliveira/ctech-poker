import {ChipStack} from '@/components/table/ChipStack';
import {CARD_REVEAL_MS, PlayingCard} from '@/components/table/PlayingCard';
import type {PotView} from '@/lib/api/table';
import {Hourglass, Rabbit, RotateCcw, ShieldCheck, Repeat2} from 'lucide-react';
import {useChipExact, useChipFormat, useChipUnit} from '@/lib/chipFormat';
import {useEnteredKeys} from '@/lib/hooks/useEnteredKeys';
import type {RabbitHuntBoard, RabbitSlot as RabbitSlotState} from '@/lib/hooks/useRabbitHunt';

const SLOT_SUITS = ['♠', '♥', '♣', '♦', '♠'];
const SLOT_NAMES = ['a 1ª carta do flop', 'a 2ª carta do flop', 'a 3ª carta do flop', 'o turn', 'o river'];

function EmptyCardSlots({count, offset = 0, rabbit, revealing}: {
  count: number;
  offset?: number;
  rabbit?: RabbitHuntBoard;
  revealing: ReadonlySet<string>
}) {
  return Array.from({length: count}, (_, index) => {
    const slot = offset + index;
    const rabbitSlot = rabbit?.slots[slot];
    if (rabbit && rabbitSlot) {
      return <RabbitBoardSlot key={`rabbit-${slot}`} slot={slot} state={rabbitSlot} fee={rabbit.fee}
                              onBuy={rabbit.onBuy} revealing={revealing}/>;
    }
    // The flop lands three cards at once, so an empty board readies all three
    // of its slots; the turn and river ready only the one that comes next.
    const isNext = offset === 0 ? slot < 3 : index === 0;
    return <span key={`empty-${slot}`}
                 className={`board-slot${isNext ? ' is-next' : ''}`}
                 data-suit={SLOT_SUITS[slot % SLOT_SUITS.length]}
                 aria-hidden="true"/>;
  });
}

/** One undealt slot of a hand won without showdown, sold on its own for the
 * small blind. Every state of the purchase lives on the slot itself. */
function RabbitBoardSlot({slot, state, fee, onBuy, revealing}: {
  slot: number;
  state: RabbitSlotState;
  fee: number;
  onBuy: (slot: number) => void;
  revealing: ReadonlySet<string>
}) {
  const chips = useChipFormat();
  const exact = useChipExact();
  const unit = useChipUnit();
  const name = SLOT_NAMES[slot];
  if (state.status === 'revealed' && state.card) {
    return <span className="board-slot rabbit-slot" data-state="revealed">
      <PlayingCard card={state.card} index={0} size="board" revealing={revealing.has(`${slot}:${state.card}`)}/>
      <span className="rabbit-slot-badge" title="Rabbit hunt: carta que teria saído, não altera o resultado">
        <Rabbit aria-hidden="true"/>
      </span>
    </span>;
  }
  if (state.status === 'offer') {
    return <span className="board-slot rabbit-slot" data-state="offer">
      <button type="button" onClick={() => onBuy(slot)}
              aria-label={`Ver ${name} (rabbit hunt) por ${exact(fee)}${unit}. Não altera o resultado`}>
        <Rabbit aria-hidden="true"/>
        <b aria-hidden="true">{chips(fee)}</b>
      </button>
    </span>;
  }
  const copy = state.status === 'failed'
    ? {icon: <RotateCcw aria-hidden="true"/>, text: 'Devolvido', label: `Não foi possível verificar ${name}. Taxa devolvida.`}
    : state.status === 'verifying'
      ? {icon: <ShieldCheck aria-hidden="true"/>, text: 'Verificando', label: `Verificando ${name} no baralho…`}
      : {icon: <Hourglass aria-hidden="true"/>, text: 'Comprando', label: `Comprando ${name}…`};
  return <span className="board-slot rabbit-slot" data-state={state.status} role="status" aria-label={copy.label}>
    {copy.icon}
    <small aria-hidden="true">{copy.text}</small>
  </span>;
}

/** The community cards for one row. Keyed by slot, not by value, so a card node
 * persists — and `useEnteredKeys` marks only the slots dealt since the previous
 * render, so those play the deal-in flip and every other card (a re-enter with
 * a full board, a replay scrub, reduced motion) just paints its resting face.
 * Bought rabbit-hunt cards go through the same mark, so each one flips once.
 * See docs/2026-09-10-card-reveal-visibility.md. */
function BoardCards({cards, slots, offset = 0, rabbit}: {
  cards: string[];
  slots: number;
  offset?: number;
  rabbit?: RabbitHuntBoard
}) {
  const rabbitKeys = rabbit ? Object.entries(rabbit.slots)
    .map(([slot, state]) => state.status === 'revealed' ? `${slot}:${state.card}` : '') : [];
  const dealt = useEnteredKeys([...cards.map((card, index) => `${offset + index}:${card}`), ...rabbitKeys],
    CARD_REVEAL_MS);
  return <>
    {cards.map((card, index) => {
      const slot = offset + index;
      return <PlayingCard key={slot} card={card} index={slot < 3 ? slot : 0} size="board"
                          slow={slot === 4} revealing={dealt.has(`${slot}:${card}`)}/>;
    })}
    <EmptyCardSlots count={Math.max(0, slots - cards.length)} offset={offset + cards.length}
                    rabbit={rabbit} revealing={dealt}/>
  </>;
}

function CardRow({cards, slots, offset = 0, label}: {
  cards: string[];
  slots: number;
  offset?: number;
  label?: string
}) {
  return <div className="board-runout-row">
    {label && <span className="board-runout-label" aria-hidden="true">{label}</span>}
    <div aria-label={label ? `${label} distribuição` : undefined}>
      <BoardCards cards={cards} slots={slots} offset={offset}/></div>
  </div>;
}

export function Board({cards, boardTwo, splitAt = 0, pot, pots, rake, bigBlind, rabbit}: {
  cards: string[];
  boardTwo?: string[];
  splitAt?: number;
  pot: number;
  pots?: PotView[];
  rake?: number;
  bigBlind?: number;
  // Present only while a rabbit hunt is on offer or bought this hand; the
  // undealt slots become the purchase controls (useRabbitHunt).
  rabbit?: RabbitHuntBoard
}) {
  const chips = useChipFormat();
  const exact = useChipExact();
  const unit = useChipUnit();
  return <div className="board">{pot > 0 && <span className="game-pot">
    <ChipStack amount={pot} bigBlind={bigBlind} size="pot"/>
    POTE <b key={pot} className="pot-value"
            aria-label={`Pote de ${exact(pot)}${unit}`}>{chips(pot)}</b>{rake ?
    <small title="Comissão da casa cobrada sobre o pote (rake)"
           aria-label={`Comissão da casa: ${exact(rake)}${unit}`}>rake {chips(rake)}</small> : null}
    {pots && pots.length > 1 && <span className="side-pots" aria-label="Divisão dos potes">
      {pots.map((item, index) => <small key={`${index}-${item.amount}`}
                                        aria-label={`${index === 0 ? 'Principal' : `Lateral ${index}`}: ${exact(item.amount)}${unit}`}>
        {index === 0 ? 'Principal' : `Lateral ${index}`}: {chips(item.amount)}
      </small>)}
    </span>}</span>}
    {boardTwo?.length ? <div className="board-runouts" aria-label="Duas distribuições do board">
      <div className="board-runouts-heading">
        <Repeat2 aria-hidden="true"/>
        <span><b>Rodando duas vezes</b><small>Dois boards no mesmo all-in</small></span>
      </div>
      {splitAt > 0 && <div className="board-common">
          <small>Comum</small>
          <CardRow cards={cards.slice(0, splitAt)} slots={splitAt}/>
      </div>}
      <div className="board-runout-pair">
        <CardRow label="1ª" cards={cards.slice(splitAt)} slots={5 - splitAt} offset={splitAt}/>
        <CardRow label="2ª" cards={boardTwo} slots={5 - splitAt} offset={splitAt}/>
      </div>
    </div> : <div><BoardCards cards={cards} slots={5} rabbit={rabbit}/></div>}
  </div>;
}
