import assert from 'node:assert/strict';
import {test} from 'vitest';
import {
  cardHashHex,
  parseCardCode,
  rootCommitHash,
  verifyWirePartialDeck,
  type WireCardReveal
} from './deckVerify.ts';

test('parseCardCode uses the same rank and suit encoding as Go', () => {
  assert.deepEqual(parseCardCode('As'), {rank: 14, suit: 3, code: 'As'});
  assert.deepEqual(parseCardCode('2c'), {rank: 2, suit: 0, code: '2c'});
  assert.throws(() => parseCardCode('ZZ'));
});

test('wire partial proof verifies and rejects a tampered card', async () => {
  const revealed: Record<number, WireCardReveal> = {};
  const hidden: Record<number, string> = {};
  const hashes: string[] = [];
  for (let index = 0; index < 52; index++) {
    const salt = index.toString(16).padStart(64, '0');
    const card = index === 5 ? 'As' : '2c';
    const parsed = parseCardCode(card);
    const hash = await cardHashHex(salt, parsed.rank, parsed.suit);
    hashes.push(hash);
    if (index === 5) revealed[index] = {card, salt_hex: salt};
    else hidden[index] = hash;
  }
  const root = await rootCommitHash(hashes);
  assert.equal((await verifyWirePartialDeck(root, revealed, hidden)).matches, true);
  revealed[5] = {...revealed[5], card: 'Ks'};
  assert.equal((await verifyWirePartialDeck(root, revealed, hidden)).matches, false);
});
