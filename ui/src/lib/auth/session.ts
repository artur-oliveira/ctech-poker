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

/** Fallback cadence, used only when the token carries no readable `exp` (the
 *  mock token, or an opaque one). Access tokens live 15 minutes. */
export const TOKEN_REFRESH_INTERVAL_MS = 4 * 60 * 1000;

/** How long before the token's own expiry the renewal is scheduled. Wide
 *  enough to absorb a slow token endpoint and a drifting client clock — the
 *  browser's `Date.now()` is not the IdP's. */
export const TOKEN_REFRESH_MARGIN_MS = 60 * 1000;

export type SessionResult = Awaited<ReturnType<typeof doRefresh>>;

/** If the RP-initiated logout redirect hasn't taken the page away within this
 * window, assume it no-op'd (IdP end-session endpoint misconfigured/unreachable,
 * popup blocked) and fall back to an interactive sign-in. */
export const EXPIRED_SESSION_REDIRECT_WATCHDOG_MS = 1_500;

let refreshPromise: Promise<SessionResult> | null = null;
// When the last refresh attempt settled. Written by every caller of
// getOrRefreshSession (the keep-alive, the 401 interceptor, the socket
// recovery), so a refresh someone else just did postpones the keep-alive
// instead of being duplicated by it.
let lastRefreshAtMs = Date.now();
// No metrics sink in this app (`lib/telemetry.ts` is the client-*error* sink),
// so this counter is what makes "refreshes per hour" assertable in tests and
// readable from the console while watching a session.
let refreshes = 0;

/** How many token refreshes this browser session has spent. */
export function sessionRefreshCount() {
  return refreshes;
}

/** Test-only: resets the refresh counter and the last-attempt clock. */
export function resetSessionRefreshCountForTests(nowMs = Date.now()) {
  refreshes = 0;
  lastRefreshAtMs = nowMs;
}

/** The access token's `exp`, in epoch milliseconds, or `undefined` when it is
 * not a JWT we can read (the mock token) or carries no numeric `exp`.
 *
 * Reading a claim is not trusting it: `exp` is used only to decide *when* to
 * ask for a new token. Authorization is the API's job, and a token whose `exp`
 * lies simply expires into the 401 path that already exists. */
export function tokenExpiryMs(token: string | null): number | undefined {
  const payload = token?.split('.')[1];
  if (!payload) return undefined;
  try {
    const claims = JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/')));
    return typeof claims?.exp === 'number' ? claims.exp * 1000 : undefined;
  } catch {
    return undefined;
  }
}

/** Delay until this token should be renewed: its own expiry minus the margin,
 * or — for a token with no readable `exp` — the fixed cadence measured from
 * the last refresh attempt. `0` means "renew now". */
export function nextRefreshDelayMs(token: string | null, nowMs = Date.now()) {
  const expiry = tokenExpiryMs(token);
  if (expiry !== undefined) return Math.max(0, expiry - TOKEN_REFRESH_MARGIN_MS - nowMs);
  return Math.max(0, lastRefreshAtMs + TOKEN_REFRESH_INTERVAL_MS - nowMs);
}
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
    refreshes += 1;
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
        lastRefreshAtMs = Date.now();
        refreshPromise = null;
      });
  }
  return refreshPromise;
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
 *
 * It renews by **expiry**, not on a fixed cadence, and it does not run at all
 * while the tab is hidden (#231). A 4-minute interval against a 15-minute
 * token spent up to 15 refreshes an hour on a tab nobody was looking at; a
 * timer armed at `exp - margin` spends at most one per token, and a hidden tab
 * spends none. Coming back is one read *only if* the token is due — the whole
 * point of pausing is not to pay for the pause on the way back.
 *
 * Concurrent attempts inside a tab coalesce in `getOrRefreshSession`; two open
 * tabs still refresh independently, each holding its own in-memory token.
 */
export function useSessionKeepAlive() {
  useEffect(() => {
    if (USE_MOCK) return () => {
    };
    let timer: ReturnType<typeof setTimeout> | undefined;
    const arm = () => {
      clearTimeout(timer);
      // Suspended in the background: nothing is on screen to go stale, and
      // `onVisible` below is what covers the way back.
      if (document.visibilityState === 'hidden') return;
      timer = setTimeout(renew, nextRefreshDelayMs(getAccessToken()));
    };
    function renew() {
      // Re-arm on the outcome, not on the token change alone: a refresh that
      // failed on a network blip leaves the token untouched, and without this
      // the keep-alive would stop there.
      void getOrRefreshSession().catch(() => undefined).finally(arm);
    }
    const renewIfDue = () => {
      if (nextRefreshDelayMs(getAccessToken()) <= 0) renew(); else arm();
    };
    // A landed refresh (from here, from the 401 interceptor, or from the
    // socket recovery) means a new expiry to schedule against.
    const unsubscribe = subscribeAccessToken(arm);
    // A laptop that slept past the token's life comes back with nothing armed,
    // so returning to the tab — or reconnecting — is where an already-expired
    // token is renewed, and where a still-valid one just re-arms its timer.
    const onVisible = () => {
      if (document.visibilityState !== 'visible') {
        clearTimeout(timer);
        return;
      }
      renewIfDue();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('online', renewIfDue);
    arm();
    return () => {
      clearTimeout(timer);
      unsubscribe();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('online', renewIfDue);
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
