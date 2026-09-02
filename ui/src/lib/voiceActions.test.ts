import assert from 'node:assert/strict';
import {test} from 'vitest';
import {isVoiceCancellation, isVoiceConfirmation, parseVoiceAction} from './voiceActions.ts';

for (const [speech, expected] of [
  ['desisto', {action: 'fold'}],
  ['check', {action: 'check'}],
  ['pagar', {action: 'call'}],
  ['aumentar', {action: 'raise'}],
  ['all in', {action: 'raise', allIn: true}]
] as const) {
  test(`maps ${speech}`, () => assert.deepEqual(parseVoiceAction(speech), expected));
}

test('rejects unrelated speech', () => assert.equal(parseVoiceAction('boa mão'), null));

test('captures a specific voiced raise total', () =>
  assert.deepEqual(parseVoiceAction('aumentar para 1.500'), {action: 'raise', amount: 1500}));

for (const speech of ['confirmar', 'confirma', 'confirmo', 'Confirmar!', 'isso', 'pode ir']) {
  test(`treats "${speech}" as a confirmation`, () => assert.equal(isVoiceConfirmation(speech), true));
}

for (const speech of ['aumentar', 'cancelar', 'não', '']) {
  test(`does not treat "${speech}" as a confirmation`, () => assert.equal(isVoiceConfirmation(speech), false));
}

for (const speech of ['cancelar', 'cancela', 'não', 'esquece', 'parar']) {
  test(`treats "${speech}" as a cancellation`, () => assert.equal(isVoiceCancellation(speech), true));
}

test('does not treat "confirmar" as a cancellation', () =>
  assert.equal(isVoiceCancellation('confirmar'), false));
