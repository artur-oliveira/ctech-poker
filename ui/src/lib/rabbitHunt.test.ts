import assert from 'node:assert/strict';
import test from 'node:test';
import type {DeckCard} from './deckVerify.ts';
import {rabbitRunout} from './rabbitHunt.ts';

const deck = Array.from({length: 52}, (_, index) => ({
  rank: 2, suit: 0, code: `c${index}`
})) satisfies DeckCard[];

test('rabbit runout honors all three burn cards', () => {
  assert.deepEqual(rabbitRunout(deck, 2, 0), ['c5', 'c6', 'c7', 'c9', 'c11']);
  assert.deepEqual(rabbitRunout(deck, 2, 3), ['c9', 'c11']);
  assert.deepEqual(rabbitRunout(deck, 2, 4), ['c11']);
});
