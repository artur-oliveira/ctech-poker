/** Chip stacks read relative to the table's big blind, not raw chip counts, so a
 * min-bet at a 500/1000 table and a min-bet at a 5/10 table both render as "tier 1". */
export const CHIP_TIER_MAX = 5;

export function chipTier(amount: number, bigBlind: number): number {
  if (amount <= 0) return 0;
  const ratio = amount / Math.max(1, bigBlind);
  return Math.min(CHIP_TIER_MAX, Math.max(1, Math.floor(Math.log2(ratio + 1)) + 1));
}
