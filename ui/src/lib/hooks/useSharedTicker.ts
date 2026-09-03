/** One `setInterval` for every clock on the table.
 *
 * During an active turn the table used to run four to six independent timers
 * at once — the action bar's 250 ms deadline clock, one per timed seat, the
 * reality-check sweep, the idle-removal warning, the reduced-motion countdown
 * — each with its own interval and its own React state update. That is the
 * heaviest render path in the app and measurable battery drain on a low-end
 * client.
 *
 * Subscribers declare the cadence they need; the module keeps a single
 * interval running at the shortest cadence among them and notifies each
 * subscriber only when its own period has elapsed, so timer accuracy is
 * unchanged. With no subscribers there is no interval at all. */
type Subscriber = { periodMs: number; nextAt: number; notify: () => void };

const subscribers = new Set<Subscriber>();
let timer: number | undefined;
let basePeriodMs = 0;

function tick() {
  const now = Date.now();
  for (const subscriber of subscribers) {
    if (now < subscriber.nextAt) continue;
    subscriber.nextAt = now + subscriber.periodMs;
    subscriber.notify();
  }
}

// The base cadence is the shortest period anyone asked for, so a 250 ms
// subscriber joining a table that only had a 15 s one sharpens the shared
// clock; each subscriber's own `nextAt` survives the swap, so nobody's phase
// shifts when the base changes.
function reschedule() {
  const base = subscribers.size ? Math.min(...[...subscribers].map(subscriber => subscriber.periodMs)) : 0;
  if (base === basePeriodMs) return;
  if (timer !== undefined) window.clearInterval(timer);
  basePeriodMs = base;
  timer = base ? window.setInterval(tick, base) : undefined;
}

/** Subscribes to the shared clock at `periodMs`; returns the unsubscribe. */
export function subscribeTicker(periodMs: number, notify: () => void) {
  const subscriber: Subscriber = {periodMs, nextAt: Date.now() + periodMs, notify};
  subscribers.add(subscriber);
  reschedule();
  return () => {
    subscribers.delete(subscriber);
    reschedule();
  };
}

/** How many real intervals the shared clock is running: 0 or 1, never more.
 * Exposed so the "one clock during a turn" guarantee is assertable. */
export function tickerIntervalCount() {
  return timer === undefined ? 0 : 1;
}
