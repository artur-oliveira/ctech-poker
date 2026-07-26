import assert from 'node:assert/strict';
import test from 'node:test';
import type {TableSnapshot} from './api/table.ts';
import {seatParticipated, shouldShowOutcome, tableOutcomeKind} from './tableOutcome.ts';

const snapshot = {
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
      eligible_player_ids: ['main', 'side', 'loser'], winner_player_ids: ['main']
    },
    {
      amount: 100, payout_amount: 100,
      eligible_player_ids: ['side', 'loser'], winner_player_ids: ['side']
    }
  ]
} satisfies TableSnapshot;

test('different side-pot winners are wins, not a tie', () => {
  assert.equal(tableOutcomeKind(snapshot, 'main'), 'win');
  assert.equal(tableOutcomeKind(snapshot, 'side'), 'win');
  assert.equal(tableOutcomeKind(snapshot, 'loser'), 'lose');
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
});

test('a busted showdown participant still receives the outcome', () => {
  assert.equal(shouldShowOutcome(snapshot.seats[2]), true);
});

test('protocol-v1 participation falls back to the viewer cards', () => {
  assert.equal(seatParticipated({
    player_id: 'old', stack: 10, state: 'active', contributed: 10, hole_cards: ['Ah', 'Kd']
  }), true);
  assert.equal(seatParticipated({
    player_id: 'waiting', stack: 10, state: 'active', contributed: 0
  }), false);
});
