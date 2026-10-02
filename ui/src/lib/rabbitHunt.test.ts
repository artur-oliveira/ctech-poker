import {describe, expect, test} from 'vitest';
import type {TableSnapshot} from '@/lib/api/table';
import {mockPartialDeckProof} from '@/dev/mockRabbitProof';
import {rabbitDeckIndex, rabbitHuntFee, verifyRabbitCard} from './rabbitHunt';

const SEED = '11'.repeat(32);

function snapshot(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
  return {
    stage: 'complete', board: [], won_without_showdown: true, protocol_version: 12,
    shuffle_commit_hash: 'commit', rabbit_hunt_fee: 10,
    seats: [
      {player_id: 'viewer', stack: 1000, state: 'active', contributed: 0, dealt_in: true},
      {player_id: 'other', stack: 1000, state: 'folded', contributed: 0, dealt_in: true},
    ],
    ...overrides,
  };
}

describe('rabbitDeckIndex', () => {
  test('honours the flop, turn and river burns', () => {
    expect([0, 1, 2, 3, 4].map(slot => rabbitDeckIndex(2, slot))).toEqual([5, 6, 7, 9, 11]);
  });
});

describe('rabbitHuntFee', () => {
  test('offers the per-card price on an eligible hand', () => {
    expect(rabbitHuntFee(snapshot(), 'viewer')).toBe(10);
  });
  test.each([
    {protocol_version: 11},
    {stage: 'river'},
    {won_without_showdown: false},
    {board: ['AH', 'KD', 'QS', 'JC', 'TH']},
    {shuffle_commit_hash: undefined},
    {rabbit_hunt_fee: 0},
  ] satisfies Partial<TableSnapshot>[])('is closed when %o', overrides => {
    expect(rabbitHuntFee(snapshot(overrides), 'viewer')).toBe(0);
  });
  test('is closed to a viewer who was not dealt in', () => {
    expect(rabbitHuntFee(snapshot(), 'spectator')).toBe(0);
  });
});

describe('verifyRabbitCard', () => {
  test('accepts a bought card whose salt and proof rebuild the root commit', async () => {
    const index = rabbitDeckIndex(2, 3);
    const proof = await mockPartialDeckProof(SEED, [index]);
    const snap = snapshot({
      root_commit_hash: proof.root, revealed_card_salts: proof.revealed, unrevealed_card_hashes: proof.unrevealed,
      rabbit_cards: {3: proof.deck[index].code},
    });
    expect(await verifyRabbitCard(snap, 3)).toBe(true);
  });

  test('rejects a card that is not the one salted at that slot', async () => {
    const index = rabbitDeckIndex(2, 3);
    const proof = await mockPartialDeckProof(SEED, [index]);
    const wrong = proof.deck.find(card => card.code !== proof.deck[index].code)!.code;
    const snap = snapshot({
      root_commit_hash: proof.root, revealed_card_salts: proof.revealed, unrevealed_card_hashes: proof.unrevealed,
      rabbit_cards: {3: wrong},
    });
    expect(await verifyRabbitCard(snap, 3)).toBe(false);
  });

  test('rejects a proof that does not rebuild the published root', async () => {
    const index = rabbitDeckIndex(2, 4);
    const proof = await mockPartialDeckProof(SEED, [index]);
    const snap = snapshot({
      root_commit_hash: 'ff'.repeat(32), revealed_card_salts: proof.revealed,
      unrevealed_card_hashes: proof.unrevealed, rabbit_cards: {4: proof.deck[index].code},
    });
    expect(await verifyRabbitCard(snap, 4)).toBe(false);
  });
});
