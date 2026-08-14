import {useEffect, useState} from 'react';

function secondsUntil(deadlineMs: number) {
  return Math.max(0, Math.ceil((deadlineMs - Date.now()) / 1000));
}

/** Whole seconds remaining until `deadlineMs`, ticking once per second — but
 * only under reduced motion, where the perimeter ring's CSS animation is
 * frozen (globals.css) and this becomes the only surviving signal of time
 * left. Returns null when motion isn't reduced, so the ring alone is enough. */
export function useReducedMotionCountdown(deadlineMs: number): number | null {
  const [reduced] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  const [secondsLeft, setSecondsLeft] = useState(() => secondsUntil(deadlineMs));

  useEffect(() => {
    if (!reduced) return undefined;
    const interval = setInterval(() => setSecondsLeft(secondsUntil(deadlineMs)), 1000);
    return () => clearInterval(interval);
  }, [deadlineMs, reduced]);

  return reduced ? secondsLeft : null;
}
