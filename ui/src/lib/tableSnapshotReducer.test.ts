import assert from 'node:assert/strict';
import {test} from 'vitest';
import type {TableSnapshot} from '@/lib/api/table';
import {applySnapshotEquity, reduceTableSnapshot, revealViewerCards} from './tableSnapshotReducer.ts';

function snapshot(version = 4): TableSnapshot {
  return {
    snapshot_version: version,
    protocol_version: 6,
    hand_id: 'hand-1',
    seats: [
      {player_id: 'viewer', equity: 0.2},
      {player_id: 'muted', equity: 0.8}
    ],
    chat_messages: [
      {id: 'c1', player_id: 'viewer', message: 'oi'},
      {id: 'c2', player_id: 'muted', message: 'hidden'}
    ],
    reactions: [
      {id: 'r1', player_id: 'viewer', reaction_id: 'respect', expires_at: 2000},
      {id: 'r2', player_id: 'muted', reaction_id: 'respect', expires_at: 2000},
      {id: 'r3', player_id: 'viewer', reaction_id: 'unknown', expires_at: 2000},
      {id: 'r4', player_id: 'viewer', reaction_id: 'respect', expires_at: 999}
    ]
  } as TableSnapshot;
}

test('snapshot reduction rejects regressions and suppresses social events only', () => {
  assert.equal(reduceTableSnapshot(snapshot(3), 4, undefined, 1000), null);
  const source = snapshot();
  const reduced = reduceTableSnapshot(source, 4, new Set(['muted']), 1000);
  assert.ok(reduced);
  assert.equal(reduced.snapshot, source);
  assert.equal(reduced.version, 4);
  assert.equal(reduced.handId, 'hand-1');
  assert.deepEqual(reduced.chat.map(item => item.id), ['c1']);
  assert.deepEqual(reduced.reactions.map(item => item.id), ['r1']);
  assert.equal(reduced.snapshot.seats.length, 2, 'suppression must never remove poker seats');
});

test('legacy snapshots preserve poker state without hydrating modern social state', () => {
  const source = {...snapshot(), protocol_version: 5} as TableSnapshot;
  const reduced = reduceTableSnapshot(source, -1, undefined, 1000);
  assert.ok(reduced);
  assert.deepEqual(reduced.chat, []);
  assert.deepEqual(reduced.reactions, []);
});

test('equity and legacy reveal reconciliation update only the matching seat', () => {
  const source = snapshot();
  const equity = applySnapshotEquity(source, 'viewer', 0.75)!;
  assert.equal(equity.seats[0].equity, 0.75);
  assert.equal(equity.seats[1].equity, 0.8);
  const revealed = revealViewerCards(source, 'viewer')!;
  assert.deepEqual(revealed.seats[0].hole_cards_revealed, [true, true]);
  assert.equal(revealed.seats[1].hole_cards_revealed, undefined);
  assert.equal(applySnapshotEquity(null, 'viewer', 1), null);
  assert.equal(revealViewerCards(null, 'viewer'), null);
});
