// Unlike TermsGate, this page never forces a login redirect. It has a real
// public variant. It only checks whether a session already exists (silent
// refresh, same call TermsGate makes) so a returning player sees their own
// progress without a hard gate blocking a first-time or logged-out visitor.

import {useEffect, useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {getAccessToken, setAccessToken, setPlayerId, setUsername, subscribeAccessToken} from "@/lib/api/client";
import {USE_MOCK} from "@/lib/mockConfig";
import {doRefresh} from "@/lib/auth/oauth";
import {getMe} from "@/lib/api/player";

/** Access tokens live 15 minutes; refresh well inside that window. */
export const TOKEN_REFRESH_INTERVAL_MS = 4 * 60 * 1000;

export type SessionResult = Awaited<ReturnType<typeof doRefresh>>;

let refreshPromise: Promise<SessionResult> | null = null;
const expiredListeners = new Set<() => void>();

function clearSession() {
  setAccessToken(null);
  setUsername(null);
  setPlayerId(null);
  expiredListeners.forEach(listener => listener());
}

export function subscribeSessionExpired(listener: () => void) {
  expiredListeners.add(listener);
  return () => expiredListeners.delete(listener);
}

/**
 * Silent OAuth refresh shared by every caller, so two of them can never race
 * the same refresh token. `clearOnNetworkError` is the only difference between
 * them: the periodic keep-alive treats a thrown request as "device is offline
 * for a moment" and keeps the session, while a server that actively answered
 * `unauthorized` has already told us the credentials are dead. A refresh that
 * resolves with no result means the session is over either way.
 */
export function getOrRefreshSession(): Promise<SessionResult> {
  if (USE_MOCK) return Promise.resolve(null);
  if (!refreshPromise) {
    refreshPromise = doRefresh()
      .then(result => {
      if (result) {
        setAccessToken(result.accessToken);
        setUsername(result.username);
      } else {
        clearSession();
      }
        return result;
      })
      // A network failure is not proof that the refresh token was revoked.
      // Preserve the in-memory identity and let callers render an offline state.
      .finally(() => {
        refreshPromise = null;
      });
  }
  return refreshPromise;
}

export function refreshSession() {
  void getOrRefreshSession().catch(() => undefined);
}

/**
 * Called when a WebSocket is answered with `unauthorized`. Without it the
 * socket reconnects with the same dead token forever: the upgrade itself
 * succeeds, so ws-client resets its backoff on every open, and the server
 * closes right after reading the auth frame — prod logged 325 such round
 * trips in ten minutes off a token that had expired.
 */
export function recoverSession() {
  refreshSession();
}

/**
 * Keeps the access token alive for as long as the app is open. This used to
 * live inside useTableRealtime, which meant it only ran while a table was
 * mounted: sitting in the lobby for 15 minutes expired the token with nothing
 * left to renew it.
 */
export function useSessionKeepAlive() {
  useEffect(() => {
    if (USE_MOCK) return () => {
    };
    const interval = setInterval(() => refreshSession(), TOKEN_REFRESH_INTERVAL_MS);
    return () => clearInterval(interval);
  }, []);
}

export function useOptionalSession() {
  const [token, setToken] = useState<string | null>(() => getAccessToken());
  const [checking, setChecking] = useState(() => !USE_MOCK && !getAccessToken());
  
  useEffect(() => {
    const unsubscribe = subscribeAccessToken(setToken);
    if (!USE_MOCK && !getAccessToken()) {
      void getOrRefreshSession().catch(() => undefined).finally(() => setChecking(false));
    }
    return unsubscribe;
  }, []);
  
  // Pages using this hook (leaderboard, guide, etc.) never mount TermsGate,
  // so nothing else here ever calls setPlayerId. Without this, getViewerId()
  // silently returns undefined for a real session and viewer highlighting
  // breaks, unless the player happened to already visit a TermsGate page first.
  const me = useQuery({queryKey: ['player', 'me'], queryFn: getMe, enabled: Boolean(token)});
  useEffect(() => {
    setPlayerId(me.data?.user_id ?? null);
  }, [me.data?.user_id]);
  
  return {authed: Boolean(token), checking};
}
