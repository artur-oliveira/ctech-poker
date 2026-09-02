'use client';
import {useCallback, useEffect, useRef} from 'react';

export const HOLD_DELAY_MS = 420;
export const HOLD_REPEAT_MS = 130;

/** Stride applied once a hold outlives HOLD_DELAY_MS: the first ticks stay
 * fine-grained so a short hold still lands on an exact amount, then coarsen so
 * a long one crosses a deep stack without the player waiting on it. */
function holdStride(tick: number) {
  return tick < 5 ? 1 : tick < 11 ? 5 : 10;
}

/** Press-and-hold repetition shared by the bet stepper's pointer buttons and
 * its arrow-key shortcuts, so touch and keyboard accelerate identically. The
 * caller passes the step for this press to `start`, which keeps the direction
 * (and any modifier stride) out of the hook. */
export function useHoldRepeat() {
  const stepRef = useRef<(stride: number) => void>(() => undefined);
  const delayRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const repeatRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const repeatedRef = useRef(false);
  const ticksRef = useRef(0);

  const stop = useCallback(() => {
    if (delayRef.current) clearTimeout(delayRef.current);
    if (repeatRef.current) clearInterval(repeatRef.current);
    delayRef.current = null;
    repeatRef.current = null;
  }, []);

  const start = useCallback((step: (stride: number) => void) => {
    // A key held down still emits OS auto-repeat keydowns; ignore them so the
    // cadence is ours and not the operating system's.
    if (delayRef.current || repeatRef.current) return;
    stepRef.current = step;
    repeatedRef.current = false;
    ticksRef.current = 0;
    delayRef.current = setTimeout(() => {
      repeatedRef.current = true;
      stepRef.current(1);
      repeatRef.current = setInterval(() => {
        ticksRef.current += 1;
        stepRef.current(holdStride(ticksRef.current));
      }, HOLD_REPEAT_MS);
    }, HOLD_DELAY_MS);
  }, []);

  /** True when the press that just ended had already repeated, so the caller
   * can swallow the trailing click. Reading it consumes the flag. */
  const consumeRepeated = useCallback(() => {
    const repeated = repeatedRef.current;
    repeatedRef.current = false;
    return repeated;
  }, []);

  useEffect(() => stop, [stop]);

  return {start, stop, consumeRepeated};
}
