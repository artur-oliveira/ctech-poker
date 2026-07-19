import Image from 'next/image';
import {cardPath} from '@/lib/cards';

export function Board({cards, pot, rake}: { cards: string[]; pot: number; rake?: number }) {
  return <div className="board"><span className="game-pot">POTE <b>{pot.toLocaleString('pt-BR')}</b>{rake ?
    <small>rake {rake}</small> : null}</span>
    <div>{cards.map(c => <Image key={c} src={cardPath(c)} alt={c} width={68}
                                height={95}/>)}{Array.from({length: 5 - cards.length}, (_, i) => <span key={i}/>)}</div>
  </div>
}
