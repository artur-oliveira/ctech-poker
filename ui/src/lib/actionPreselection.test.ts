import assert from 'node:assert/strict';
import {expect, test} from 'vitest';
import {resolvePreselection} from './actionPreselection.ts';

test('check/fold checks when checking is free', () => {
  expect(resolvePreselection('check_fold', ['check', 'raise'])).toBe('check');
});

test('check/fold folds instead of calling a bet', () => {
  expect(resolvePreselection('check_fold', ['fold', 'call', 'raise'])).toBe('fold');
});

test('fold never falls through to another action', () => {
  assert.equal(resolvePreselection('fold', ['check', 'raise']), null);
});

test('fixed call only pays the amount selected', () => {
  assert.equal(resolvePreselection('call', ['fold', 'call', 'raise'], 40, 40), 'call');
  assert.equal(resolvePreselection('call', ['fold', 'call', 'raise'], 80, 40), null);
});

test('call any pays the current amount or checks for free', () => {
  assert.equal(resolvePreselection('call_any', ['fold', 'call', 'raise'], 200), 'call');
  assert.equal(resolvePreselection('call_any', ['fold', 'check', 'raise']), 'check');
});
