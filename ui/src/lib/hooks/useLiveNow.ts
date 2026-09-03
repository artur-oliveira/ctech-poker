import {useEffect, useState} from 'react';
import {subscribeTicker} from '@/lib/hooks/useSharedTicker';

/** Wall-clock milliseconds, re-read every `intervalMs` while `active`.
 *
 * Most of the table renders off `useTableRealtime`'s `snapshotAt` — the instant
 * the last snapshot arrived — which is deliberately frozen so countdowns stay
 * pure functions of their props. Deadline *phases* can't use it: the turn clock
 * hands over to the time bank at an absolute instant no broadcast coincides
 * with, so a gate comparing against a frozen timestamp only ever flips when an
 * unrelated snapshot happens to land (or the page reloads).
 *
 * The tick itself comes from `useSharedTicker`, so every clock on the table
 * (action bar, each timed seat, the reality check, the idle warning) shares a
 * single interval instead of arming one apiece. */
export function useLiveNow(active: boolean, intervalMs = 250): number {
  const [now, setNow] = useState(() => Date.now());
  // While inactive the clock stands still, so re-activating after an idle
  // stretch would otherwise report a timestamp minutes old until the first
  // tick — long enough for a deadline gate to briefly read as already passed.
  // Re-read during render (React's "adjusting state when a prop changes"),
  // not in the effect, which would cascade an extra render every activation.
  const [activeSeen, setActiveSeen] = useState(active);
  if (active !== activeSeen) {
    setActiveSeen(active);
    if (active) setNow(() => Date.now());
  }

  useEffect(() => {
    if (!active) return undefined;
    return subscribeTicker(intervalMs, () => setNow(Date.now()));
  }, [active, intervalMs]);

  return now;
}
