'use client';
import {createContext, useContext} from 'react';
import {chipsExact, chipsShort} from '@/lib/chips';

/** Whether the surface under it renders play-money chips (abbreviated) or real
 * money (exact, always). Default `false` — abbreviation is opt-in, so every
 * surface that has not declared itself sandbox keeps full precision.
 *
 * A context and not a prop: the amount is painted by `Seat`, `Board`,
 * `ActionBar`, `HandOutcome`, `LastWinners` and `WinnerCards`, three of which
 * sit behind `memo` boundaries whose props must stay primitives or stable
 * identities (#230). The value flips at most once per table session, so
 * consumers re-render essentially never. */
export const ChipFormatContext = createContext(false);

/** The chip formatter for the current surface. Module-level function
 * identities, so passing it through a dependency array is free. */
export function useChipFormat(): (amount: number) => string {
  return useContext(ChipFormatContext) ? chipsShort : chipsExact;
}
