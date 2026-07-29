import {ChipStack} from '@/components/table/ChipStack';
import {PlayingCard} from '@/components/table/PlayingCard';
import type {PotView} from '@/lib/api/table';

function CardRow({cards, slots, offset = 0, label}: {
  cards: string[];
  slots: number;
  offset?: number;
  label?: string
}) {
  return <div className="board-runout-row">
    {label && <span className="board-runout-label" aria-hidden="true">{label}</span>}
    <div aria-label={label ? `${label} distribuição` : undefined}>
      {cards.map((card, index) => <PlayingCard key={`${offset + index}-${card}`} card={card}
                                               index={(offset + index) < 3 ? offset + index : 0}
                                               size="board" slow={offset + index === 4}/>)}
      {Array.from({length: Math.max(0, slots - cards.length)}, (_, index) =>
        <span key={`empty-${index}`}/>)}</div>
  </div>;
}

export function Board({cards, boardTwo, splitAt = 0, pot, pots, rake, bigBlind}: {
  cards: string[];
  boardTwo?: string[];
  splitAt?: number;
  pot: number;
  pots?: PotView[];
  rake?: number;
  bigBlind?: number
}) {
  return <div className="board">{pot > 0 && <span className="game-pot">
    <ChipStack amount={pot} bigBlind={bigBlind} size="pot"/>
    POTE <b key={pot}
            className="pot-value">{pot.toLocaleString('pt-BR')}</b>{rake ?
    <small title="Comissão da casa cobrada sobre o pote (rake)"
           aria-label={`Comissão da casa: ${rake.toLocaleString('pt-BR')} fichas`}>rake {rake.toLocaleString('pt-BR')}</small> : null}
    {pots && pots.length > 1 && <span className="side-pots" aria-label="Divisão dos potes">
      {pots.map((item, index) => <small key={`${index}-${item.amount}`}>
        {index === 0 ? 'Principal' : `Lateral ${index}`}: {item.amount.toLocaleString('pt-BR')}
      </small>)}
    </span>}</span>}
    {boardTwo?.length ? <div className="board-runouts" aria-label="Duas distribuições do board">
      {splitAt > 0 && <div className="board-common">
        <small>Comum</small>
        <CardRow cards={cards.slice(0, splitAt)} slots={splitAt}/>
      </div>}
      <div className="board-runout-pair">
        <CardRow label="1ª" cards={cards.slice(splitAt)} slots={5 - splitAt} offset={splitAt}/>
        <CardRow label="2ª" cards={boardTwo} slots={5 - splitAt} offset={splitAt}/>
      </div>
    </div> : <div>{cards.map((card, index) => <PlayingCard key={`${index}-${card}`} card={card}
                                                  index={index < 3 ? index : 0} size="board"
                                                  slow={index === 4}/>)}
      {Array.from({length: 5 - cards.length}, (_, index) => <span key={index}/>)}</div>}
  </div>;
}
