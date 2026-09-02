import assert from 'node:assert/strict';
import {test} from 'vitest';
import {betShortcutAmount, FAST_STEP_STRIDE} from './betShortcuts.ts';

test('A selects all-in amount without representing submission', () => {
  assert.equal(betShortcutAmount('a', 100, 20, 500, 10), 500);
});

test('arrow accessibility shortcuts remain bounded and support fast steps', () => {
  assert.equal(betShortcutAmount('ArrowLeft', 25, 20, 500, 10), 20);
  assert.equal(betShortcutAmount('ArrowRight', 100, 20, 500, 10 * FAST_STEP_STRIDE), 130);
  assert.equal(betShortcutAmount('ArrowUp', 100, 20, 500, 10), 500);
  assert.equal(betShortcutAmount('ArrowDown', 100, 20, 500, 10), 20);
});
