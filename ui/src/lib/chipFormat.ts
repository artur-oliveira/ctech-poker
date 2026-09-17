'use client';
import {createContext, useContext} from 'react';
import {chipsExact, chipsShort, moneyExact} from '@/lib/chips';

/** How the surface under it paints an amount.
 *
 * - `'exact'` — the default: unknown wallet, so full precision and no currency
 *   prefix. Abbreviation stays opt-in (a surface that never declared itself
 *   cannot silently round a number), and so does `R$` (the prefix is the one
 *   signal that tells a player they are looking at their own money, so no
 *   surface may print it by omission). `Seat`, `Board` and `TableStage` are
 *   deliberately wide — `HandReplayer` (`/hands/replay`, `/share`) renders them
 *   with no provider at all — and this is the value those surfaces get.
 * - `'chips'` — the live table's play-money chips: abbreviated on display,
 *   exact in the accessible name, whole units on input.
 * - `'money'` — real money: exact, always, always `R$`-prefixed, from centavos.
 *
 * A context and not a prop: the amount is painted by `Seat`, `Board`,
 * `ActionBar`, `HandOutcome`, `LastWinners` and `WinnerCards`, three of which
 * sit behind `memo` boundaries whose props must stay primitives or stable
 * identities (#230). A string literal is a primitive, and the value flips at
 * most once per table session, so consumers re-render essentially never. */
export type ChipFormatMode = 'exact' | 'chips' | 'money';

export const ChipFormatContext = createContext<ChipFormatMode>('exact');

/** The display formatter for the current surface: abbreviated chips at a live
 * table, exact prefixed money in a real room, exact chips everywhere else.
 * Module-level function identities, so passing it through a dependency array
 * is free. */
export function useChipFormat(): (amount: number) => string {
  const mode = useContext(ChipFormatContext);
  if (mode === 'money') return moneyExact;
  return mode === 'chips' ? chipsShort : chipsExact;
}

/** The same amount as an accessible name carries it: never abbreviated. Pair
 * it with `useChipUnit()`, because a chip count needs the noun spelled out and
 * a money figure already carries its unit in the `R$` prefix. */
export function useChipExact(): (amount: number) => string {
  return useContext(ChipFormatContext) === 'money' ? moneyExact : chipsExact;
}

/** The unit noun that follows an exact figure, space included — `' fichas'`
 * for chips, empty for money, which is already prefixed. */
export function useChipUnit(): string {
  return useContext(ChipFormatContext) === 'money' ? '' : ' fichas';
}

/** True where the surface renders the live table's play-money chips:
 * abbreviated on display, and whole units on input — a chip has no fraction. */
export function useSandboxChips(): boolean {
  return useContext(ChipFormatContext) === 'chips';
}
