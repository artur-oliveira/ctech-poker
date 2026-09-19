import axios from 'axios';
import {getOrRefreshSession} from '@/lib/auth/session';
import {USE_MOCK} from '@/lib/mockConfig';
import {notifyApiError} from '@/lib/notify';
import {
  checkApiLiveness,
  HTTP_TIMEOUT_MS,
  navigateToUnavailable,
  requireApiLiveness,
} from '@/lib/network/liveness';

declare module 'axios' {
  export interface AxiosRequestConfig {
    silentError?: boolean
    _retried?: boolean
    _networkRetryCount?: number
  }
}

// next_action's closed set (api-commons/problem, poker issue #319): what the
// client can DO about this error without parsing `detail` textually.
export type ApiNextAction = 'retry' | 'wait' | 'reauthenticate' | 'contact_support';

export interface ApiProblem {
  type?: string;
  title?: string;
  detail?: string;
  request_id?: string;
  // RFC 9457 extension members (#319): structured recovery guidance a
  // service sets only when it actually knows the answer (a rate limiter's
  // own window, an expired-token response). Absent means "no guidance",
  // never a guess.
  next_action?: ApiNextAction;
  retry_after_seconds?: number;
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status?: number,
    public readonly problem?: ApiProblem,
    public readonly retryAfterMs?: number,
    public readonly original?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
  }

  // Convenience accessor — same value as problem?.next_action, so a caller
  // doesn't have to null-check `problem` first to ask "should I retry,
  // wait, or send this player back to login?".
  get nextAction(): ApiNextAction | undefined {
    return this.problem?.next_action;
  }
}

// Standard envelope for every list endpoint (cursor-based DynamoDB pagination).
export interface Page<T> {
  data: T[];
  has_next: boolean;
  next_cursor: string | null;
  has_previous: boolean;
  previous_cursor: string | null;
}

let token: string | null = null;
const listeners = new Set<(v: string | null) => void>();

// Whether we've already asked getOrRefreshSession() once for the current
// "no token" streak. Reset to false whenever the token drops to null, so a
// fresh logout gets one re-check but an already-confirmed guest doesn't have
// every request re-triggering a silent refresh call.
let hasCheckedSession = false;

export function setAccessToken(v: string | null) {
  token = v;
  if (v === null) hasCheckedSession = false;
  listeners.forEach(f => f(v));
}

export function getAccessToken() {
  return token;
}

export function subscribeAccessToken(f: (v: string | null) => void) {
  listeners.add(f);
  return () => {
    listeners.delete(f);
  };
}

// The access token carries no display name (it's audience-restricted to this
// resource server); the username comes from the id_token at exchange/refresh
// time instead. Same module-singleton shape as the access token above.
let username: string | null = null;
const usernameListeners = new Set<(v: string | null) => void>();

export function setUsername(v: string | null) {
  username = v;
  usernameListeners.forEach(f => f(v));
}

export function getUsername() {
  return username;
}

export function subscribeUsername(f: (v: string | null) => void) {
  usernameListeners.add(f);
  return () => {
    usernameListeners.delete(f);
  };
}

// "Who am I" for turn/seat comparisons. The access token's `sub` can't be read
// client-side (decodeIdToken only surfaces id_token display claims), so this
// is set from GET /v1.0/players/me's `user_id`, the same value the server
// uses as seat.player_id / current_player_id.
let playerId: string | null = null;
const playerIdListeners = new Set<(v: string | null) => void>();

export function setPlayerId(v: string | null) {
  playerId = v;
  playerIdListeners.forEach(f => f(v));
}

export function getPlayerId() {
  return playerId;
}

export function subscribePlayerId(f: (v: string | null) => void) {
  playerIdListeners.add(f);
  return () => {
    playerIdListeners.delete(f);
  };
}

// A 404 (room deleted/expired) is permanent. Retrying it is pointless and
// just hammers the API. Query configs use this to skip TanStack's default
// retry for that one status while still retrying real network hiccups.
export function isNotFound(error: unknown) {
  return error instanceof ApiError ? error.status === 404 :
    axios.isAxiosError(error) && error.response?.status === 404;
}

export function normalizeApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error;
  if (!axios.isAxiosError<ApiProblem>(error)) return new ApiError('Unexpected client error', undefined, undefined, undefined, error);
  const status = error.response?.status;
  const problem = error.response?.data;
  const retryAfterMs = retryAfterMsFrom(error.response?.headers?.['retry-after'], problem?.retry_after_seconds);
  return new ApiError(problem?.detail || problem?.title || error.message || 'API request failed',
    status, problem, retryAfterMs, error);
}

// The standard HTTP `Retry-After` header wins when present (it's what a
// generic intermediary — a CDN, a load balancer — might also have set); the
// problem+json body's own `retry_after_seconds` (#319) is the fallback, since
// today only this service's own responses (the rate limiter, TooManyRequests)
// ever set it and none set both.
function retryAfterMsFrom(headerValue: unknown, bodySeconds?: number): number | undefined {
  const headerSeconds = headerValue == null ? Number.NaN : Number(headerValue);
  const seconds = Number.isFinite(headerSeconds) ? headerSeconds : bodySeconds;
  return typeof seconds === 'number' && Number.isFinite(seconds) ? Math.max(0, seconds * 1000) : undefined;
}

