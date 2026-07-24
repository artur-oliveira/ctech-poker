'use client';
import {useSyncExternalStore} from 'react';
import {Board} from '@/components/table/Board';
import {Seat} from '@/components/table/Seat';
import {HandOutcomeBanner, type HandOutcomeState} from '@/components/table/HandOutcome';
import {rotateSeats} from '@/lib/utils';
import type {TableSnapshot} from '@/lib/api/table';

// Portrait handhelds get a different experience, not a shrunk table: a tall
// capsule ringed by compact opponents, with the viewer promoted to a hero HUD
// (large hole cards) docked above the action bar. Landscape phones, tablets in
// landscape, and desktop keep the classic oval. Selected per layout tree via
// matchMedia instead of stacking CSS overrides on one DOM — the geometry of
// the two stages is too different to patch across breakpoints.
const VERTICAL_STAGE_QUERY = '(orientation: portrait) and (max-width: 1023px)';

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
  pot: number;
  bigBlind: number;
  nowMs: number;
  outcome: HandOutcomeState | null;
  holdOutcomeOpen: boolean;
};

export function TableStage({snapshot, viewer, pot, bigBlind, nowMs, outcome, holdOutcomeOpen}: Props) {
  const vertical = useVerticalStage();
  const seats = rotateSeats(snapshot.seats, viewer);
  const seatNode = (seat: TableSnapshot['seats'][number], index: number) =>
    <Seat key={seat.player_id} seat={seat} index={index}
          isTurn={snapshot.current_player_id === seat.player_id}
          payout={snapshot.payouts?.[seat.player_id] || 0}
          deadlineMs={snapshot.action_deadline_unix_ms}
          nowMs={nowMs}
          bigBlind={bigBlind}
          isViewer={seat.player_id === viewer}/>;
  const board = <Board cards={snapshot.board} pot={pot} rake={snapshot.rake} bigBlind={bigBlind}/>;

  if (!vertical) return (
    <div className="game-table">
      <div className="game-rail"/>
      <div className="game-felt">{board}</div>
      {seats.map(seatNode)}
      <HandOutcomeBanner outcome={outcome} holdOpen={holdOutcomeOpen}/>
    </div>
  );

  // rotateSeats guarantees the viewer (when seated) is first; that seat leaves
  // the ring entirely and becomes the hero HUD at the stage's bottom edge.
  const viewerFirst = seats[0]?.player_id === viewer;
  const opponents = viewerFirst ? seats.slice(1) : seats;
  return (
    <div className="game-table stage-v">
      <div className="stage-v-ring">
        <div className="game-rail"/>
        <div className="game-felt">{board}</div>
        {opponents.map((seat, i) => seatNode(seat, i + 1))}
        <HandOutcomeBanner outcome={outcome} holdOpen={holdOutcomeOpen}/>
      </div>
      {viewerFirst && seatNode(seats[0], 0)}
    </div>
  );
}
