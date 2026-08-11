import assert from 'node:assert/strict';
import {test} from 'vitest';
import type {TableSnapshot} from './api/table.ts';
import {
  playerPotBreakdown,
  relevantRunnerUp,
  seatParticipated,
  shouldShowOutcome,
  tableOutcomeKind,
  winnerStandings
} from './tableOutcome.ts';

const snapshot: TableSnapshot = {
  stage: 'complete',
  board: [],
  seats: [
    {player_id: 'main', stack: 100, state: 'all_in', dealt_in: true, contributed: 50},
    {player_id: 'side', stack: 200, state: 'all_in', dealt_in: true, contributed: 100},
    {player_id: 'loser', stack: 0, state: 'sitting_out', dealt_in: true, contributed: 100}
  ],
  winners: ['main', 'side'],
  pot_results: [
    {
      amount: 150, payout_amount: 150,
      eligible_player_ids: ['main', 'side', 'loser'], winner_player_ids: ['main'], payouts: {main: 150}
    },
    {
      amount: 100, payout_amount: 100,
      eligible_player_ids: ['side', 'loser'], winner_player_ids: ['side'], payouts: {side: 100}
    }
  ]
};

test('different side-pot winners are never mislabeled as a tie', () => {
  assert.equal(tableOutcomeKind(snapshot, 'main'), 'win');
  assert.equal(tableOutcomeKind(snapshot, 'side'), 'mixed');
  assert.equal(tableOutcomeKind(snapshot, 'loser'), 'lose');
});

test('orders side-pot winners by chips won instead of presenting a tie', () => {
  assert.deepEqual(winnerStandings(snapshot), [
    {playerId: 'main', amount: 150, place: 1, tied: false},
    {playerId: 'side', amount: 100, place: 2, tied: false}
  ]);
});

test('only winners sharing the same pot are tied', () => {
  const tied = {
    ...snapshot,
    pot_results: [{
      amount: 150, payout_amount: 150,
      eligible_player_ids: ['main', 'side'], winner_player_ids: ['main', 'side']
    }]
  };
  assert.equal(tableOutcomeKind(tied, 'main'), 'tie');
  assert.equal(tableOutcomeKind(tied, 'side'), 'tie');
  assert.deepEqual(winnerStandings(tied).map(item => ({playerId: item.playerId, tied: item.tied})), [
    {playerId: 'main', tied: true}, {playerId: 'side', tied: true}
  ]);
});

test('winning one pot and losing another is a mixed result', () => {
  const mixed = {
    ...snapshot,
    pot_results: [
      snapshot.pot_results![0],
      {...snapshot.pot_results![1], eligible_player_ids: ['main', 'side', 'loser']}
    ]
  };
  assert.equal(tableOutcomeKind(mixed, 'main'), 'mixed');
});

test('losing the contested pot while your own uncalled excess is refunded is still a clean loss, not mixed', () => {
  // The exact shape of an all-in that gets fully called for less than
  // shoved: one contested pot (you vs. the caller, you lose it) plus a
  // refund-only layer no one else ever staked (your own excess chips
  // coming back). `tableOutcomeKind` must not count that refund layer as a
  // pot you "won" — the player put no one else's money at risk in it.
  const shortCallLoss: TableSnapshot = {
    ...snapshot,
    winners: ['side'],
    pot_results: [
      {
        amount: 100, payout_amount: 100,
        eligible_player_ids: ['main', 'side'], winner_player_ids: ['side'], payouts: {side: 100}
      },
      {
        amount: 40, payout_amount: 40, eligible_player_ids: ['main'],
        winner_player_ids: [], payouts: {main: 40}, refund: true
      }
    ]
  };
  assert.equal(tableOutcomeKind(shortCallLoss, 'main'), 'lose');
});

test('relevantRunnerUp picks the best-scoring revealed opponent from a pot the viewer won', () => {
  const withScores: TableSnapshot = {
    ...snapshot,
    seats: [
      {
        player_id: 'main', stack: 100, state: 'all_in', dealt_in: true, contributed: 50,
        hand_score: 500, hole_cards_revealed: [true, true]
      },
      {
        player_id: 'side', stack: 200, state: 'all_in', dealt_in: true, contributed: 100,
        hand_score: 300, hole_cards_revealed: [true, true]
      },
      {
        player_id: 'loser', stack: 0, state: 'sitting_out', dealt_in: true, contributed: 100,
        hand_score: 100, hole_cards_revealed: [true, true]
      }
    ]
  };
  assert.equal(relevantRunnerUp(withScores, 'main')?.player_id, 'side');
});

test('relevantRunnerUp is undefined when the runner-up never revealed their cards', () => {
  const unrevealed: TableSnapshot = {
    ...snapshot,
    seats: snapshot.seats.map(seat => ({...seat, hand_score: seat.player_id === 'loser' ? 100 : undefined}))
  };
  assert.equal(relevantRunnerUp(unrevealed, 'main'), undefined);
});

test('contested winnings and refunds remain separate', () => {
  const withRefund = {
    ...snapshot,
    payouts: {main: 190},
    pot_results: [
      snapshot.pot_results![0],
      {
        amount: 40, payout_amount: 40, eligible_player_ids: ['main'],
        winner_player_ids: [], payouts: {main: 40}, refund: true
      }
    ]
  };
  assert.deepEqual(playerPotBreakdown(withRefund, 'main'), {credit: 190, won: 150, refund: 40});
});

test('a busted showdown participant still receives the outcome', () => {
  assert.equal(shouldShowOutcome(snapshot.seats[2]), true);
});

test('a folded participant still receives the outcome and reveal controls', () => {
  assert.equal(shouldShowOutcome({...snapshot.seats[0], state: 'folded'}), true);
});

test('protocol-v1 participation falls back to the viewer cards', () => {
  assert.equal(seatParticipated({
    player_id: 'old', stack: 10, state: 'active', contributed: 10, hole_cards: ['Ah', 'Kd']
  }), true);
  assert.equal(seatParticipated({
    player_id: 'waiting', stack: 10, state: 'active', contributed: 0
  }), false);
});
