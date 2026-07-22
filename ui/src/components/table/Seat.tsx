import {PlayingCard} from '@/components/table/PlayingCard';
import type {SeatView} from '@/lib/api/table';
import {playerName} from '@/lib/utils';

const STATE_LABELS: Record<string, string> = {
  folded: 'Desistiu',
  all_in: 'All-in',
  sitting_out: 'Ausente',
  disconnected: 'Desconectado',
  pending_entry: 'Aguardando'
};

// Seats 3/4/5 sit on the top rail; their winner pill must drop below instead of above.
const TOP_SEAT_INDICES = [3, 4, 5];

export function Seat({seat, isViewer, isTurn, index, payout = 0}: {
  seat: SeatView;
  isViewer: boolean;
  isTurn: boolean;
  index: number;
  payout?: number
}) {
  const cards = seat.hole_cards;
  const chance = seat.equity == null ? null : Math.round(seat.equity * 100);
  return <div data-state={seat.state} aria-current={isTurn ? 'true' : undefined}
    className={`game-seat seat-${index} ${seat.state} ${isViewer ? 'viewer' : ''} ${isTurn ? 'is-turn' : ''} ${payout > 0 ? 'is-winner' : ''} ${TOP_SEAT_INDICES.includes(index) ? 'top-seat' : ''}`}>
    <div className="seat-cards">{[0, 1].map(i => {
      const card = cards?.[i];
      return <PlayingCard key={`${i}-${card || 'back'}`} card={card} index={i} size="hole"
        owner={isViewer ? 'viewer' : 'opponent'}/>;
    })}</div>
    <div className="seat-info">
      <b>{playerName(seat.player_id, isViewer ? seat.player_id : undefined)}</b><span>{seat.stack.toLocaleString('pt-BR')} fichas</span>{chance != null && isViewer &&
        <small className="seat-equity"
          aria-label={`Chance estimada de vitória: ${chance}%`}>Chance {chance}%</small>}{STATE_LABELS[seat.state] &&
        <small className="seat-state">{STATE_LABELS[seat.state]}</small>}</div>
    {seat.contributed > 0 &&
        <span key={seat.contributed} className="seat-bet">{seat.contributed.toLocaleString('pt-BR')}</span>}
    {payout > 0 && <span key={payout} className="seat-win" role="status">+{payout.toLocaleString('pt-BR')}</span>
    }</div>;
}
