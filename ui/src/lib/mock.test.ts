import {describe, expect, test} from 'vitest';
import {MOCK_PLAYER_ID, snapshotForScenario, type MockScenario} from './mock';

const scenarios: MockScenario[] = [
  'full_hand', 'full_hand_loss', 'full_hand_tie', 'all_in', 'auto_fold',
  'waiting', 'pre_flop', 'flop', 'turn', 'river', 'showdown', 'side_pot',
  'reconnecting', 'action_error', 'timeout', 'complete_loss',
  'complete_tie', 'fold_win', 'complete',
];

describe('mock table state contract', () => {
  test.each(scenarios)('%s always returns a renderable backend snapshot', scenario => {
    const snapshot = snapshotForScenario(scenario);

    expect(snapshot.stage).toBeTruthy();
    expect(snapshot.seats.length).toBeGreaterThan(0);
    expect(snapshot.board.length).toBeLessThanOrEqual(5);
    expect(new Set(snapshot.seats.map(seat => seat.player_id)).size).toBe(snapshot.seats.length);
    expect(snapshot.seats.every(seat => seat.stack >= 0 && seat.contributed >= 0)).toBe(true);
    if (snapshot.current_player_id) {
      expect(snapshot.seats.some(seat => seat.player_id === snapshot.current_player_id)).toBe(true);
    }
  });

  test('waiting has no board, pot contribution or active decision', () => {
    const snapshot = snapshotForScenario('waiting');
    expect(snapshot).toMatchObject({stage: 'waiting_for_players', board: []});
    expect(snapshot.seats.every(seat => seat.contributed === 0)).toBe(true);
    expect(snapshot.current_player_id).toBeUndefined();
  });

  test('every street exposes the expected number of community cards', () => {
    expect(snapshotForScenario('pre_flop').board).toHaveLength(0);
    expect(snapshotForScenario('flop').board).toHaveLength(3);
    expect(snapshotForScenario('turn').board).toHaveLength(4);
    expect(snapshotForScenario('river').board).toHaveLength(5);
    expect(snapshotForScenario('showdown').board).toHaveLength(5);
  });

  test('normal decision exposes coherent call and raise limits', () => {
    const snapshot = snapshotForScenario('pre_flop');
    expect(snapshot.current_player_id).toBe(MOCK_PLAYER_ID);
    expect(snapshot.legal_actions?.actions).toEqual(['fold', 'call', 'raise']);
    expect(snapshot.legal_actions!.min_raise_to).toBeLessThan(snapshot.legal_actions!.max_raise_to!);
  });

  test.each([
    ['complete', [MOCK_PLAYER_ID], 1275],
    ['complete_loss', ['bia_sp'], 1275],
    ['complete_tie', [MOCK_PLAYER_ID, 'bia_sp'], 1275],
  ] as const)('%s payouts reconcile with the pot', (scenario, winners, total) => {
    const snapshot = snapshotForScenario(scenario);
    expect(snapshot.winners).toEqual(winners);
    expect(Object.values(snapshot.payouts || {}).reduce((sum, value) => sum + value, 0)).toBe(total);
    expect(snapshot.stage).toBe('complete');
  });

  test('fold win does not leak any private cards', () => {
    const snapshot = snapshotForScenario('fold_win');
    expect(snapshot.board).toEqual([]);
    expect(snapshot.seats.filter(seat => seat.player_id !== MOCK_PLAYER_ID)
      .flatMap(seat => seat.hole_cards || []).every(card => card === 'back')).toBe(true);
    expect(snapshot.seats.flatMap(seat => seat.hole_cards_revealed || []).every(Boolean)).toBe(false);
  });

  test('side pots list eligible players and reconcile their amounts', () => {
    const snapshot = snapshotForScenario('side_pot');
    expect(snapshot.pot_results?.length).toBeGreaterThan(1);
    expect(snapshot.pot_results?.every(pot => pot.amount > 0 && pot.eligible_player_ids.length > 0)).toBe(true);
  });
});
