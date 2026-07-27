import assert from 'node:assert/strict';
import test from 'node:test';
import {resolvePreselection} from './actionPreselection.ts';

test('check/fold checks when checking is free', () => {
  assert.equal(resolvePreselection('check_fold', ['check', 'raise']), 'check');
});

test('check/fold folds instead of calling a bet', () => {
  assert.equal(resolvePreselection('check_fold', ['fold', 'call', 'raise']), 'fold');
});

test('fold never falls through to another action', () => {
  assert.equal(resolvePreselection('fold', ['check', 'raise']), null);
});
