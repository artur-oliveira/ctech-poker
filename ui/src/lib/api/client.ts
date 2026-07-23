import axios from 'axios';
import {doRefresh} from '@/lib/auth/oauth';
import {mockAdapter, USE_MOCK} from '@/lib/mock';
import {notifyApiError} from '@/lib/notify';

declare module 'axios' {
  export interface AxiosRequestConfig {
    silentError?: boolean
  }
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
// is set from GET /v1.0/players/me's `user_id` — the same value the server
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

// A 404 (room deleted/expired) is permanent — retrying it is pointless and
// just hammers the API. Query configs use this to skip TanStack's default
// retry for that one status while still retrying real network hiccups.
export function isNotFound(error: unknown) {
  return axios.isAxiosError(error) && error.response?.status === 404;
}

export const apiClient = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL || '',
  adapter: USE_MOCK ? mockAdapter : undefined
});
apiClient.interceptors.request.use(c => {
  if (token) c.headers.Authorization = `Bearer ${token}`;
  return c;
});
apiClient.interceptors.response.use(r => r, async e => {
  if (e.response?.status === 401 && !e.config._retried) {
    e.config._retried = true;
    const r = await doRefresh();
    if (r) {
      setAccessToken(r.accessToken);
      setUsername(r.username);
      e.config.headers.Authorization = `Bearer ${r.accessToken}`;
      return apiClient.request(e.config);
    }
  }
  if (!e.config?.silentError) notifyApiError(e);
  return Promise.reject(e);
});
