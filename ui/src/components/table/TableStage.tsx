'use client';
import {type ReactNode, useCallback, useState, useSyncExternalStore} from 'react';
import {Board} from '@/components/table/Board';
import {Seat} from '@/components/table/Seat';
import {HandOutcomeBanner, type HandOutcomeState} from '@/components/table/HandOutcome';
import {rotateSeats} from '@/lib/utils';
import {PokerLogo} from '@/components/PokerLogo';
import type {TableSnapshot} from '@/lib/api/table';
import {playerPotBreakdown, winnerStandings} from '@/lib/tableOutcome';
import type {PlayerNote} from '@/lib/api/playerNotes';
import {RabbitHunt} from '@/components/table/RabbitHunt';
import {ExitStatus} from '@/components/table/ExitStatus';
import {WinnerCards} from '@/components/table/WinnerCards';
import {DEFAULT_TURN_TIMEOUT_MS} from '@/lib/gameTiming';

// Portrait handhelds get a different experience, not a shrunk table: a tall
// capsule ringed by compact opponents, with the viewer promoted to a hero HUD
// (large hole cards) docked above the action bar. Landscape phones, tablets in
// landscape, and desktop keep the classic oval. Selected per layout tree via
// matchMedia instead of stacking CSS overrides on one DOM, since the geometry of
// the two stages is too different to patch across breakpoints.
const VERTICAL_STAGE_QUERY = '(orientation: portrait) and (max-width: 1023px)';
const STREET_STAGES = ['pre_flop', 'flop', 'turn', 'river'] as const;
type TableCapacity = 2 | 6 | 9;

const OPPONENT_SLOTS: Record<TableCapacity, number[]> = {
  2: [4],
  6: [1, 3, 4, 6, 8],
  9: [1, 2, 3, 4, 5, 6, 7, 8],
};

const OBSERVER_SLOTS: Record<TableCapacity, number[]> = {
  2: [4, 0],
  6: [1, 2, 3, 5, 7, 8],
  9: [1, 2, 3, 4, 5, 6, 7, 8, 0],
};

export function tableCapacity(maxSeats?: number): TableCapacity {
  if (maxSeats != null && maxSeats <= 2) return 2;
  if (maxSeats != null && maxSeats <= 6) return 6;
  return 9;
}

/** Preserve surviving players' physical slots while filling a vacancy after
 * the nearest preceding player in turn order. This is presentation-only: the
 * wire has no seat-number field and game state remains server-authored. */
export function stableSeatOccupants(playerIds: string[], previous: Array<string | null>): Array<string | null> {
  const active = new Set(playerIds);
  const next = previous.map(playerId => playerId && active.has(playerId) ? playerId : null);
  for (const [playerIndex, playerId] of playerIds.entries()) {
    if (next.includes(playerId)) continue;

    let precedingSlot = -1;
    for (let offset = 1; offset <= playerIds.length; offset += 1) {
      const precedingId = playerIds[(playerIndex - offset + playerIds.length) % playerIds.length];
      const found = next.indexOf(precedingId);
      if (found >= 0) {
        precedingSlot = found;
        break;
      }
    }

    for (let offset = 1; offset <= next.length; offset += 1) {
      const candidate = (precedingSlot + offset + next.length) % next.length;
      if (next[candidate] == null) {
        next[candidate] = playerId;
        break;
      }
    }
  }
  return next;
}
// Exported so the page's sr-only heading can name the current stage without
// keeping a second copy of this copy in sync with the felt's own label.
export const STAGE_LABELS: Record<string, string> = {
  waiting_for_players: 'Aguardando jogadores', pre_flop: 'Pré-flop', flop: 'Flop', turn: 'Turn', river: 'River',
  showdown: 'Showdown', complete: 'Mão encerrada'
};

// The house mark woven into the felt, matching the landing hero's table
// preview. Purely decorative (aria-hidden) and tone-on-tone: the SVG filter
// frays the letter edges into the felt weave rather than tinting them, so it
// never competes with the gold that carries value on this surface. The filter
// uses a fixed seed so the fray is stable across renders and reduced-motion
// contexts; `prefers-contrast: more` drops it for a clean, higher-contrast
// mark (see globals.css).
function FeltWordmark() {
  return <div className="felt-wordmark" aria-hidden="true">
    <svg className="felt-fx-defs" focusable="false" aria-hidden="true">
      <filter id="felt-weave" x="-15%" y="-15%" width="130%" height="130%">
        <feTurbulence type="fractalNoise" baseFrequency="0.62 0.92" numOctaves="1" seed="7" result="noise"/>
        <feDisplacementMap in="SourceGraphic" in2="noise" scale="0.7"
          xChannelSelector="R" yChannelSelector="G"/>
      </filter>
    </svg>
    <PokerLogo size={14}/> CTECH
  </div>;
}

