import {Avatar, AvatarFallback} from '@/components/ui/avatar';
import {Progress} from '@/components/ui/progress';
import {ChipStack} from '@/components/table/ChipStack';
import {PlayingCard} from '@/components/table/PlayingCard';
import type {SeatView} from '@/lib/api/table';
import {HAND_CATEGORY_LABELS, initials, playerName} from '@/lib/utils';
import {useCountUp} from '@/lib/hooks/useCountUp';

// chance <= 20% red, <= 60% yellow (reusing the --gold token already used for
// bet amounts on this same seat card), > 60% green.
function equityTone(chance: number) {
  if (chance <= 20) return 'bg-[var(--danger)]';
  if (chance <= 60) return 'bg-[var(--gold)]';
  return 'bg-[var(--success)]';
}

const STATE_LABELS: Record<string, string> = {
  folded: 'Desistiu',
  all_in: 'All-in',
  sitting_out: 'Ausente',
  disconnected: 'Desconectado',
  pending_entry: 'Aguardando'
};

// Seats 3/4/5 sit on the top rail; their winner pill must drop below instead of above.
const TOP_SEAT_INDICES = [3, 4, 5];

export function Seat({seat, isViewer, isTurn, index, payout = 0, isWinner = false, deadlineMs, nowMs, bigBlind, stackBefore}: {
  seat: SeatView;
  isViewer: boolean;
  isTurn: boolean;
  index: number;
  payout?: number;
  isWinner?: boolean;
  deadlineMs?: number;
  nowMs?: number;
  bigBlind?: number;
  // Set only on the viewer's own seat, only while a loss's payout is still
  // on screen — lets the stack count down the same way a payout counts it up
  // below, instead of just snapping to the smaller number.
  stackBefore?: number
}) {
  const cards = seat.hole_cards;
  const chance = seat.equity == null ? null : Math.round(seat.equity * 100);
  const pendingName = !isViewer && !seat.name;
  const remainingMs = isTurn && deadlineMs && nowMs ? Math.max(0, deadlineMs - nowMs) : null;
  const stackFrom = payout > 0 ? seat.stack - payout : stackBefore ?? seat.stack;
  const displayStack = useCountUp(stackFrom, seat.stack);
  return <div data-state={seat.state} aria-current={isTurn ? 'true' : undefined}
              className={`game-seat seat-${index} ${seat.state} ${isViewer ? 'viewer' : ''} ${isTurn ? 'is-turn' : ''} ${isWinner ? 'is-winner' : ''} ${pendingName ? 'is-pending-name' : ''} ${TOP_SEAT_INDICES.includes(index) ? 'top-seat' : ''}`}>
    {remainingMs != null &&
        <span key={deadlineMs} className="seat-turn-ring" style={{animationDuration: `${remainingMs}ms`}}
              aria-hidden="true"/>}
    <div className="seat-cards">{[0, 1].map(i => {
      const card = cards?.[i];
      return <PlayingCard key={`${i}-${card || 'back'}`} card={card} index={i} size="hole"
                          owner={isViewer ? 'viewer' : 'opponent'}/>;
    })}</div>
    <Avatar className="seat-avatar"
            aria-hidden="true"><AvatarFallback>{isViewer ? 'EU' : initials(seat.name)}</AvatarFallback></Avatar>
    <div className="seat-info">
      <b
        title={seat.name || undefined}>{playerName(seat.player_id, isViewer ? seat.player_id : undefined, seat.name)}</b><span>{displayStack.toLocaleString('pt-BR')} fichas</span>{chance != null && isViewer &&
        <div className="seat-equity" aria-label={`Chance estimada de vitória: ${chance}%`}>
          <Progress value={chance} indicatorClassName={equityTone(chance)}/>
          <small>Chance {chance}%</small>
        </div>}{STATE_LABELS[seat.state] &&
        <small className="seat-state">{STATE_LABELS[seat.state]}</small>}{seat.hand_category &&
        <small className="seat-hand-category">{HAND_CATEGORY_LABELS[seat.hand_category] || seat.hand_category}</small>}
    </div>
    {seat.contributed > 0 && <span key={`bet-${seat.contributed}`} className="seat-bet">
        <ChipStack amount={seat.contributed} bigBlind={bigBlind}/>
        <b aria-label={`Aposta de ${seat.contributed.toLocaleString('pt-BR')} fichas`}>{seat.contributed.toLocaleString('pt-BR')}</b>
      </span>}
    {isWinner && payout > 0 &&
        <span key={`win-${payout}`} className="seat-win" role="status">+{payout.toLocaleString('pt-BR')}</span>
    }</div>;
}
