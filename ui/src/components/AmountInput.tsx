'use client';
import {useLayoutEffect, useRef, useState, type ComponentProps} from 'react';
import {Input} from '@aoctech/ui';
import {formatAmount, parseAmountInput, parseAmountText} from '@/lib/amountInput';

type AmountInputProps = Omit<ComponentProps<typeof Input>, 'value' | 'onChange' | 'type' | 'max' | 'min' | 'step' | 'maxLength'> & {
  /** Integer value: centavos when `decimals` is 2, whole units when 0. `null` is empty. */
  value: number | null;
  onValueChange: (value: number | null) => void;
  decimals?: 0 | 2;
  /** Largest accepted value, in the same unit as `value`. Typing past it is ignored. */
  max: number;
  /** Shown inside the field before the number, e.g. "R$". */
  prefix?: string;
};

/** Caret index after the n-th digit/comma of `text`, so reformatting does not throw the caret to the end. */
function caretAfter(text: string, count: number): number {
  if (count <= 0) return 0;
  let seen = 0;
  for (let i = 0; i < text.length; i++) {
    if (/[\d,]/.test(text[i])) seen++;
    if (seen === count) return i + 1;
  }
  return text.length;
}

/**
 * A typed amount: pt-BR grouping ("1.234,56"), at most two decimals, a ceiling, and an
 * optional "R$" prefix. Digits only, so a pasted "2193921932193219" or a letter never lands.
 * Controlled by an integer (never a float); see lib/amountInput.ts.
 */
export function AmountInput({value, onValueChange, decimals = 2, max, prefix, className, ...rest}: AmountInputProps) {
  const [text, setText] = useState(formatAmount(value, decimals));
  const ref = useRef<HTMLInputElement>(null);
  const caret = useRef<number | null>(null);

  // An outside change (form reset, server value) wins; our own typing already matches.
  if (parseAmountText(text, decimals) !== value) setText(formatAmount(value, decimals));

  useLayoutEffect(() => {
    if (caret.current !== null && ref.current) ref.current.setSelectionRange(caret.current, caret.current);
    caret.current = null;
  }, [text]);

  return <div className={`amount-input${className ? ` ${className}` : ''}`}>
    {prefix && <span className="amount-input-prefix" aria-hidden="true">{prefix}</span>}
    <Input compact={false} {...rest} ref={ref} type="text" inputMode={decimals === 2 ? 'decimal' : 'numeric'} autoComplete="off"
           maxLength={formatAmount(max, decimals).length} className={prefix ? 'amount-input-field has-prefix' : 'amount-input-field'}
           value={text}
           onChange={event => {
             const input = event.target;
             const before = input.value.slice(0, input.selectionStart ?? input.value.length);
             const parsed = parseAmountInput(input.value, {decimals, max}, text);
             caret.current = caretAfter(parsed.text, (before.match(/[\d,]/g) ?? []).length);
             setText(parsed.text);
             onValueChange(parsed.value);
           }}/>
  </div>;
}
