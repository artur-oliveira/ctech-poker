import {render} from '@testing-library/react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {MOCK_PLAYER_ID, snapshotForScenario} from '@/dev/mockRuntime';
import {applySnapshotEquity} from '@/lib/tableSnapshotReducer';
import {TableStage} from '@/components/table/TableStage';
import {Chat} from '@/components/table/Chat';
import {TableReactions} from '@/components/table/TableReactions';
import {LastWinners} from '@/components/table/LastWinners';

/** Renders-per-snapshot budget for the table (#230).
 *
 * `Seat` is memoised, so counting how many times its body runs is the only
 * honest measure of "did that frame re-render the whole table?". Every seat
 * renders exactly one `PlayerAvatar`, and nothing else on the stage does, so
 * mocking that leaf turns seat renders into an assertable number without
 * putting a counter in production code. */
vi.mock('@/lib/hooks/useDeckVariant', () => ({useDeckVariant: () => 'four-color'}));

const seatRenders: string[] = [];
vi.mock('@/components/ui/player-avatar', () => ({
  PlayerAvatar: ({name}: {name?: string}) => {
    seatRenders.push(name || 'anon');
    return <div data-testid="avatar"/>;
  },
}));

// Stable identities, exactly as the table page now hands them down: the whole
// point of the memo boundary is that these do not change per frame.
const noop = () => undefined;
const playerNotes = {};

function stage(snapshot: ReturnType<typeof snapshotForScenario>, chatBubbles?: Record<string, {
  id: string;
  message: string
}>) {
  return <TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID} maxSeats={9} seatLayoutKey="room-1"
                     pot={0} bigBlind={50} nowMs={1_700_000_000_000} outcome={null} holdOutcomeOpen={false}
                     playerNotes={playerNotes} onEditPlayerNoteAction={noop}
                     onPeekCardsAction={noop} onCancelExitAction={noop}
                     chatBubbles={chatBubbles}/>;
}

describe('table render budget', () => {
  beforeEach(() => {
    seatRenders.length = 0;
  });

  test('a nine-max table renders each seat exactly once on the first snapshot', () => {
    const snapshot = snapshotForScenario('nine_max');
    render(stage(snapshot));
    expect(seatRenders).toHaveLength(snapshot.seats.length);
    expect(new Set(seatRenders).size).toBe(snapshot.seats.length);
  });

  test('a chat bubble re-renders one seat, not the table', () => {
    const snapshot = snapshotForScenario('nine_max');
    const {rerender} = render(stage(snapshot));
    seatRenders.length = 0;

    const talker = snapshot.seats.find(seat => seat.player_id !== MOCK_PLAYER_ID)!;
    rerender(stage(snapshot, {[talker.player_id]: {id: 'm1', message: 'boa'}}));
    expect(seatRenders).toEqual([talker.name || 'anon']);
  });

  test('an equity delta re-renders only the seat whose equity moved', () => {
    const snapshot = snapshotForScenario('nine_max');
    const {rerender} = render(stage(snapshot));
    seatRenders.length = 0;

    // The reducer's own shape: a new snapshot, a new seats array, and every
    // untouched seat object kept by identity. That is what lets one delta cost
    // one seat instead of nine.
    const target = snapshot.seats.find(seat => seat.player_id !== MOCK_PLAYER_ID)!;
    rerender(stage(applySnapshotEquity(snapshot, target.player_id, 0.42)!));
    expect(seatRenders).toEqual([target.name || 'anon']);
  });

  test('a state frame that moves one player re-renders only the seats it touched', () => {
    const snapshot = snapshotForScenario('nine_max');
    const {rerender} = render(stage(snapshot));
    seatRenders.length = 0;

    const actor = snapshot.seats.find(seat => seat.player_id !== MOCK_PLAYER_ID)!;
    rerender(stage({
      ...snapshot,
      current_player_id: actor.player_id,
      seats: snapshot.seats.map(seat =>
        seat.player_id === actor.player_id ? {...seat, stack: seat.stack - 100, contributed: 100} : seat),
    }));
    // The actor (stack + contribution) plus the seats whose `isTurn` changed
    // hands. A third of the ring, where a bare snapshot swap used to cost all
    // nine seats — timers, count-ups and cards included.
    expect(seatRenders.length).toBeLessThanOrEqual(3);
    expect(seatRenders.length).toBeLessThan(snapshot.seats.length);
    expect(seatRenders).toContain(actor.name || 'anon');
  });

  test('the asides are memo boundaries, so a snapshot alone cannot re-render them', () => {
    // Their props are the page's memoised projections (`seatRoster`,
    // `favoriteReactions`, the cached panel handlers). Without the boundary the
    // projections would be pointless; without the projections the boundary
    // would be. Both halves have to hold.
    for (const surface of [Chat, TableReactions, LastWinners]) {
      expect((surface as unknown as {$$typeof: symbol}).$$typeof).toBe(Symbol.for('react.memo'));
    }
  });
});