export function redirectOnServiceUnavailable(status?: number) {
  if (status !== 503) return false;
  return navigateToUnavailable();
}

const MAX_HTTP_RETRIES = 2;
const RETRYABLE_HTTP_STATUSES = new Set([408, 425, 429, 500, 502, 503, 504]);
const SAFE_HTTP_METHODS = new Set(['get', 'head', 'options']);

// retryAfter is the HTTP `Retry-After` header value (seconds, as a string);
// bodyRetryAfterSeconds is problem+json's own `retry_after_seconds` (#319),
// consulted only when the header is absent — same precedence as
// retryAfterMsFrom above.
export function httpRetryDelay(attempt: number, retryAfter?: string, random = Math.random,
                                bodyRetryAfterSeconds?: number) {
  const retryAfterMs = retryAfterMsFrom(retryAfter, bodyRetryAfterSeconds);
  if (retryAfterMs !== undefined) return retryAfterMs + Math.floor(random() * 250);
  const ceiling = Math.min(3_000, 250 * 2 ** Math.max(0, attempt - 1));
  return Math.floor(random() * ceiling);
}

function retryableConfig(config?: {method?: string; headers?: Record<string, unknown>; _networkRetryCount?: number}) {
  if (!config || (config._networkRetryCount || 0) >= MAX_HTTP_RETRIES) return false;
  const method = (config.method || 'get').toLowerCase();
  return SAFE_HTTP_METHODS.has(method) || Boolean(config.headers?.['Idempotency-Key'] || config.headers?.['idempotency-key']);
}

function retryableFailure(error: {response?: {status?: number}; config?: Parameters<typeof retryableConfig>[0]}) {
  if (!retryableConfig(error.config)) return false;
  const status = error.response?.status;
  return status === undefined || RETRYABLE_HTTP_STATUSES.has(status);
}

function wait(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

export const apiClient = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL || '',
  timeout: HTTP_TIMEOUT_MS,
  adapter: USE_MOCK
    ? async config => (await import('@/dev/mockRuntime')).mockAdapter(config)
    : undefined
});
apiClient.interceptors.request.use(async c => {
  // The public, dependency-free health probe owns recovery while the API is
  // down. Do not let every mounted query become its own availability probe.
  if (!USE_MOCK) await requireApiLiveness();
  // On first load, other data requests can fire before the silent session
  // refresh resolves (visible in the HAR as unauthenticated calls racing the
  // token exchange). Gate once on the outcome of that check instead of
  // letting every caller fire blind and eat a guaranteed 401 round trip.
  if (!token && !hasCheckedSession && !USE_MOCK) {
    await getOrRefreshSession().catch(() => undefined);
    hasCheckedSession = true;
  }
  if (token) c.headers.Authorization = `Bearer ${token}`;
  return c;
});
apiClient.interceptors.response.use(r => r, async e => {
  if (e?.response?.status === 401 && !e.config?._retried) {
    e.config._retried = true;
    const r = await getOrRefreshSession().catch(() => null);
    if (r) {
      setAccessToken(r.accessToken);
      setUsername(r.username);
      e.config.headers.Authorization = `Bearer ${r.accessToken}`;
      return apiClient.request(e.config);
    }
    // A server-confirmed 401 plus a failed token issue is terminal. Merely
    // clearing the in-memory token leaves the IdP cookie alive and can trap
    // the app in a refresh/401 loop, so end the complete SSO session.
    const {endExpiredSession} = await import('@/lib/auth/session');
    endExpiredSession();
  }
  // Network errors and 3 s timeouts are ambiguous in browsers. Verify them
  // against the liveness endpoint before TanStack decides whether a read is
  // safe to retry. A CORS-shaped health failure marks the API unavailable.
  let livenessAllowsRetry = e?.name !== 'ApiUnavailableError';
  if (!USE_MOCK && !e?.response && livenessAllowsRetry) livenessAllowsRetry = await checkApiLiveness();
  if (livenessAllowsRetry && retryableFailure(e)) {
    const retryCount = (e.config._networkRetryCount || 0) + 1;
    e.config._networkRetryCount = retryCount;
    const retryAfter = e.response?.headers?.['retry-after'];
    // #319: fall back to problem+json's own retry_after_seconds (e.g. the
    // rate limiter's window) when no Retry-After header was sent — the
    // automatic retry then waits the server's exact window instead of
    // guessing an exponential backoff.
    const bodyRetryAfterSeconds = (e.response?.data as ApiProblem | undefined)?.retry_after_seconds;
    await wait(httpRetryDelay(retryCount, retryAfter, undefined, bodyRetryAfterSeconds));
    return apiClient.request(e.config);
  }
  redirectOnServiceUnavailable(e?.response?.status);
  const normalized = normalizeApiError(e);
  if (e?.name !== 'ApiUnavailableError' && !e?.config?.silentError) notifyApiError(normalized);
  return Promise.reject(normalized);
});