function StreetProgress({stage}: { stage: string }) {
  const label = STAGE_LABELS[stage] || stage.replaceAll('_', ' ');
  if (stage === 'waiting_for_players') return <div className="street-progress" aria-hidden="true">
    <span className="street-progress-label">{label}</span>
  </div>;
  const current = STREET_STAGES.indexOf(stage as typeof STREET_STAGES[number]);
  const resolved = stage === 'showdown' || stage === 'complete';
  return <div className="street-progress" aria-hidden="true">
    <span className="street-progress-label">{label}</span>
    <div className="street-progress-pips">
      {STREET_STAGES.map((street, index) => <span key={street}
        className={resolved || index < current ? 'is-complete' : index === current ? 'is-current' : ''}>
        <i/>{street === 'pre_flop' ? 'Pré' : street[0].toUpperCase() + street.slice(1)}
      </span>)}
    </div>
  </div>;
}

function calloutCopy(announcement: string) {
  const parts = announcement.split('. ').map(part => part.trim()).filter(Boolean);
  const event = parts.find(part => /^(Flop|Turn|River):/.test(part)) ||
    parts.find(part => part.includes(' colocou ')) ||
    parts.find(part => part.startsWith('Sua vez')) ||
    parts.find(part => part.startsWith('Vez de '));
  if (event) return event;
  return announcement.startsWith('Mesa atualizada. ') ? announcement.slice('Mesa atualizada. '.length) :
    parts.at(-1) || announcement;
}

function subscribeToStage(onChange: () => void) {
  const query = window.matchMedia(VERTICAL_STAGE_QUERY);
  query.addEventListener('change', onChange);
  return () => query.removeEventListener('change', onChange);
}

function useVerticalStage() {
  // Server snapshot says desktop: the table only renders after the socket
  // delivers a snapshot (post-hydration), so the mismatch frame never paints.
  return useSyncExternalStore(subscribeToStage, () => window.matchMedia(VERTICAL_STAGE_QUERY).matches, () => false);
}

type Props = {
  snapshot: TableSnapshot;
  viewer?: string;
  maxSeats?: number;
  seatLayoutKey?: string;
  pot: number;
  bigBlind: number;
  turnTimeoutMs?: number;
  nowMs: number;
  outcome: HandOutcomeState | null;
  holdOutcomeOpen: boolean;
  // Undefined/0 hides the ring; the page only supplies a deadline once one
  // is armed and no connection notice is already claiming the player's
  // attention (see the gating comment at the call site).
  nextHandDeadlineMs?: number;
  nextHandDurationMs?: number;
  // The viewer's stack right before the currently-showing resolution, so
  // their own seat can count a chip loss down the same way the win pill
  // already counts a payout up. Only meaningful (non-undefined) while that
  // resolution's payouts are still on screen.
  viewerStackBefore?: number;
  canRevealCards?: boolean;
  revealPending?: boolean;
  onRevealCardAction?: (index: number) => void;
  onPeekCardsAction?: () => void;
  rabbitHuntPending?: boolean;
  rabbitHuntFailCount?: number;
  onRequestRabbitHuntAction?: () => void;
  onRabbitHuntVerifyFailedAction?: () => void;
  viewerPendingExit?: boolean;
  onCancelExitAction?: () => void;
  winnerCardsPending?: boolean;
  onRequestWinnerCardsAction?: () => void;
  onAnswerWinnerCardsAction?: (accept: boolean) => void;
  playerNotes?: Record<string, PlayerNote>;
  onEditPlayerNoteAction?: (seat: TableSnapshot['seats'][number]) => void;
  targetedReactionLabel?: string;
  onTargetPlayerAction?: (playerId: string) => void;
  announcement?: string;
  chatBubbles?: Record<string, { id: string; message: string }>;
  // Live table only: builds the per-seat player menu. Replay passes nothing.
  renderPlayerActionsAction?: (seat: TableSnapshot['seats'][number]) => ReactNode;
};

