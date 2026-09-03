'use client';

// Unlike TermsGate, this page never forces a login redirect. It has a real
// public variant. It only checks whether a session already exists (silent
// refresh, same call TermsGate makes) so a returning player sees their own
// progress without a hard gate blocking a first-time or logged-out visitor.

import {useEffect, useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {getAccessToken, setAccessToken, setPlayerId, setUsername, subscribeAccessToken} from "@/lib/api/client";
import {MOCK_PLAYER_ID, USE_MOCK} from "@/lib/mockConfig";
import {doRefresh, endSession, startOAuthFlow} from "@/lib/auth/oauth";
import {getMe} from "@/lib/api/player";

/** Access tokens live 15 minutes; refresh well inside that window. */
export const TOKEN_REFRESH_INTERVAL_MS = 4 * 60 * 1000;

export type SessionResult = Awaited<ReturnType<typeof doRefresh>>;

/** If the RP-initiated logout redirect hasn't taken the page away within this
 * window, assume it no-op'd (IdP end-session endpoint misconfigured/unreachable,
 * popup blocked) and fall back to an interactive sign-in. */
export const EXPIRED_SESSION_REDIRECT_WATCHDOG_MS = 1_500;

let refreshPromise: Promise<SessionResult> | null = null;
let endingExpiredSession = false;
const expiredListeners = new Set<() => void>();

function clearSession() {
  setAccessToken(null);
  setUsername(null);
  setPlayerId(null);
  expiredListeners.forEach(listener => listener());
}

/** Ends both the local identity and the IdP SSO session after a credentialed
 * channel was explicitly rejected and no replacement token could be issued. */
export function endExpiredSession() {
  // The in-memory identity is cleared on every call, even a deduped one — a
  // second caller must never keep a dead token alive.
  clearSession();
  if (endingExpiredSession || typeof window === 'undefined') return;
  endingExpiredSession = true;
  endSession('/');
  // Watchdog: if endSession() didn't navigate away, the app would be left
  // tokenless with the latch stuck true and no route back to an interactive
  // login. Reset the latch and start a fresh OAuth flow instead.
  window.setTimeout(() => {
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return;
    endingExpiredSession = false;
    void startOAuthFlow('/');
  }, EXPIRED_SESSION_REDIRECT_WATCHDOG_MS);
}

/** Test-only: clears the redirect latch between cases. */
export function resetExpiredSessionLatchForTests() {
  endingExpiredSession = false;
}

export function subscribeSessionExpired(listener: () => void) {
  expiredListeners.add(listener);
  return () => expiredListeners.delete(listener);
}

/**
 * Silent OAuth refresh shared by every caller, so two of them can never race
 * the same refresh token. A thrown request/deadline preserves the in-memory
 * identity for a later keep-alive attempt; a refresh that resolves with no
 * result clears it. Callers reacting to an explicit `unauthorized` additionally
 * end the IdP session through endExpiredSession().
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
  void getOrRefreshSession()
    .then(result => {
      if (!result) endExpiredSession();
    })
    // A transport/5xx failure says nothing about the refresh credential.
    // Keep the identity and let the keep-alive/online hooks try again.
    .catch(() => undefined);
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
    // The interval alone can't cover a laptop that slept past the token's
    // 15-minute life: refresh as soon as the tab comes back or the network
    // reconnects, so the first action after a wake isn't a guaranteed 401.
    const onVisible = () => {
      if (document.visibilityState === 'visible') refreshSession();
    };
    const onOnline = () => refreshSession();
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('online', onOnline);
    return () => {
      clearInterval(interval);
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('online', onOnline);
    };
  }, []);
}

export function useOptionalSession() {
  const [token, setToken] = useState<string | null>(() => USE_MOCK ? MOCK_PLAYER_ID : getAccessToken());
  const [checking, setChecking] = useState(() => !USE_MOCK && !getAccessToken());
  
  useEffect(() => {
    const unsubscribe = subscribeAccessToken(setToken);
    if (USE_MOCK) {
      setAccessToken(MOCK_PLAYER_ID);
    } else if (!getAccessToken()) {
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
