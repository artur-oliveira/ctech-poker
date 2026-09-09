import {describe, expect, test} from 'vitest';
import {BET_INPUT_ERRORS, betCommitNote, betInputMaxError, checkBetInput} from './betInput';

const sandbox = {maxRaise: 1000, allowFraction: false};
const money = {maxRaise: 1000, allowFraction: true};

function reason(value: string, options = sandbox) {
  const check = checkBetInput(value, options);
  return check.ok ? null : check.reason;
}

function amount(value: string, options = sandbox) {
  const check = checkBetInput(value, options);
  return check.ok ? check.amount : 'rejected';
}

describe('checkBetInput', () => {
  test('accepts a plain whole number and reports its value', () => {
    expect(amount('250')).toBe(250);
  });

  test('accepts the empty field as a transient state with no amount', () => {
    expect(amount('')).toBeNull();
  });

  test.each([
    ['a letter', '25a'],
    ['scientific notation', '1e3'],
    ['a leading plus', '+25'],
    ['a leading minus', '-25'],
    ['a space', '2 5'],
    ['a currency prefix', 'R$25']
  ])('rejects %s', (_label, value) => {
    expect(reason(value)).toBe(BET_INPUT_ERRORS.digits);
  });

  test('rejects a separator with no digit in front of it', () => {
    expect(reason(',5', money)).toBe(BET_INPUT_ERRORS.digits);
  });

  test('rejects a decimal separator in sandbox, where there are no fractional chips', () => {
    expect(reason('25,5')).toBe(BET_INPUT_ERRORS.whole);
    expect(reason('25.5')).toBe(BET_INPUT_ERRORS.whole);
  });

  test('accepts one separator and up to two fraction digits in real money', () => {
    expect(amount('25,5', money)).toBe(25.5);
    expect(amount('25,50', money)).toBe(25.5);
    expect(amount('25.50', money)).toBe(25.5);
  });

  test('accepts a trailing separator mid-typing but commits no amount yet', () => {
    expect(amount('25,', money)).toBeNull();
  });

  test('rejects a second separator in real money', () => {
    expect(reason('2,5,5', money)).toBe(BET_INPUT_ERRORS.oneSeparator);
  });

  test('rejects a third fraction digit in real money', () => {
    expect(reason('25,505', money)).toBe(BET_INPUT_ERRORS.fraction);
  });

  test('rejects a leading zero followed by a digit, and keeps a lone zero', () => {
    expect(reason('01')).toBe(BET_INPUT_ERRORS.leadingZero);
    expect(reason('007')).toBe(BET_INPUT_ERRORS.leadingZero);
    expect(amount('0')).toBe(0);
  });

  test('blocks the keystroke on the resulting VALUE, not on a digit count', () => {
    expect(amount('999')).toBe(999);
    // The fourth digit takes 999 to 9990 — over the 1.000 ceiling.
    expect(reason('9990')).toBe(betInputMaxError(1000));
    // ...but the ceiling itself is still typable, and one chip past it is not.
    expect(amount('1000')).toBe(1000);
    expect(reason('1001')).toBe(betInputMaxError(1000));
  });

  test('applies the same ceiling to a pasted value', () => {
    expect(reason('999999')).toBe(betInputMaxError(1000));
    expect(reason('1.200,00', money)).toBe(BET_INPUT_ERRORS.oneSeparator);
    expect(amount('750', {maxRaise: 1000, allowFraction: false})).toBe(750);
  });

  test('never blocks a value below the minimum — that would make a deep raise untypable', () => {
    // "2" on the way to 250000, at a table whose minimum is 250000.
    expect(amount('2', {maxRaise: 900_000, allowFraction: false})).toBe(2);
    expect(amount('250000', {maxRaise: 900_000, allowFraction: false})).toBe(250_000);
  });
});

describe('betCommitNote', () => {
  test('says nothing when the typed number survived untouched', () => {
    expect(betCommitNote(300, 300, 100)).toBe('');
  });

  test('names the minimum clamp with the copy the control already used', () => {
    expect(betCommitNote(7, 100, 100)).toBe('ajustado ao mínimo');
  });

  test('names the settled figure when the increment rounded it', () => {
    expect(betCommitNote(1234, 1225, 100)).toBe('arredondado para 1.225');
  });
});
