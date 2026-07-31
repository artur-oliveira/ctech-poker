import {type CSSProperties, useState} from 'react';
import {PlayerAvatar} from '@/components/ui/player-avatar';
import {Progress} from '@/components/ui/progress';
import {ChipStack} from '@/components/table/ChipStack';
import {PerimeterTimer} from '@/components/table/PerimeterTimer';
import {PlayingCard} from '@/components/table/PlayingCard';
import type {SeatView} from '@/lib/api/table';
import {HAND_CATEGORY_LABELS, playerName} from '@/lib/utils';
import {useCountUp} from '@/lib/hooks/useCountUp';
import {NotebookPen} from 'lucide-react';
import type {PlayerNote} from '@/lib/api/playerNotes';
import {playstyleMeta} from '@/lib/playstyle';
import type {WinnerStanding} from '@/lib/tableOutcome';

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

// Poker jargon stays in its short form on the badge itself (matches D/SB/BB
// conventions used table-side worldwide); the full word only surfaces via
// title/aria-label for hover and assistive tech.
const ROLE_LABELS: Record<string, string> = {
  D: 'Dealer',
  SB: 'Small blind',
  BB: 'Big blind',
  'D/SB': 'Dealer e small blind'
};

// Seats 3/4/5 sit on the top rail; their winner pill must drop below instead of above.
const TOP_SEAT_INDICES = [3, 4, 5];

// A small burst around the winner's own seat, independent of the viewer's
// personal win/lose banner, so every player at the table can tell who just
// won without reading anyone's chip count. Angles only; color alternates via
// CSS nth-child, same trick as the center-table confetti in HandOutcome.
const SEAT_CONFETTI_ANGLES = [0, 45, 90, 135, 180, 225, 270, 315];

function SeatTurnTimer({baseDeadlineMs, observedAtMs, durationMs}: {
  baseDeadlineMs: number;
  observedAtMs: number;
  durationMs: number;
}) {
  // Capture the absolute turn position only when a genuinely new base
  // deadline mounts this keyed component. Later snapshots (presence, equity,
  // reconnects) must not rewrite a running CSS animation's duration/offset.
  const [initialElapsedMs] = useState(() => Math.min(durationMs, Math.max(0,
    observedAtMs - (baseDeadlineMs - durationMs))));
  return <PerimeterTimer className="seat-turn-ring" durationMs={durationMs}
                         elapsedMs={initialElapsedMs} restartKey={baseDeadlineMs} radius={14}/>;
}

