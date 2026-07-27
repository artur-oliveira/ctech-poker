import {ChipStack} from '@/components/table/ChipStack';
import {PlayingCard} from '@/components/table/PlayingCard';
import type {PotView} from '@/lib/api/table';

export function Board({cards, pot, pots, rake, bigBlind}: {
  cards: string[];
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
    <div>{cards.map((card, index) => <PlayingCard key={`${index}-${card}`} card={card}
                                                  index={index < 3 ? index : 0} size="board"
                                                  slow={index === 4}/>)}{Array.from({length: 5 - cards.length}, (_, i) =>
      <span key={i}/>)}</div>
  </div>;
}
