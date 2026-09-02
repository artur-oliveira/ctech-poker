/** Stride multiplier for a "faster" bet step (ctrl+arrow). Callers scale `step`
 * by it; the hold-repeat ramp scales the same way. */
export const FAST_STEP_STRIDE = 3;

export function betShortcutAmount(
  key: string,
  current: number,
  min: number,
  max: number,
  step: number,
  halfPot?: number
) {
  if (key === 'a' || key === 'ArrowUp') return max;
  if (key === 'h') return halfPot ?? min;
  if (key === 'ArrowDown') return min;
  if (key === 'ArrowLeft' || key === 'ArrowRight') {
    const delta = step * (key === 'ArrowRight' ? 1 : -1);
    return Math.min(max, Math.max(min, current + delta));
  }
  return undefined;
}
