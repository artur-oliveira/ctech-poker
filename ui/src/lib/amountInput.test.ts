import {describe, expect, test} from 'vitest';
import {formatAmount, parseAmountInput, parseAmountText} from './amountInput';

const brl = {decimals: 2 as const, max: 100_000_000};
const parse = (raw: string, prev = '') => parseAmountInput(raw, brl, prev);

describe('amount input', () => {
  test('formats centavos pt-BR', () => {
    expect(formatAmount(123456, 2)).toBe('1.234,56');
    expect(formatAmount(5, 2)).toBe('0,05');
    expect(formatAmount(null, 2)).toBe('');
    expect(formatAmount(1250000, 0)).toBe('1.250.000');
  });

  test('groups thousands while typing and keeps the comma', () => {
    expect(parse('1500')).toEqual({text: '1.500', value: 150000});
    expect(parse('1.500,5')).toEqual({text: '1.500,5', value: 150050});
    expect(parse('1500,')).toEqual({text: '1.500,', value: 150000});
  });

  test('allows at most two decimals and a single comma', () => {
    expect(parse('10,999').text).toBe('10,99');
    expect(parse('10,5,5').text).toBe('10,55');
  });

  test('a lone dot with one or two digits is a decimal separator', () => {
    expect(parse('10.5')).toEqual({text: '10,5', value: 1050});
    expect(parse('1.500').text).toBe('1.500');
  });

  test('strips letters, leading zeros and empties', () => {
    expect(parse('R$ 007')).toEqual({text: '7', value: 700});
    expect(parse('abc')).toEqual({text: '', value: null});
    expect(parse('0,5')).toEqual({text: '0,5', value: 50});
  });

  test('refuses anything above the ceiling', () => {
    expect(parse('1000000')).toEqual({text: '1.000.000', value: 100_000_000});
    expect(parse('1000001', '1.000.000')).toEqual({text: '1.000.000', value: 100_000_000});
    expect(parse('2193921932193219', '10')).toEqual({text: '10', value: 1000});
  });

  test('whole units drop the decimal part', () => {
    const fichas = {decimals: 0 as const, max: 999_999_999_999};
    expect(parseAmountInput('1234,56', fichas, '')).toEqual({text: '123.456', value: 123456});
    expect(parseAmountInput('1000000000000', fichas, '5').text).toBe('5');
  });

  test('text round-trips to the integer', () => {
    expect(parseAmountText('1.234,5', 2)).toBe(123450);
    expect(parseAmountText('', 2)).toBeNull();
    expect(parseAmountText('7', 0)).toBe(7);
  });
});
