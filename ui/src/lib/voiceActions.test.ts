import assert from 'node:assert/strict';
import test from 'node:test';
import {parseVoiceAction} from './voiceActions.ts';

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
