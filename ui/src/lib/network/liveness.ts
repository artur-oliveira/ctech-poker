export const HTTP_TIMEOUT_MS = 3_000;
export const HEALTHY_POLL_INTERVAL_MS = 30_000;
export const MAX_UNAVAILABLE_POLL_INTERVAL_MS = 30_000;

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
export function checkApiLiveness(): Promise<boolean> {
  if (browserIsOffline()) {
    markApiOffline();
    return Promise.resolve(false);
  }
  if (inFlightCheck) return inFlightCheck;

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), HTTP_TIMEOUT_MS);
  inFlightCheck = fetch(apiURL('/v1.0/health'), {
    method: 'GET',
    cache: 'no-store',
    credentials: 'omit',
    headers: {Accept: 'application/json'},
    signal: controller.signal,
  }).then(response => {
    const available = response.ok;
    publish({
      status: available ? 'available' : 'unavailable',
      reason: available ? null : 'server',
      checkedAt: Date.now(),
    });
    return available;
  }).catch(() => {
    publish({
      status: 'unavailable',
      reason: browserIsOffline() ? 'offline' : 'server',
      checkedAt: Date.now(),
    });
    return false;
  }).finally(() => {
    clearTimeout(timeout);
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
