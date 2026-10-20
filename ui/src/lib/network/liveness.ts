import {createLiveness, type LivenessSnapshot} from '@aoctech/ui';
import {USE_MOCK} from '@/lib/mockConfig';

export const HTTP_TIMEOUT_MS = 3_000;

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

/**
 * The one probe of `/v1.0/health` for the app (api client, sockets, banner).
 * Three consecutive failed probes (the package's backoff between them) are a
 * server outage; a device that is offline never escalates. Mock mode has no API
 * to probe, so it answers OK (offline is still reported, so the banner works).
 */
export const liveness = createLiveness({
  url: `${(process.env.NEXT_PUBLIC_API_URL || '').replace(/\/$/, '')}/v1.0/health`,
  timeoutMs: HTTP_TIMEOUT_MS,
  failuresToUnavailable: 3,
  fetch: USE_MOCK ? () => Promise.resolve({ok: true} as Response) : undefined,
});

const INITIAL_SNAPSHOT: ApiLivenessSnapshot = {status: 'checking', reason: null, checkedAt: null};

let last: {source: LivenessSnapshot; derived: ApiLivenessSnapshot} | null = null;

/** The product's three-state view of the package snapshot (degraded still counts as available: the outage is only published at the threshold). Referentially stable for useSyncExternalStore. */
export function getApiLivenessSnapshot(): ApiLivenessSnapshot {
  const source = liveness.getSnapshot();
  if (source.checkedAt === null) return INITIAL_SNAPSHOT;
  if (last?.source === source) return last.derived;
  const derived: ApiLivenessSnapshot = source.status === 'offline' ? {status: 'unavailable', reason: 'offline', checkedAt: source.checkedAt}
    : source.status === 'unavailable' ? {status: 'unavailable', reason: 'server', checkedAt: source.checkedAt}
      : {status: 'available', reason: null, checkedAt: source.checkedAt};
  last = {source, derived};
  return derived;
}

export function getServerApiLivenessSnapshot() {
  return INITIAL_SNAPSHOT;
}

export const subscribeApiLiveness = liveness.subscribe;

export const checkApiLiveness = () => liveness.check();

/** Normal API calls wait for the first health result and then fail fast while
 * unavailable. This makes the health probe, not every screen query, the retry
 * loop responsible for discovering recovery. */
export async function requireApiLiveness() {
  const snapshot = getApiLivenessSnapshot();
  if (snapshot.status === 'available') return;
  if (snapshot.status === 'unavailable') throw new ApiUnavailableError(snapshot.reason || 'server');
  if (!await checkApiLiveness()) throw new ApiUnavailableError(getApiLivenessSnapshot().reason || 'server');
}

const RETURN_AFTER_OUTAGE_KEY = 'poker:return-after-outage';

/**
 * Navigates to the full-bleed `/unavailable` screen, saving the interrupted
 * route first so it can be restored once the health probe recovers. Shared by
 * the explicit-503 path (`redirectOnServiceUnavailable`), by a confirmed
 * outage from the probe and by an unrecoverable OAuth callback; no-op once
 * already there.
 */
export function navigateToUnavailable(): boolean {
  if (typeof window === 'undefined' || window.location.pathname === '/unavailable') return false;
  try {
    window.sessionStorage.setItem(RETURN_AFTER_OUTAGE_KEY, `${window.location.pathname}${window.location.search || ''}`);
  } catch {
    // Storage can be unavailable in privacy modes; /lobby remains the safe fallback.
  }
  window.location.replace('/unavailable');
  return true;
}
