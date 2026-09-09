import {useState} from 'react';
import {useLiveNow} from '@/lib/hooks/useLiveNow';

/** How much of a turn is left when the numeric readout joins the perimeter
 * ring. The ring alone answers "roughly how long"; a digit answers "exactly
 * how long", and that only matters once the answer is short. Showing it for
 * the whole turn would be a number changing every second on every timed seat,
 * which is noise, not information. */
export const TURN_COUNTDOWN_SECONDS = 10;

/** Whole seconds remaining until `deadlineMs`, ticking once per second — from
 * `TURN_COUNTDOWN_SECONDS` down, or for the whole turn under reduced motion,
 * where the perimeter ring's CSS animation is frozen (renderer.css) and this
 * is the only surviving signal of time left. Returns null while the ring is
 * still enough on its own.
 *
 * The tick rides the shared table clock (`useSharedTicker`) rather than an
 * interval of its own. */
export function useTurnCountdown(deadlineMs: number): number | null {
  const [reduced] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  const now = useLiveNow(true, 1000);
  const seconds = Math.max(0, Math.ceil((deadlineMs - now) / 1000));
  return reduced || seconds <= TURN_COUNTDOWN_SECONDS ? seconds : null;
}
