import assert from 'node:assert/strict';
import {test} from 'vitest';
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

test('text export labels a partial file when action history is unavailable', () => {
  const text = serializeHand({
    pk: 'viewer', sk: 'h2', table_id: 't2', hand_id: 'h2', outcome: 'tied',
    net_change: 0, ended_at: 1_700_000_000_000
  }, [], 'viewer', false);
  assert.match(text, /Resultado: Empate/);
  assert.match(text, /Ações:\nIndisponíveis no momento da exportação/);
});

test('text export names unknown players, empty history and a loss without a board', () => {
  const text = serializeHand({
    pk: 'viewer', sk: 'h3', table_id: 't3', hand_id: 'h3', outcome: 'lost',
    net_change: -80, ended_at: 1_700_000_000_000,
    opponents: [{player_id: 'p2', won: true}]
  }, []);
  assert.match(text, /Resultado: Derrota \(-80 fichas\)/);
  assert.match(text, /Suas cartas: não disponíveis/);
  assert.match(text, /Board: sem board/);
  assert.match(text, /Adversário: cartas não reveladas \(vencedor\)/);
  assert.match(text, /Ações:\nNenhuma ação registrada/);
  assert.doesNotMatch(text, /Commit hash/);
});

test('text export falls back for an unlabelled action, a table event and a missing timestamp', () => {
  const text = serializeHand({
    pk: 'viewer', sk: 'h4', table_id: 't4', hand_id: 'h4', outcome: 'won',
    net_change: 10, ended_at: 1_700_000_000_000,
    opponents: [{player_id: 'abcdefgh1234', name: ''}]
  }, [
    {seq: 1, player_id: '', action: 'runout_step', amount: 0, timestamp: 0},
    {seq: 2, player_id: 'abcdefgh1234', action: 'set_run_it_twice', amount: 0, timestamp: 1_700_000_000_000},
  ], 'viewer');
  assert.match(text, /--:--:--\s+Mesa: runout_step/);
  assert.match(text, /Adversário: set_run_it_twice/);
});
