import {useEffect, useState} from 'react';

const DEFAULT_MS = 700;

/** Ticks a displayed number from `from` to `to` — up or down, whichever the
 * sign of the delta calls for. No-ops (jumps straight to `to`) when the two
 * already match, or under reduced motion. */
export function useCountUp(from: number, to: number, durationMs = DEFAULT_MS): number {
  const [reduced] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  const [display, setDisplay] = useState(() => (reduced ? to : from));

  useEffect(() => {
    if (from === to || reduced) return () => {
    };
    setDisplay(from);
    const start = performance.now();
    let raf = requestAnimationFrame(function tick(now) {
      const t = Math.min(1, (now - start) / durationMs);
      const eased = 1 - (1 - t) ** 4;
      setDisplay(Math.round(from + (to - from) * eased));
      if (t < 1) raf = requestAnimationFrame(tick);
    });
    return () => cancelAnimationFrame(raf);
  }, [from, to, durationMs, reduced]);

  return display;
}
