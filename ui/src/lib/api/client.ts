import axios from 'axios';
import {getOrRefreshSession} from '@/lib/auth/session';
import {USE_MOCK} from '@/lib/mockConfig';
import {notifyApiError} from '@/lib/notify';

declare module 'axios' {
  export interface AxiosRequestConfig {
    silentError?: boolean
    _retried?: boolean
  }
}

export interface ApiProblem {
  type?: string;
  title?: string;
  detail?: string;
  request_id?: string;
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

export function setAccessToken(v: string | null) {
  token = v;
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
  const retryAfter = error.response?.headers?.['retry-after'];
  const seconds = retryAfter == null ? Number.NaN : Number(retryAfter);
  const retryAfterMs = Number.isFinite(seconds) ? Math.max(0, seconds * 1000) : undefined;
  return new ApiError(problem?.detail || problem?.title || error.message || 'API request failed',
    status, problem, retryAfterMs, error);
}

export const apiClient = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL || '',
  timeout: 10_000,
  adapter: USE_MOCK
    ? async config => (await import('@/dev/mockRuntime')).mockAdapter(config)
    : undefined
});
apiClient.interceptors.request.use(c => {
  if (token) c.headers.Authorization = `Bearer ${token}`;
  return c;
});
apiClient.interceptors.response.use(r => r, async e => {
  if (e.response?.status === 401 && !e.config._retried) {
    e.config._retried = true;
    const r = await getOrRefreshSession();
    if (r) {
      setAccessToken(r.accessToken);
      setUsername(r.username);
      e.config.headers.Authorization = `Bearer ${r.accessToken}`;
      return apiClient.request(e.config);
    }
  }
  const normalized = normalizeApiError(e);
  if (!e.config?.silentError) notifyApiError(normalized);
  return Promise.reject(normalized);
});
