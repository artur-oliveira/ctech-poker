'use client';
import {
  memo, type ReactNode, useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore
} from 'react';
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
import {useEnteredKeys} from '@/lib/hooks/useEnteredKeys';

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
 * diamond layouts for 2/3/4 players without capacity-specific magic slots.
 *
 * Every seat lands on the orbit ellipse — the rail band's centreline
 * (`--table-orbit-*`) — so opponents sit on the walnut with their bet chips on
 * the felt inside it. Portrait and the desktop oval share this rule; portrait
 * differs only in the capsule shape and per-occupancy rail insets, both in CSS.
 * In portrait, index 0 (the viewer) leaves the ring to become the bottom HUD,
 * so its bottom slot is shared out evenly rather than weighting one side. */
export function balancedSeatPosition(index: number, playerCount: number): SeatLayoutPosition {
  const safeCount = Math.max(1, playerCount);
  const angle = Math.PI / 2 + index * (Math.PI * 2 / safeCount);
  const cosine = Math.cos(angle);
  const sine = Math.sin(angle);
  const zone = sine > .55 ? 'bottom' : sine < -.55 ? 'top' : cosine < 0 ? 'left' : 'right';
  const side = cosine < -.15 ? 'left' : cosine > .15 ? 'right' : 'center';
  return {s: Number((.5 + cosine * .5).toFixed(4)), t: Number((.5 + sine * .5).toFixed(4)), zone, side};
}

// `balancedSeatPosition` is pure and its inputs are a tiny bounded set
// (index x occupancy), but it returns a fresh object — which is a new prop
// identity for every seat on every render, and enough on its own to defeat
// `memo(Seat)`. Caching the results is what makes the memo real (#230).
const layoutPositionCache = new Map<string, SeatLayoutPosition>();