export function TableStage({
                             snapshot,
                             viewer,
                             maxSeats,
                             seatLayoutKey,
                             pot,
                             bigBlind,
                             turnTimeoutMs = DEFAULT_TURN_TIMEOUT_MS,
                             nowMs,
                             outcome,
                             holdOutcomeOpen,
                             nextHandDeadlineMs,
                             nextHandDurationMs,
                             viewerStackBefore,
                             canRevealCards,
                             revealPending,
                             onRevealCardAction,
                             onPeekCardsAction,
                             rabbitHuntPending,
                             rabbitHuntFailCount,
                             onRequestRabbitHuntAction,
                             onRabbitHuntVerifyFailedAction,
                             viewerPendingExit,
                             onCancelExitAction,
                             winnerCardsPending,
                             onRequestWinnerCardsAction,
                             onAnswerWinnerCardsAction,
                             playerNotes,
                             onEditPlayerNoteAction,
                             targetedReactionLabel,
                             onTargetPlayerAction,
                             announcement,
                             chatBubbles,
                             renderPlayerActionsAction
                           }: Props) {
  const vertical = useVerticalStage();
  const capacity = tableCapacity(maxSeats);
  const [outcomeLayer, setOutcomeLayer] = useState({key: outcome?.key, dismissed: false});
  if (outcomeLayer.key !== outcome?.key) setOutcomeLayer({key: outcome?.key, dismissed: false});
  const onOutcomeDismissedChange = useCallback((dismissed: boolean) => {
    setOutcomeLayer(previous => previous.key === outcome?.key && previous.dismissed === dismissed ? previous :
      {key: outcome?.key, dismissed});
  }, [outcome?.key]);
  const [mobileLayout, setMobileLayout] = useState<{
    key: string;
    playerOrder: string;
    occupants: Array<string | null>;
  }>({key: '', playerOrder: '', occupants: []});
  const seats = rotateSeats(snapshot.seats, viewer);
  const standings = winnerStandings(snapshot);
  const seatNode = (seat: TableSnapshot['seats'][number], index: number) => {
    const breakdown = playerPotBreakdown(snapshot, seat.player_id);
    const standing = standings.find(item => item.playerId === seat.player_id);
    return <Seat key={seat.player_id} seat={seat} index={index}
                 isTurn={snapshot.current_player_id === seat.player_id}
                 credit={snapshot.payouts?.[seat.player_id] || 0}
                 winAmount={breakdown.won}
                 winStanding={standing}
                 refundAmount={breakdown.refund}
                 isWinner={snapshot.winners?.includes(seat.player_id) ?? false}
                 baseDeadlineMs={snapshot.action_base_deadline_unix_ms}
                 actionDeadlineMs={snapshot.action_deadline_unix_ms}
                 nowMs={nowMs}
                 turnTimeoutMs={turnTimeoutMs}
                 bigBlind={bigBlind}
                 isViewer={seat.player_id === viewer}
                 canRevealCards={seat.player_id === viewer && canRevealCards}
                 revealPending={revealPending}
                 onRevealCardAction={onRevealCardAction}
                 handComplete={snapshot.stage === 'complete'}
                 handId={snapshot.hand_id}
                 onPeekCards={seat.player_id === viewer ? onPeekCardsAction : undefined}
                 playerNote={playerNotes?.[seat.player_id]}
                 onEditNote={seat.player_id !== viewer && onEditPlayerNoteAction ? () => onEditPlayerNoteAction(seat) : undefined}
                 reactionTargetLabel={seat.player_id !== viewer ? targetedReactionLabel : undefined}
                 onReactionTarget={seat.player_id !== viewer && onTargetPlayerAction ? () => onTargetPlayerAction(seat.player_id) : undefined}
                 stackBefore={seat.player_id === viewer ? viewerStackBefore : undefined}
                 isDealer={snapshot.dealer_player_id === seat.player_id}
                 isSmallBlind={snapshot.small_blind_player_id === seat.player_id}
                 isBigBlind={snapshot.big_blind_player_id === seat.player_id}
                 chatBubble={chatBubbles?.[seat.player_id]}
                 actionsMenu={seat.player_id !== viewer ? renderPlayerActionsAction?.(seat) : undefined}/>;
  };
  const board = <Board cards={snapshot.board} boardTwo={snapshot.board_two}
                       splitAt={snapshot.board_split_at} pot={pot} pots={snapshot.pots}
                       rake={snapshot.rake} bigBlind={bigBlind}/>;
  const feltContent = <>
    <span key={`${snapshot.hand_id || 'waiting'}:${snapshot.stage}`} className="table-street-wash" aria-hidden="true"/>
    <FeltWordmark/>
    {announcement && snapshot.stage !== 'complete' && <div key={announcement} className="table-callout"
      aria-hidden="true"><span>D</span><p>{calloutCopy(announcement)}</p></div>}
    {board}
    <StreetProgress stage={snapshot.stage}/>
  </>;

  if (!vertical) return (
    <div className="game-table" data-stage={snapshot.stage} data-capacity={capacity}>
      <div className="game-rail"/>
      <div className="game-felt">{feltContent}</div>
      {seats.map(seatNode)}
      <HandOutcomeBanner outcome={outcome} holdOpen={holdOutcomeOpen}
                         onDismissedChangeAction={onOutcomeDismissedChange}
                         nextHandDeadlineMs={nextHandDeadlineMs} nextHandDurationMs={nextHandDurationMs}/>
      <RabbitHunt key={snapshot.hand_id} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                  pending={rabbitHuntPending} failCount={rabbitHuntFailCount}
                  onRequestRabbitHuntAction={onRequestRabbitHuntAction}
                  onRabbitHuntVerifyFailedAction={onRabbitHuntVerifyFailedAction}/>
      <ExitStatus pendingExit={Boolean(viewerPendingExit)}
                  isViewerTurn={snapshot.current_player_id === viewer}
                  onCancelAction={() => onCancelExitAction?.()}/>
      <WinnerCards key={`winner-cards:${snapshot.hand_id}`} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                   pending={winnerCardsPending} onRequestWinnerCardsAction={onRequestWinnerCardsAction}
                   onAnswerWinnerCardsAction={onAnswerWinnerCardsAction}
                   offerBlocked={Boolean(outcome && !outcomeLayer.dismissed)}/>
    </div>
  );

  // rotateSeats guarantees the viewer (when seated) is first; that seat leaves
  // the ring entirely and becomes the hero HUD at the stage's bottom edge.
  const viewerFirst = seats[0]?.player_id === viewer;
  const opponents = viewerFirst ? seats.slice(1) : seats;
  const mobileSlots = viewerFirst ? OPPONENT_SLOTS[capacity] : OBSERVER_SLOTS[capacity];
  const layoutKey = `${seatLayoutKey || 'table'}:${viewer || 'observer'}:${capacity}:${viewerFirst}`;
  const playerOrder = opponents.map(seat => seat.player_id).join(':');
  const previousOccupants = mobileLayout.key === layoutKey && mobileLayout.occupants.length === mobileSlots.length ?
    mobileLayout.occupants :
    Array.from<string | null>({length: mobileSlots.length}).fill(null);
  const occupants = stableSeatOccupants(opponents.map(seat => seat.player_id), previousOccupants);
  if (mobileLayout.key !== layoutKey || mobileLayout.playerOrder !== playerOrder) {
    setMobileLayout({key: layoutKey, playerOrder, occupants});
  }
  const mobileSlotByPlayer = new Map(occupants.flatMap((playerId, index) =>
    playerId ? [[playerId, mobileSlots[index]] as const] : []));
  return (
    <div className="game-table stage-v" data-stage={snapshot.stage} data-capacity={capacity}>
      <div className="stage-v-ring">
        <div className="game-rail"/>
        <div className="game-felt">{feltContent}</div>
        {opponents.map((seat, index) => seatNode(seat,
          mobileSlotByPlayer.get(seat.player_id) ?? mobileSlots[index] ?? index + 1))}
        <HandOutcomeBanner outcome={outcome} holdOpen={holdOutcomeOpen}
                           onDismissedChangeAction={onOutcomeDismissedChange}
                           nextHandDeadlineMs={nextHandDeadlineMs} nextHandDurationMs={nextHandDurationMs}/>
        <RabbitHunt key={snapshot.hand_id} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                  pending={rabbitHuntPending} failCount={rabbitHuntFailCount}
                  onRequestRabbitHuntAction={onRequestRabbitHuntAction}
                  onRabbitHuntVerifyFailedAction={onRabbitHuntVerifyFailedAction}/>
        <ExitStatus pendingExit={Boolean(viewerPendingExit)}
                    isViewerTurn={snapshot.current_player_id === viewer}
                    onCancelAction={() => onCancelExitAction?.()}/>
        <WinnerCards key={`winner-cards:${snapshot.hand_id}`} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                     pending={winnerCardsPending} onRequestWinnerCardsAction={onRequestWinnerCardsAction}
                   onAnswerWinnerCardsAction={onAnswerWinnerCardsAction}
                   offerBlocked={Boolean(outcome && !outcomeLayer.dismissed)}/>
      </div>
      {viewerFirst && seatNode(seats[0], 0)}
    </div>
  );
}
