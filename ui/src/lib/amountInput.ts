/**
 * Pure parsing/formatting behind AmountInput. Values travel as integers (centavos for
 * decimals=2, whole units for decimals=0), never floats. Display is pt-BR: "." thousands,
 * "," decimal.
 */
export type AmountFormat = {decimals: 0 | 2; max: number};

const group = (digits: string) => digits.replace(/\B(?=(\d{3})+(?!\d))/g, '.');

/** Integer value to its display text ("" for null). */
export function formatAmount(value: number | null, decimals: 0 | 2): string {
  if (value === null || !Number.isFinite(value)) return '';
  if (decimals === 0) return group(String(Math.trunc(value)));
  const cents = String(Math.trunc(value) % 100).padStart(2, '0');
  return `${group(String(Math.trunc(value / 100)))},${cents}`;
}

/**
 * What the player typed or pasted to the clean display text and its integer value.
 * Keeps digits and at most one decimal separator ("," or a lone "." followed by one or two
 * digits, the way people type "10.5"), truncates to `decimals` places, drops leading zeros,
 * and refuses (keeps the previous text) anything above `max`.
 */
export function parseAmountInput(raw: string, {decimals, max}: AmountFormat, previous: string): {text: string; value: number | null} {
  let s = raw.replace(/[^\d.,]/g, '');
  if (decimals === 2 && !s.includes(',') && /^\d+\.\d{1,2}$/.test(s)) s = s.replace('.', ',');
  const comma = decimals === 2 ? s.indexOf(',') : -1;
  const intPart = (comma === -1 ? s : s.slice(0, comma)).replace(/\D/g, '').replace(/^0+(?=\d)/, '');
  const hasComma = comma !== -1;
  const decPart = hasComma ? s.slice(comma + 1).replace(/\D/g, '').slice(0, 2) : '';
  if (!intPart && !hasComma) return {text: '', value: null};
  const value = Number(intPart || '0') * (decimals === 2 ? 100 : 1) + (decimals === 2 ? Number(decPart.padEnd(2, '0')) : 0);
  if (!Number.isSafeInteger(value) || value > max) return {text: previous, value: parseAmountText(previous, decimals)};
  const text = group(intPart || '0') + (hasComma ? `,${decPart}` : '');
  return {text, value};
}

/** Display text back to the integer value (null when empty). */
export function parseAmountText(text: string, decimals: 0 | 2): number | null {
  if (!text) return null;
  const [int = '', dec = ''] = text.replace(/\./g, '').split(',');
  return Number(int || '0') * (decimals === 2 ? 100 : 1) + (decimals === 2 ? Number(dec.padEnd(2, '0')) : 0);
}
