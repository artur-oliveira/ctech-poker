import {useEffect, useRef} from 'react';

/** A single short buzz — long enough to feel, too short to read as an alarm.
 * The table already owns the player's eyes; this is only for the moment they
 * are not on it. */
export const TURN_VIBRATION_MS = 20;

/** Buzzes once when the viewer's own turn opens.
 *
 * Once per turn, not once per render: a turn is the contiguous stretch
 * `isTurn` stays true, so the latch clears only when it goes false again — a
 * resync, a re-render or a new snapshot mid-turn cannot re-fire it. Never for
 * anyone else's turn (the caller only ever passes the viewer's own), never in
 * a hidden tab (the latch still arms there, so returning to the tab does not
 * buzz for a turn that opened while it was away), and never where the API is
 * missing — most desktop browsers have no Vibration API at all. */
export function useTurnHaptic(isTurn: boolean) {
  const buzzed = useRef(false);
  useEffect(() => {
    if (!isTurn) {
      buzzed.current = false;
      return;
    }
    if (buzzed.current) return;
    buzzed.current = true;
    if (document.visibilityState === 'hidden') return;
    if (typeof navigator.vibrate !== 'function') return;
    navigator.vibrate(TURN_VIBRATION_MS);
  }, [isTurn]);
}
