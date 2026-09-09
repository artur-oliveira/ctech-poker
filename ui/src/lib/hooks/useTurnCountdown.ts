import {useState} from 'react';
import {useLiveNow} from '@/lib/hooks/useLiveNow';

/** Whole seconds remaining until `deadlineMs`, ticking once per second — but
 * only under reduced motion, where the perimeter ring's CSS animation is
 * frozen (globals.css) and this becomes the only surviving signal of time
 * left. Returns null when motion isn't reduced, so the ring alone is enough.
 *
 * The second-by-second tick rides the shared table clock (`useSharedTicker`)
 * rather than an interval of its own. */
export function useReducedMotionCountdown(deadlineMs: number): number | null {
  const [reduced] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  const now = useLiveNow(reduced, 1000);
  return reduced ? Math.max(0, Math.ceil((deadlineMs - now) / 1000)) : null;
}
