import assert from 'node:assert/strict';
import test from 'node:test';
import {wasDecidedByKicker} from './pokerRules.ts';

test('different two-pair values are not mislabeled as a kicker decision', () => {
  assert.equal(wasDecidedByKicker(
    ['Th', 'Ts', '2d', '2s', 'Qh'],
    ['7h', '7s', '2d', '2s', 'Qh']
  ), false);
});

test('the fifth card can decide otherwise identical two-pair hands', () => {
  assert.equal(wasDecidedByKicker(
    ['Th', 'Ts', '2d', '2s', 'Ah'],
    ['Tc', 'Td', '2d', '2s', 'Qh']
  ), true);
});
