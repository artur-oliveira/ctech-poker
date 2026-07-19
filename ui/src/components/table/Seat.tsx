import Image from 'next/image';
import {back, cardPath} from '@/lib/cards';
import type {SeatView} from '@/lib/api/table'

export function Seat({seat, isViewer, index}: { seat: SeatView; isViewer: boolean; index: number }) {
  const cards = seat.hole_cards;
  return <div className={`game-seat seat-${index} ${seat.state} ${isViewer ? 'viewer' : ''}`}>
    <div className="seat-cards">{[0, 1].map(i => <Image key={i} src={cards?.[i] ? cardPath(cards[i]) : back} alt="Carta"
                                                        width={46} height={64}/>)}</div>
    <div className="seat-info">
      <b>{isViewer ? 'Você' : seat.player_id.slice(0, 8)}</b><span>{seat.stack.toLocaleString('pt-BR')} fichas</span>{seat.equity != null &&
        <small>Equity {Math.round(seat.equity * 100)}%</small>}</div>
    {seat.contributed > 0 && <span className="seat-bet">{seat.contributed}</span>}</div>
}
