import {chipsExact} from '@/lib/chips';

/** Real money is priced to the centavo; sandbox chips are whole units and
 * always will be, so a separator there is not a rounding choice, it is a
 * typo. */
export const BET_INPUT_FRACTION_DIGITS = 2;

export const BET_INPUT_ERRORS = {
  digits: 'Apenas números.',
  whole: 'As fichas não têm centavos.',
  oneSeparator: 'Use apenas uma vírgula.',
  fraction: `No máximo ${BET_INPUT_FRACTION_DIGITS} casas decimais.`,
  leadingZero: 'Sem zero à esquerda.'
} as const;

export function betInputMaxError(maxRaise: number): string {
  return `Máximo ${chipsExact(maxRaise)} fichas.`;
}

export type BetInputCheck =
  | { ok: true; amount: number | null }
  | { ok: false; reason: string };

/** The one filter every keystroke, paste and drop into the raise field goes
 * through. It answers "may the field *become* this?", never "is this a legal
 * raise?" — those are different questions and conflating them is what makes
 * amount fields unusable:
 *
 * - The **upper** bound blocks the keystroke, on the resulting *value* rather
 *   than on a digit count: with a 1.000 ceiling, `999` is accepted and the
 *   fourth digit (→ `9990`) is not, while `1000` still types cleanly.
 * - `minRaise` and `raiseStep` deliberately do NOT block anything. Blocking
 *   below the minimum makes a 250.000 raise untypable — you would be stopped
 *   at `2`. They are applied once, on blur/Enter, by `clampSnapRaise`.
 * - The empty string is a legal transient state (the player selected all and
 *   started over); the caller resets it to `minRaise` when focus leaves.
 *
 * Returns the parsed amount when the candidate is a complete number, and
 * `null` when it is a legal-but-incomplete draft (`''`, or `12,` mid-typing). */
export function checkBetInput(value: string, {maxRaise, allowFraction}: {
  maxRaise: number;
  allowFraction: boolean;
}): BetInputCheck {
  if (value === '') return {ok: true, amount: null};
  // Everything the platform can hand a text input that is not a digit or a
  // separator — letters, `e`, `+`, `-`, spaces, a pasted "R$ 1.200,00".
  if (!/^[\d.,]+$/.test(value)) return {ok: false, reason: BET_INPUT_ERRORS.digits};
  const separators = value.match(/[.,]/g)?.length ?? 0;
  if (separators > 0 && !allowFraction) return {ok: false, reason: BET_INPUT_ERRORS.whole};
  if (separators > 1) return {ok: false, reason: BET_INPUT_ERRORS.oneSeparator};
  if (!/^\d/.test(value)) return {ok: false, reason: BET_INPUT_ERRORS.digits};
  if (separators === 1 && !new RegExp(`^\\d+[.,]\\d{0,${BET_INPUT_FRACTION_DIGITS}}$`).test(value)) {
    return {ok: false, reason: BET_INPUT_ERRORS.fraction};
  }
  if (/^0\d/.test(value)) return {ok: false, reason: BET_INPUT_ERRORS.leadingZero};
  const amount = Number(value.replace(',', '.'));
  if (!Number.isFinite(amount)) return {ok: false, reason: BET_INPUT_ERRORS.digits};
  if (amount > maxRaise) return {ok: false, reason: betInputMaxError(maxRaise)};
  // A trailing separator ("12,") parses to 12, but the player is mid-number:
  // report no amount so nothing commits until they finish or leave the field.
  return {ok: true, amount: /[.,]$/.test(value) ? null : amount};
}

/** What to tell the player when blur/Enter moved the number they typed.
 *
 * The upper bound blocks a keystroke outright, so the field can only ever
 * settle somewhere else for the two reasons typing is deliberately NOT
 * blocked on: below the street minimum, or off the table's raise increment.
 * Silence there is the bug the read-only `<output>` never had — a number that
 * changes itself on blur with no explanation reads as the field losing input.
 * Empty string when nothing moved. */
export function betCommitNote(typed: number, settled: number, minRaise: number): string {
  if (settled === typed) return '';
  return typed < minRaise ? 'ajustado ao mínimo' : `arredondado para ${chipsExact(settled)}`;
}