function seatLayoutPosition(index: number, playerCount: number): SeatLayoutPosition {
  const key = `${index}:${playerCount}`;
  const cached = layoutPositionCache.get(key);
  if (cached) return cached;
  const position = balancedSeatPosition(index, playerCount);
  layoutPositionCache.set(key, position);
  return position;
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

/** Four bare pips and one word: the name of the street the hand is actually
 * on. Naming all four at once (the shape this shipped as) spent the felt's
 * whole centre band on three labels nobody reads — where the hand *is* is the
 * only one that matters, and the pips already carry how far along that is.
 *
 * It is a labelled image rather than aria-hidden decoration: "etapa 2 de 4" is
 * progress the sr-only page heading does not otherwise convey. */
function StreetProgress({stage}: { stage: string }) {
  const label = STAGE_LABELS[stage] || stage.replaceAll('_', ' ');
  if (stage === 'waiting_for_players') return <div className="street-progress" role="img" aria-label={label}>
    <span className="street-progress-label" aria-hidden="true">{label}</span>
  </div>;
  const current = STREET_STAGES.indexOf(stage as typeof STREET_STAGES[number]);
  const resolved = stage === 'showdown' || stage === 'complete';
  const position = resolved ? STREET_STAGES.length : current + 1;
  return <div className="street-progress" role="img"
              aria-label={`${label} — etapa ${position} de ${STREET_STAGES.length}`}>
    <div className="street-progress-pips" aria-hidden="true">
      {STREET_STAGES.map((street, index) => <span key={street}
        className={resolved || index < current ? 'is-complete' : index === current ? 'is-current' : ''}>
        <i/>
      </span>)}
    </div>
    <span className="street-progress-label" aria-hidden="true">{label}</span>
  </div>;
}

/** How long a departing seat stays mounted to play its exit. Keep in sync with
 * the `seat-leave` keyframes in (app)/table/table.css. */
const SEAT_EXIT_MS = 260;

/** Seats that left since the last snapshot, held for one exit animation with
 * the geometry they last had (their old index in their old occupancy), so the
 * ghost slides out from where the player was sitting while the survivors take
 * their new places immediately — the ring's own reflow is `left`/`top`, which
 * must never be animated. Rejoining players are not tracked: the ghost is
 * inert (`pointer-events: none`) and gone within `SEAT_EXIT_MS`. */
type DepartedSeat = { seat: TableSnapshot['seats'][number]; index: number; count: number };

function useDepartedSeats(seats: TableSnapshot['seats']): DepartedSeat[] {
  const [departed, setDeparted] = useState<DepartedSeat[]>([]);
  const previous = useRef<DepartedSeat[]>([]);
  const seatKey = seats.map(seat => seat.player_id).join(',');
  useEffect(() => {
    const live = new Set(seats.map(seat => seat.player_id));
    const gone = previous.current.filter(entry => !live.has(entry.seat.player_id));
    previous.current = seats.map((seat, index) => ({seat, index, count: seats.length}));
    if (!gone.length) return undefined;
    setDeparted(current => [...current, ...gone]);
    const timer = setTimeout(() =>
      setDeparted(current => current.filter(entry => !gone.includes(entry))), SEAT_EXIT_MS);
    return () => clearTimeout(timer);
    // Membership, not identity: a stack or a bet moving must not re-run this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seatKey]);
  return departed;
}

/** How long a newly-seated player keeps `data-seat-joining`. Keep in sync with
 * the `seat-join` keyframes in (app)/table/table.css. */
const SEAT_ENTER_MS = 260;

/** The player ids that appeared since the previous membership — the only seats
 * that play `seat-join`. See `useEnteredKeys`: the animation must never be what
 * makes a seat visible (WebKit strands a `pending` animation on a busy frame,
 * which once rendered the whole felt seatless). Seats that are simply there
 * carry no animation; their resting state is their stylesheet's. */
function useJoinedSeats(seats: TableSnapshot['seats']): ReadonlySet<string> {
  return useEnteredKeys(seats.map(seat => seat.player_id), SEAT_ENTER_MS);
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

function TableStageImpl({
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
  // Recomputed only when the snapshot itself changes: `winnerStandings` builds
  // fresh objects, and a chat bubble or a reaction arriving would otherwise
  // hand every seat a new `winStanding` and defeat `memo(Seat)` (#230).
  const standings = useMemo(() => winnerStandings(snapshot), [snapshot]);
  const departed = useDepartedSeats(snapshot.seats);
  const joined = useJoinedSeats(snapshot.seats);
  const departedNodes = (positioned: boolean) => departed.map(entry =>
    seatNode(entry.seat, entry.index, positioned ? seatLayoutPosition(entry.index, entry.count) : undefined, true));
  const seatNode = (seat: TableSnapshot['seats'][number], index: number, layoutPosition?: SeatLayoutPosition,
                    leaving = false) => {
    const breakdown = playerPotBreakdown(snapshot, seat.player_id);
    const standing = standings.find(item => item.playerId === seat.player_id);
    // Only the seat on the clock consumes the timing props, and only while it
    // is on the clock (Seat's showNormalClock/showTimeBank both require
    // isTurn). Handing them to the other eight would re-render all of them on
    // every frame for a clock none of them draws.
    const isTurn = snapshot.current_player_id === seat.player_id;
    return <Seat key={leaving ? `left:${seat.player_id}` : seat.player_id} seat={seat} index={index}
                 leaving={leaving}
                 joining={!leaving && joined.has(seat.player_id)}
                 isTurn={isTurn}
                 credit={snapshot.payouts?.[seat.player_id] || 0}
                 winAmount={breakdown.won}
                 winStanding={standing}
                 refundAmount={breakdown.refund}
                 isWinner={snapshot.winners?.includes(seat.player_id) ?? false}
                 baseDeadlineMs={isTurn ? snapshot.action_base_deadline_unix_ms : undefined}
                 actionDeadlineMs={isTurn ? snapshot.action_deadline_unix_ms : undefined}
                 nowMs={isTurn ? nowMs : undefined}
                 turnTimeoutMs={isTurn ? turnTimeoutMs : undefined}
                 bigBlind={bigBlind}
                 isViewer={seat.player_id === viewer}
                 canRevealCards={seat.player_id === viewer && canRevealCards}
                 revealPending={revealPending}
                 onRevealCardAction={onRevealCardAction}
                 handComplete={snapshot.stage === 'complete'}
                 handId={snapshot.hand_id}
                 onPeekCards={seat.player_id === viewer ? onPeekCardsAction : undefined}
                 playerNote={playerNotes?.[seat.player_id]}
                 onEditNote={seat.player_id !== viewer ? onEditPlayerNoteAction : undefined}
                 reactionTargetLabel={seat.player_id !== viewer ? targetedReactionLabel : undefined}
                 onReactionTarget={seat.player_id !== viewer ? onTargetPlayerAction : undefined}
                 stackBefore={seat.player_id === viewer ? viewerStackBefore : undefined}
                 isDealer={snapshot.dealer_player_id === seat.player_id}
                 isSmallBlind={snapshot.small_blind_player_id === seat.player_id}
                 isBigBlind={snapshot.big_blind_player_id === seat.player_id}
                 chatBubble={chatBubbles?.[seat.player_id]}
                 layoutPosition={layoutPosition}
                 renderActionsMenu={seat.player_id !== viewer ? renderPlayerActionsAction : undefined}/>;
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
      {seats.map((seat, index) => seatNode(seat, index, seatLayoutPosition(index, seats.length)))}
      {departedNodes(true)}
      <HandOutcomeBanner outcome={outcome} holdOpen={holdOutcomeOpen}
                         onDismissedChangeAction={onOutcomeDismissedChange}
                         nextHandDeadlineMs={nextHandDeadlineMs} nextHandDurationMs={nextHandDurationMs}/>
      <div className="table-overlay-stack">
        <WinnerCards key={`winner-cards:${snapshot.hand_id}`} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                     pending={winnerCardsPending} onRequestWinnerCardsAction={onRequestWinnerCardsAction}
                     onAnswerWinnerCardsAction={onAnswerWinnerCardsAction}
                     offerBlocked={Boolean(outcome && !outcomeLayer.dismissed)}/>
        <RabbitHunt key={snapshot.hand_id} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                    pending={rabbitHuntPending} failCount={rabbitHuntFailCount}
                    onRequestRabbitHuntAction={onRequestRabbitHuntAction}
                    onRabbitHuntVerifyFailedAction={onRabbitHuntVerifyFailedAction}/>
        <ExitStatus pendingExit={Boolean(viewerPendingExit)}
                    isViewerTurn={snapshot.current_player_id === viewer}
                    onCancelAction={() => onCancelExitAction?.()}/>
      </div>
    </div>
  );

  // rotateSeats guarantees the viewer (when seated) is first; that seat leaves
  // the ring entirely and becomes the hero HUD at the stage's bottom edge.
  const viewerFirst = seats[0]?.player_id === viewer;
  const opponents = viewerFirst ? seats.slice(1) : seats;

  const overlayStack = <>
    <HandOutcomeBanner outcome={outcome} holdOpen={holdOutcomeOpen}
                       onDismissedChangeAction={onOutcomeDismissedChange}
                       nextHandDeadlineMs={nextHandDeadlineMs} nextHandDurationMs={nextHandDurationMs}/>
    <div className="table-overlay-stack">
      <WinnerCards key={`winner-cards:${snapshot.hand_id}`} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                   pending={winnerCardsPending} onRequestWinnerCardsAction={onRequestWinnerCardsAction}
                   onAnswerWinnerCardsAction={onAnswerWinnerCardsAction}
                   offerBlocked={Boolean(outcome && !outcomeLayer.dismissed)}/>
      <RabbitHunt key={snapshot.hand_id} snapshot={snapshot} viewer={viewer} bigBlind={bigBlind}
                  pending={rabbitHuntPending} failCount={rabbitHuntFailCount}
                  onRequestRabbitHuntAction={onRequestRabbitHuntAction}
                  onRabbitHuntVerifyFailedAction={onRabbitHuntVerifyFailedAction}/>
      <ExitStatus pendingExit={Boolean(viewerPendingExit)}
                  isViewerTurn={snapshot.current_player_id === viewer}
                  onCancelAction={() => onCancelExitAction?.()}/>
    </div>
  </>;

  // Portrait handhelds and short landscape both use the avatar ring: the
  // viewer leaves it and becomes a separate hero seat. Portrait (stage-v) docks
  // that seat below the capsule; short landscape (stage-h) uses `display:
  // contents` on this wrapper so the ring and the hero seat become siblings in
  // the .game grid — ring in the left column, hero above the action dock on
  // the right. Same composition, one extra class.
  return (
    <div className={`game-table stage-v${compactLandscape ? ' stage-h' : ''}`} data-stage={snapshot.stage}
         data-capacity={capacity} data-player-count={seats.length} data-layout-key={seatLayoutKey}>
      <div className="stage-v-ring">
        <div className="game-rail"/>
        <div className="game-felt">{feltContent}</div>
        {opponents.map((seat, index) => {
          const tableIndex = viewerFirst ? index + 1 : index;
          return seatNode(seat, tableIndex, seatLayoutPosition(tableIndex, seats.length));
        })}
        {departedNodes(true)}
        {overlayStack}
      </div>
      {viewerFirst && seatNode(seats[0], 0)}
    </div>
  );
}

/** Memoised: The felt is the most expensive subtree on the page and it depends on the snapshot, not on chat, reactions, dialogs or panel state. Memoised, those stop reaching it at all.
 *  Every prop it receives is either a primitive or a stable identity
 *  (see `useTableRealtimeSession`'s `commands` memo, `useTableOverlays`'
 *  cached panel handlers, and the table page's memoised projections), so the
 *  comparison actually pays off. Issue #230. */
export const TableStage = memo(TableStageImpl);
TableStage.displayName = 'TableStage';