export function Seat({
                       seat,
                       isViewer,
                       isTurn,
                       index,
                       credit = 0,
                       winAmount = 0,
                       winStanding,
                       refundAmount = 0,
                       isWinner = false,
                       baseDeadlineMs,
                       nowMs,
                       turnTimeoutMs,
                       bigBlind,
                       stackBefore,
                       isDealer = false,
                       isSmallBlind = false,
                       isBigBlind = false,
                       canRevealCards = false,
                       revealPending = false,
                       onRevealCardAction,
                       playerNote,
                       onEditNote,
                       reactionTargetLabel,
                       onReactionTarget
                     }: {
  seat: SeatView;
  isViewer: boolean;
  isTurn: boolean;
  index: number;
  credit?: number;
  winAmount?: number;
  winStanding?: WinnerStanding;
  refundAmount?: number;
  isWinner?: boolean;
  baseDeadlineMs?: number;
  nowMs?: number;
  turnTimeoutMs?: number;
  bigBlind?: number;
  // Set only on the viewer's own seat, only while a loss's payout is still
  // on screen. Lets the stack count down the same way a payout counts it up
  // below, instead of just snapping to the smaller number.
  stackBefore?: number;
  isDealer?: boolean;
  isSmallBlind?: boolean;
  isBigBlind?: boolean;
  canRevealCards?: boolean;
  revealPending?: boolean;
  onRevealCardAction?: (index: number) => void
  playerNote?: PlayerNote;
  onEditNote?: () => void;
  reactionTargetLabel?: string;
  onReactionTarget?: () => void;
}) {
  const cards = seat.hole_cards;
  const chance = seat.equity == null ? null : Math.round(seat.equity * 100);
  const pendingName = !isViewer && !seat.name;
  const isDisconnected = seat.connection_state === 'disconnected';
  // The seat perimeter is the room's normal decision clock only. Once its
  // base deadline expires, the separately labelled time-bank readout owns the
  // remaining reserve; it must never lengthen or restart this rectangle.
  const showNormalClock = Boolean(isTurn && baseDeadlineMs && nowMs && turnTimeoutMs && baseDeadlineMs > nowMs);
  const stackFrom = credit > 0 ? seat.stack - credit : stackBefore ?? seat.stack;
  const displayStack = useCountUp(stackFrom, seat.stack);
  // Heads-up has the dealer double as the small blind, so one combined badge
  // replaces two overlapping pills.
  const role = isDealer && isSmallBlind ? 'D/SB' : isDealer ? 'D' : isSmallBlind ? 'SB' : isBigBlind ? 'BB' : null;
  const pausedAfterHand = seat.ready === false && seat.state !== 'sitting_out';
  const playstyle = seat.playstyle_badge ? playstyleMeta(seat.playstyle_badge) : undefined;
  return <div data-state={seat.state} data-connection-state={seat.connection_state}
              data-player-id={seat.player_id}
              aria-current={isTurn ? 'true' : undefined}
              className={`game-seat seat-${index} ${seat.state} ${isDisconnected ? 'disconnected' : ''} ${isViewer ? 'viewer' : ''} ${isTurn ? 'is-turn' : ''} ${isWinner ? 'is-winner' : ''} ${reactionTargetLabel ? 'is-reaction-target' : ''} ${pendingName ? 'is-pending-name' : ''} ${TOP_SEAT_INDICES.includes(index) ? 'top-seat' : ''}`}>
    {reactionTargetLabel && onReactionTarget && <button type="button" className="seat-reaction-target"
                                                        aria-label={`${reactionTargetLabel} em ${playerName(seat.player_id, undefined, seat.name)}`}
                                                        onClick={onReactionTarget}><span>Escolher</span></button>}
    {showNormalClock && baseDeadlineMs && nowMs && turnTimeoutMs &&
        <SeatTurnTimer key={baseDeadlineMs} baseDeadlineMs={baseDeadlineMs}
                       observedAtMs={nowMs} durationMs={turnTimeoutMs}/>}
    {role && <span className={`seat-role ${isDealer ? 'is-dealer' : ''}`} title={ROLE_LABELS[role]}
                   aria-label={ROLE_LABELS[role]}>{role}</span>}
    <div className={`seat-cards ${isWinner && winAmount > 0 ? 'is-collecting' : ''}`}>{[0, 1].map(i => {
      const card = cards?.[i];
      const publiclyRevealed = seat.hole_cards_revealed?.[i] ?? false;
      return <PlayingCard key={`${i}-${card || 'back'}`} card={card} index={i} size="hole"
                          owner={isViewer ? 'viewer' : 'opponent'}
                          onReveal={canRevealCards && !publiclyRevealed ? () => onRevealCardAction?.(i) : undefined}
                          revealPending={revealPending}/>;
    })}</div>
    {isWinner && winAmount > 0 && <span key={`confetti-${winAmount}`} className="seat-confetti" aria-hidden="true">
      {SEAT_CONFETTI_ANGLES.map((rot, i) => <span key={i} style={{
        '--rot': `${rot}deg`,
        animationDelay: `${(i % 4) * 20}ms`
      } as CSSProperties}/>)}
    </span>}
    <PlayerAvatar className="seat-avatar" name={seat.name} avatarUrl={seat.avatar_url}
                  isViewer={isViewer} decorative/>
    {!isViewer && onEditNote && <button type="button"
                                        className={`seat-note-trigger ${playerNote ? 'has-note' : ''}`}
                                        aria-label={playerNote ? `Editar nota privada sobre ${seat.name || 'jogador'}` : `Adicionar nota privada sobre ${seat.name || 'jogador'}`}
                                        title={playerNote ? 'Editar nota privada' : 'Adicionar nota privada'}
                                        onClick={onEditNote}>
      {playerNote?.tag && <span className={`player-note-dot tag-${playerNote.tag}`} aria-hidden="true"/>}
        <NotebookPen aria-hidden="true"/>
    </button>}
    <div className="seat-info">
      {playstyle && <span className="seat-playstyle" title={playstyle.reason}>{playstyle.label}</span>}
      <b
        title={seat.name || undefined}>{playerName(seat.player_id, isViewer ? seat.player_id : undefined, seat.name)}</b><span>{displayStack.toLocaleString('pt-BR')} fichas</span>{chance != null && isViewer &&
        <div className="seat-equity" aria-label={`Chance estimada de vitória: ${chance}%`}>
            <Progress value={chance} indicatorClassName={equityTone(chance)}/>
            <small>Chance {chance}%</small>
        </div>}{STATE_LABELS[seat.state] &&
        <small className="seat-state">{STATE_LABELS[seat.state]}</small>}{pausedAfterHand &&
        <small className="seat-state seat-next-state">Pausa na próxima
            mão</small>}{isDisconnected && seat.state !== 'disconnected' &&
        <small className="seat-state">Desconectado</small>}{seat.hand_category &&
        <small className="seat-hand-category">{HAND_CATEGORY_LABELS[seat.hand_category] || seat.hand_category}</small>}
    </div>
    {seat.contributed > 0 && <span key={`bet-${seat.contributed}`} className="seat-bet">
        <ChipStack amount={seat.contributed} bigBlind={bigBlind}/>
        <b aria-label={`Aposta de ${seat.contributed.toLocaleString('pt-BR')} fichas`}>{seat.contributed.toLocaleString('pt-BR')}</b>
      </span>}
    {isWinner && winAmount > 0 &&
        <span key={`win-${winAmount}`} className="seat-win" role="status">
          <small>{winStanding?.tied ? 'Empate' : winStanding?.place ? `${winStanding.place}º lugar` : 'Venceu'}</small>
          +{winAmount.toLocaleString('pt-BR')}
        </span>
    }
    {refundAmount > 0 &&
        <span key={`refund-${refundAmount}`} className="seat-refund">
          ↩ {refundAmount.toLocaleString('pt-BR')}
        </span>
    }</div>;
}
