export function betShortcutAmount(
  key: string,
  current: number,
  min: number,
  max: number,
  step: number,
  fast = false,
  halfPot?: number
) {
  if (key === 'a' || key === 'ArrowUp') return max;
  if (key === 'h') return halfPot ?? min;
  if (key === 'ArrowDown') return min;
  if (key === 'ArrowLeft' || key === 'ArrowRight') {
    const delta = step * (fast ? 3 : 1) * (key === 'ArrowRight' ? 1 : -1);
    return Math.min(max, Math.max(min, current + delta));
  }
  return undefined;
}
