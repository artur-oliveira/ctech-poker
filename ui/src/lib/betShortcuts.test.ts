import assert from 'node:assert/strict';
import {test} from 'vitest';
import {betShortcutAmount, clampSnapRaise, FAST_STEP_STRIDE, stageBetPresets} from './betShortcuts.ts';
import {actionState} from './tableActions.ts';

test('A selects all-in amount without representing submission', () => {
  assert.equal(betShortcutAmount('a', 100, 20, 500, 10), 500);
});

test('arrow accessibility shortcuts remain bounded and support fast steps', () => {
  assert.equal(betShortcutAmount('ArrowLeft', 25, 20, 500, 10), 20);
  assert.equal(betShortcutAmount('ArrowRight', 100, 20, 500, 10 * FAST_STEP_STRIDE), 130);
  assert.equal(betShortcutAmount('ArrowUp', 100, 20, 500, 10), 500);
  assert.equal(betShortcutAmount('ArrowDown', 100, 20, 500, 10), 20);
});

test('h without a half-pot figure falls back to the minimum rather than guessing', () => {
  assert.equal(betShortcutAmount('h', 100, 20, 500, 10), 20);
  assert.equal(betShortcutAmount('h', 100, 20, 500, 10, 250), 250);
  assert.equal(betShortcutAmount('Escape', 100, 20, 500, 10), undefined);
});

const server = [
  {label: 'Mín', value: 100},
  {label: '⅓ pote', value: 220},
  {label: '½ pote', value: 250},
  {label: '⅔ pote', value: 300},
  {label: 'Máx', value: 4000},
];

test('mixed opens in big blinds pre-flop and sizes against the pot afterwards', () => {
  const preflop = stageBetPresets({
    mode: 'mixed', stage: 'pre_flop', bigBlind: 100, serverPresets: server,
    minRaise: 100, maxRaise: 4000, raiseStep: 25,
  });
  assert.deepEqual(preflop, [
    {label: 'BB', value: 100}, {label: '2BB', value: 200},
    {label: '3BB', value: 300}, {label: 'All in', value: 4000},
  ]);

  const flop = stageBetPresets({
    mode: 'mixed', stage: 'flop', bigBlind: 100, serverPresets: server,
    minRaise: 100, maxRaise: 4000, raiseStep: 25,
  });
  // 220 snaps to the 25 grid.
  assert.deepEqual(flop.map(preset => preset.label), ['1/3', '1/2', '2/3', 'All in']);
  assert.equal(flop[0].value, 225);
});

test('bb and pot ignore the street entirely', () => {
  const options = {bigBlind: 100, serverPresets: server, minRaise: 100, maxRaise: 4000, raiseStep: 25} as const;
  assert.deepEqual(
    stageBetPresets({...options, mode: 'bb', stage: 'river'}).map(preset => preset.label),
    ['BB', '2BB', '3BB', 'All in']);
  assert.deepEqual(
    stageBetPresets({...options, mode: 'pot', stage: 'pre_flop'}).map(preset => preset.label),
    ['1/3', '1/2', '2/3', 'All in']);
});

test('a fraction the server did not price is dropped, never shown below a smaller one', () => {
  const presets = stageBetPresets({
    mode: 'pot', stage: 'flop', bigBlind: 100,
    serverPresets: [{label: 'Mín', value: 100}, {label: '½ pote', value: 175}],
    minRaise: 100, maxRaise: 4000, raiseStep: 25,
  });
  assert.deepEqual(presets, [{label: '1/2', value: 175}, {label: 'All in', value: 4000}]);
});

test('a short stack collapses onto All in instead of a fraction that means everything', () => {
  assert.deepEqual(stageBetPresets({
    mode: 'pot', stage: 'flop', bigBlind: 100, serverPresets: server,
    minRaise: 150, maxRaise: 150, raiseStep: 25,
  }), [{label: 'All in', value: 150}]);
});

test('no legal raise, no presets', () => {
  assert.deepEqual(stageBetPresets({
    mode: 'mixed', stage: 'flop', bigBlind: 100, serverPresets: server,
    minRaise: 400, maxRaise: 0, raiseStep: 25,
  }), []);
});

test('a zero big blind and a zero step cannot divide by zero', () => {
  assert.deepEqual(stageBetPresets({
    mode: 'bb', stage: 'flop', bigBlind: 0, serverPresets: server,
    minRaise: 10, maxRaise: 4000, raiseStep: 0,
  }).map(preset => preset.value), [10, 4000]);
});

test('actionState omits a pot fraction the server did not send', () => {
  const snapshot = {
    stage: 'flop', seats: [{player_id: 'me', stack: 900, state: 'active', dealt_in: true, contributed: 0}],
    legal_actions: {actions: ['raise'], min_raise_to: 100, max_raise_to: 900, step: 25, half_pot_raise_to: 175},
  } as unknown as Parameters<typeof actionState>[0];
  const labels = actionState(snapshot, 'me').raisePresets.map(preset => preset.label);
  assert.deepEqual(labels, ['Mín', '½ pote', 'Máx']);
});

test('clampSnapRaise snaps a typed amount to the table increment', () => {
  assert.equal(clampSnapRaise(733, 100, 1000, 25), 725);
  assert.equal(clampSnapRaise(740, 100, 1000, 25), 750);
});

test('clampSnapRaise clamps AFTER snapping, so a bound off the increment grid still wins', () => {
  // 610 is not a multiple of 25; snapping 605 to 600 must not fall below the minimum.
  assert.equal(clampSnapRaise(605, 610, 1000, 25), 610);
  assert.equal(clampSnapRaise(9999, 100, 990, 25), 990);
});

test('clampSnapRaise treats a zero increment as one chip rather than dividing by it', () => {
  assert.equal(clampSnapRaise(733, 100, 1000, 0), 733);
});
