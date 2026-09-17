/** Chip stacks read relative to the table's big blind, not raw chip counts, so a
 * min-bet at a 500/1000 table and a min-bet at a 5/10 table both render as "tier 1". */
export const CHIP_TIER_MAX = 5;

export function chipTier(amount: number, bigBlind: number): number {
  if (amount <= 0) return 0;
  const ratio = amount / Math.max(1, bigBlind);
  return Math.min(CHIP_TIER_MAX, Math.max(1, Math.floor(Math.log2(ratio + 1)) + 1));
}

// --- Chip amount formatting -------------------------------------------------
// One source for every chip number the table paints. Sandbox stacks reach
// seven and eight digits, and "1.250.000" does not fit a 64px seat caption at
// any font size that stays legible — so sandbox amounts are abbreviated to at
// most four visible characters and the exact figure moves to the accessible
// name. Real money is never abbreviated: rounding somebody's balance to "1,2K"
// is not a display choice, it is a wrong number.

const CHIP_UNITS: [number, string][] = [[1e12, 'T'], [1e9, 'B'], [1e6, 'M'], [1e3, 'K']];

/** The exact figure, pt-BR grouped. Always what an accessible name carries. */
export function chipsExact(amount: number): string {
  return amount.toLocaleString('pt-BR');
}

/** Abbreviated to ≤4 visible characters ("999", "1,2K", "600K", "1M", "1,5B").
 *
 * Truncates rather than rounds, so a value never reads as a unit it has not
 * reached (999.999 is "999K", never "1000K"), and keeps one decimal only while
 * the mantissa is a single digit — which is what holds the width at four. */
export function chipsShort(amount: number): string {
  if (!Number.isFinite(amount)) return chipsExact(amount);
  const sign = amount < 0 ? '-' : '';
  const value = Math.abs(amount);
  for (const [size, suffix] of CHIP_UNITS) {
    if (value < size) continue;
    const scaled = value / size;
    // One decimal under 10 (1,2K), whole numbers above it (600K) — both four
    // characters wide at most. Truncated toward zero, never rounded up.
    const shown = scaled < 10 ? Math.trunc(scaled * 10) / 10 : Math.trunc(scaled);
    return `${sign}${shown.toLocaleString('pt-BR', {maximumFractionDigits: 1})}${suffix}`;
  }
  return chipsExact(amount);
}

// --- Real-money amount formatting -------------------------------------------
// The other half of the same job, and the only one allowed to print money.
// "Fichas" and "Dinheiro real" stopped being self-explanatory labels when the
// sandbox wording left the product, so the `R$ ` prefix is now the *only*
// thing that tells a player which wallet a number came from. That makes it a
// correctness rule, not a style one: every real-money figure in the app comes
// from here, and nothing else in `src/` may call `style: 'currency'`.

/** Real money, exact, always prefixed. Takes **integer centavos** — the unit
 * every real-money number crosses the wire in (`price_cents`, `game_balance`,
 * `entry_fee_cents`, and a real room's blinds, buy-in window and seat stacks,
 * which `walletclient` hands to ctech-wallet unconverted).
 *
 * Never abbreviated, for the same reason `chipsShort` exists and this has no
 * counterpart: rounding a balance to "R$ 1,2K" is not a display choice, it is
 * a wrong number.
 *
 * Note the separator: `Intl` emits U+00A0 (no-break space) between `R$` and
 * the digits, not a plain space. Hand-written template strings used to emit a
 * plain one, which is invisible on screen and fatal to a `getByText` or a
 * snapshot — which is most of why this function exists. Match on the digits,
 * or on ` `, never on `'R$ '` typed with a space bar. */
export function moneyExact(cents?: number | null): string {
  return ((cents ?? 0) / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
}
