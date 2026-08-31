export const HTTP_TIMEOUT_MS = 3_000;
export const HEALTHY_POLL_INTERVAL_MS = 30_000;
export const MAX_UNAVAILABLE_POLL_INTERVAL_MS = 30_000;
/** A single timed-out or failed /health probe is not proof the API is down —
 * it retries this many times (with a short backoff) before the outage is
 * published. Only once every attempt is exhausted does the app go offline. */
export const HEALTH_PROBE_ATTEMPTS = 3;
export const HEALTH_PROBE_RETRY_MS = 600;

export type ApiLivenessStatus = 'checking' | 'available' | 'unavailable';
export type ApiUnavailableReason = 'offline' | 'server' | null;

export interface ApiLivenessSnapshot {
  status: ApiLivenessStatus;
  reason: ApiUnavailableReason;
  checkedAt: number | null;
}

export class ApiUnavailableError extends Error {
  constructor(public readonly reason: Exclude<ApiUnavailableReason, null>) {
    super(reason === 'offline' ? 'Device is offline' : 'Poker API is unavailable');
    this.name = 'ApiUnavailableError';
  }
}

const INITIAL_SNAPSHOT: ApiLivenessSnapshot = {status: 'checking', reason: null, checkedAt: null};

let snapshot = INITIAL_SNAPSHOT;
let inFlightCheck: Promise<boolean> | null = null;
const listeners = new Set<() => void>();

function apiURL(path: string) {
  const base = (process.env.NEXT_PUBLIC_API_URL || '').replace(/\/$/, '');
  return `${base}${path}`;
}

function publish(next: ApiLivenessSnapshot) {
  if (snapshot.status === next.status && snapshot.reason === next.reason && snapshot.checkedAt === next.checkedAt) return;
  snapshot = next;
  listeners.forEach(listener => listener());
}

function browserIsOffline() {
  return typeof navigator !== 'undefined' && !navigator.onLine;
}

export function getApiLivenessSnapshot() {
  return snapshot;
}

export function getServerApiLivenessSnapshot() {
  return INITIAL_SNAPSHOT;
}

export function subscribeApiLiveness(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function markApiOffline() {
  publish({status: 'unavailable', reason: 'offline', checkedAt: Date.now()});
}

/**
 * The dependency-free liveness endpoint is the only request allowed while the
 * Poker API is unavailable. A fetch rejection is intentionally equivalent to
 * a failed response: a dead HAProxy can omit CORS headers, so browsers expose
 * that outage as a TypeError rather than an HTTP status.
 */
function probeHealthOnce(): Promise<boolean> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), HTTP_TIMEOUT_MS);
  return fetch(apiURL('/v1.0/health'), {
    method: 'GET',
    cache: 'no-store',
    credentials: 'omit',
    headers: {Accept: 'application/json'},
    signal: controller.signal,
  }).then(response => response.ok).catch(() => false).finally(() => clearTimeout(timeout));
}

const wait = (ms: number) => new Promise<void>(resolve => setTimeout(resolve, ms));

export function checkApiLiveness(): Promise<boolean> {
  if (browserIsOffline()) {
    markApiOffline();
    return Promise.resolve(false);
  }
  if (inFlightCheck) return inFlightCheck;

  inFlightCheck = (async () => {
    // A dead HAProxy times out or omits CORS headers, so both a rejected
    // fetch and a non-OK response mean "could not confirm healthy". Retry a
    // few times before concluding the API is actually down — a single slow
    // response must not black out the whole app.
    for (let attempt = 1; attempt <= HEALTH_PROBE_ATTEMPTS; attempt++) {
      if (browserIsOffline()) {
        markApiOffline();
        return false;
      }
      if (await probeHealthOnce()) {
        publish({status: 'available', reason: null, checkedAt: Date.now()});
        return true;
      }
      if (attempt < HEALTH_PROBE_ATTEMPTS) await wait(HEALTH_PROBE_RETRY_MS);
    }
    // Every attempt failed while the browser still believed it was online:
    // treat it as a server-side outage (a genuine offline drop is caught at
    // the top of the loop and reported as such).
    publish({status: 'unavailable', reason: 'server', checkedAt: Date.now()});
    return false;
  })().finally(() => {
    inFlightCheck = null;
  });
  return inFlightCheck;
}

/** Normal API calls wait for the first health result and then fail fast while
 * unavailable. This makes the health probe—not every screen query—the retry
 * loop responsible for discovering recovery. */
export async function requireApiLiveness() {
  if (snapshot.status === 'available') return;
  if (snapshot.status === 'unavailable') {
    throw new ApiUnavailableError(snapshot.reason || 'server');
  }
  if (!await checkApiLiveness()) throw new ApiUnavailableError(snapshot.reason || 'server');
}

export function livenessPollDelay(failureCount: number, random = Math.random) {
  if (failureCount <= 0) return HEALTHY_POLL_INTERVAL_MS;
  const ceiling = Math.min(MAX_UNAVAILABLE_POLL_INTERVAL_MS, 1_000 * 2 ** (failureCount - 1));
  // Equal jitter avoids both a near-zero busy loop and synchronized clients.
  return Math.floor(ceiling / 2 + random() * ceiling / 2);
}

/** Test-only reset kept explicit so runtime code cannot accidentally hide an outage. */
export function resetApiLivenessForTests() {
  snapshot = INITIAL_SNAPSHOT;
  inFlightCheck = null;
  listeners.clear();
}
