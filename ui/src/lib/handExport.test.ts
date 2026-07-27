import assert from 'node:assert/strict';
import test from 'node:test';
import {serializeHand} from './handExport.ts';

test('text export includes authorized cards, actions and fairness proof', () => {
  const text = serializeHand({
    pk: 'viewer', sk: 'h1', table_id: 't1', hand_id: 'h1', outcome: 'won',
    net_change: 150, ended_at: 1_700_000_000_000, board: ['Ah', 'Kd', '2c'],
    hole_cards: ['As', 'Ad'], opponents: [{player_id: 'p2', name: 'Bia'}], commit_hash: 'abc'
  }, [{seq: 1, player_id: 'viewer', action: 'raise', amount: 100, timestamp: 1_700_000_000_000}], 'viewer');
  assert.match(text, /Suas cartas: As Ad/);
  assert.match(text, /Você: Raise 100/);
  assert.match(text, /Bia: cartas não reveladas/);
  assert.match(text, /Commit hash: abc/);
});
