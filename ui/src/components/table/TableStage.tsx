'use client';
import {type ReactNode, useCallback, useState, useSyncExternalStore} from 'react';
import {Board} from '@/components/table/Board';
import {Seat, type SeatLayoutPosition} from '@/components/table/Seat';
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

// Handhelds get a different experience, not a shrunk desktop table: compact
// opponents ring the rail and the viewer becomes a separate hero HUD. Portrait
// uses a capsule; short landscape uses a shallow oval beside its action dock.
const VERTICAL_STAGE_QUERY = '(orientation: portrait) and (max-width: 1023px)';
const COMPACT_LANDSCAPE_QUERY = '(orientation: landscape) and (max-height: 620px)';
const STREET_STAGES = ['pre_flop', 'flop', 'turn', 'river'] as const;
type TableCapacity = 2 | 6 | 9;

export function tableCapacity(maxSeats?: number): TableCapacity {
  if (maxSeats != null && maxSeats <= 2) return 2;
  if (maxSeats != null && maxSeats <= 6) return 6;
  return 9;
}

/** Evenly divide the occupied perimeter with the viewer at index zero on the
 * bottom. The same angular rule produces face-to-face, reversed-triangle and
 * diamond layouts for 2/3/4 players without capacity-specific magic slots. */
export function balancedSeatPosition(index: number, playerCount: number, portrait = false): SeatLayoutPosition {
  const safeCount = Math.max(1, playerCount);
  if (portrait) {
    // Nine reference points trace the centre of the capsule rail, in the
    // normalised orbit space CSS resolves against the rail tokens (0 = the
    // left/top orbit line, 1 = the right/bottom one — see
    // --table-orbit-* in base.css). Sampling that path by occupancy keeps
    // every player on the material itself; an ellipse cuts the lower pair
    // through the felt, because a capsule's sides stay straight before they
    // turn into its bottom arc.
    const rail = [
      [.5, 1], [.078, .798], [0, .488], [.078, .179], [.3, 0],
      [.7, 0], [.922, .179], [1, .488], [.922, .798], [.5, 1],
    ] as const;
    const progress = index / safeCount * 9;
    const start = Math.floor(progress);
    const mix = progress - start;
    const from = rail[start];
    const to = rail[Math.min(start + 1, rail.length - 1)];
    const s = from[0] + (to[0] - from[0]) * mix;
    const t = from[1] + (to[1] - from[1]) * mix;
    // Only the shallow top arc uses the inward-card/outward-label treatment.
    // The t=.179 reference points are already on the straight upper sides.
    const zone = t > .726 ? 'bottom' : t < .131 ? 'top' : s < .5 ? 'left' : 'right';
    const side = s < .444 ? 'left' : s > .556 ? 'right' : 'center';
    return {s: Number(s.toFixed(4)), t: Number(t.toFixed(4)), zone, side};
  }
  // The desktop oval's seats are the orbit ellipse itself, so they track the
  // rail band the same way the capsule's polyline does.
  const angle = Math.PI / 2 + index * (Math.PI * 2 / safeCount);
  const cosine = Math.cos(angle);
  const sine = Math.sin(angle);
  const zone = sine > .55 ? 'bottom' : sine < -.55 ? 'top' : cosine < 0 ? 'left' : 'right';
  const side = cosine < -.15 ? 'left' : cosine > .15 ? 'right' : 'center';
  return {s: Number((.5 + cosine * .5).toFixed(4)), t: Number((.5 + sine * .5).toFixed(4)), zone, side};
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

// The house mark on the felt, matching the landing hero's table preview.
// Purely decorative (aria-hidden). It crowns the board group (.felt-center)
// rather than the felt's top arc: up there the top-row seats' bet chips travel
// straight through it on their way to the pot, so the lockup was covered on
// every raise. Riding directly above the pot readout keeps it inside the
// centre band the chips stop at. See docs/2026-08-31-felt-wordmark.md.
function FeltWordmark() {
  return <div className="felt-wordmark" aria-hidden="true">
    <PokerLogo size={26}/><b>CTECH</b>
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

function subscribeToCompactLandscape(onChange: () => void) {
  const query = window.matchMedia(COMPACT_LANDSCAPE_QUERY);
  query.addEventListener('change', onChange);
  return () => query.removeEventListener('change', onChange);
}

function useCompactLandscapeStage() {
  return useSyncExternalStore(subscribeToCompactLandscape,
    () => window.matchMedia(COMPACT_LANDSCAPE_QUERY).matches, () => false);
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
  const compactLandscape = useCompactLandscapeStage();
  const capacity = tableCapacity(maxSeats);
  const [outcomeLayer, setOutcomeLayer] = useState({key: outcome?.key, dismissed: false});
  if (outcomeLayer.key !== outcome?.key) setOutcomeLayer({key: outcome?.key, dismissed: false});
  const onOutcomeDismissedChange = useCallback((dismissed: boolean) => {
    setOutcomeLayer(previous => previous.key === outcome?.key && previous.dismissed === dismissed ? previous :
      {key: outcome?.key, dismissed});
  }, [outcome?.key]);
  const seats = rotateSeats(snapshot.seats, viewer);
  const standings = winnerStandings(snapshot);
  const seatNode = (seat: TableSnapshot['seats'][number], index: number, layoutPosition?: SeatLayoutPosition) => {
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
                 layoutPosition={layoutPosition}
                 actionsMenu={seat.player_id !== viewer ? renderPlayerActionsAction?.(seat) : undefined}/>;
  };
  const board = <Board cards={snapshot.board} boardTwo={snapshot.board_two}
                       splitAt={snapshot.board_split_at} pot={pot} pots={snapshot.pots}
                       rake={snapshot.rake} bigBlind={bigBlind}/>;
  const feltContent = <>
    <span key={`${snapshot.hand_id || 'waiting'}:${snapshot.stage}`} className="table-street-wash" aria-hidden="true"/>
    {announcement && snapshot.stage !== 'complete' && <div key={announcement} className="table-callout"
      aria-hidden="true"><span>D</span><p>{calloutCopy(announcement)}</p></div>}
    {/* The street rail hangs off the board rather than off the felt's bottom
        edge: down there it sat in the lane the bottom-row seats' bet chips
        travel through, so a raise from the viewer's own seat covered it. */}
    <div className="felt-center"><FeltWordmark/>{board}<StreetProgress stage={snapshot.stage}/></div>
  </>;

  if (!vertical && !compactLandscape) return (
    <div className="game-table" data-stage={snapshot.stage} data-capacity={capacity}
         data-player-count={seats.length} data-layout-key={seatLayoutKey}>
      <div className="game-rail"/>
      <div className="game-felt">{feltContent}</div>
      {seats.map((seat, index) => seatNode(seat, index, balancedSeatPosition(index, seats.length)))}
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
  return (
    <div className={`game-table stage-v ${compactLandscape ? 'stage-h' : ''}`} data-stage={snapshot.stage} data-capacity={capacity}
         data-player-count={seats.length} data-layout-key={seatLayoutKey}>
      <div className="stage-v-ring">
        <div className="game-rail"/>
        <div className="game-felt">{feltContent}</div>
        {opponents.map((seat, index) => {
          const tableIndex = viewerFirst ? index + 1 : index;
          return seatNode(seat, tableIndex, balancedSeatPosition(tableIndex, seats.length, vertical));
        })}
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
