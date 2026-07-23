import {ChipStack} from '@/components/table/ChipStack';
import {PlayingCard} from '@/components/table/PlayingCard';

export function Board({cards, pot, rake, bigBlind}: { cards: string[]; pot: number; rake?: number; bigBlind?: number }) {
  return <div className="board"><span className="game-pot">
    <ChipStack amount={pot} bigBlind={bigBlind} size="pot"/>
    POTE <b key={pot}
    className="pot-value">{pot.toLocaleString('pt-BR')}</b>{rake ?
    <small title="Comissão da casa cobrada sobre o pote (rake)"
      aria-label={`Comissão da casa: ${rake.toLocaleString('pt-BR')} fichas`}>rake {rake.toLocaleString('pt-BR')}</small> : null}</span>
  <div>{cards.map((card, index) => <PlayingCard key={`${index}-${card}`} card={card}
    index={index < 3 ? index : 0} size="board" slow={index === 4}/>)}{Array.from({length: 5 - cards.length}, (_, i) =>
    <span key={i}/>)}</div>
  </div>;
}
