import {decodeIdToken as sdkDecodeIdToken, OAuthClient} from '@aoctech/auth-client';
import {OAUTH_SCOPE} from './scopes';

const client = new OAuthClient({
  baseUrl: process.env.NEXT_PUBLIC_CTECH_URL || '',
  clientId: process.env.NEXT_PUBLIC_CTECH_CLIENT_ID || '',
  redirectUri: typeof window !== 'undefined' ? `${window.location.origin}/callback` : '',
  scope: OAUTH_SCOPE
});
export const decodeIdToken = sdkDecodeIdToken;

const TOKEN_REQUEST_TIMEOUT_MS = 3_000;

/**
 * @aoctech/auth-client v1.1.0's `exchangeCode`/`refresh` take no `AbortSignal`
 * (checked against the shipped `.d.ts`), so this cannot cancel the underlying
 * `fetch` itself — that needs a library change. What it *can* do, and does:
 * once the deadline fires, `controller.signal.aborted` is set and a late
 * settlement of the real request is discarded rather than resolving/rejecting
 * the caller a second time. That is the abort this module is responsible for.
 */
function withTokenDeadline<T>(request: Promise<T>): {promise: Promise<T>; controller: AbortController} {
  const controller = new AbortController();
  const promise = new Promise<T>((resolve, reject) => {
    const timeout = setTimeout(() => {
      controller.abort();
      reject(new Error('Token request timed out after 3000ms'));
    }, TOKEN_REQUEST_TIMEOUT_MS);
    request.then(
      value => {
        clearTimeout(timeout);
        if (!controller.signal.aborted) resolve(value);
      },
      error => {
        clearTimeout(timeout);
        if (!controller.signal.aborted) reject(error);
      }
    );
  });
  return {promise, controller};
}

// #342: @aoctech/auth-client already round-trips `returnTo` through its own
// sessionStorage across the IdP redirect, but it deletes that copy as soon as
// `exchangeCode` reads it — even on a failure that isn't a state mismatch. A
// manual retry from the callback's failure screens calls `startOAuthFlow()`
// again, so this repo keeps a second, never-cleared copy (same one-shot
// sessionStorage handoff pattern as `achievementRecency.ts`) purely so a
// retry can resend the same intent instead of silently falling back to
// `/lobby`. Not a new state-management provider: a plain sessionStorage key.
const RETURN_TO_KEY = 'ctech-poker:oauth-return-to';

function rememberReturnTo(returnTo: string) {
  try {
    window.sessionStorage.setItem(RETURN_TO_KEY, returnTo);
  } catch {
    // Private mode / storage disabled: retry just falls back to /lobby.
  }
}

/** The destination passed into the most recent `startOAuthFlow()` call, so a
 * retry after a failed exchange can resend the same intent. */
export function lastReturnTo(): string | null {
  try {
    return window.sessionStorage.getItem(RETURN_TO_KEY);
  } catch {
    return null;
  }
}

export async function startOAuthFlow(returnTo = '/lobby') {
  rememberReturnTo(returnTo);
  await client.startOAuthFlow(returnTo);
}

function usernameFrom(idToken?: string | null) {
  return idToken ? decodeIdToken(idToken)?.username ?? null : null;
}

/** Why a callback exchange failed, so the page can react instead of
 * collapsing every rejection into "code expired":
 * - `transient`: network blip or the 3 s deadline — the code is likely still
 *   valid; retry the same exchange.
 * - `invalid`: a PKCE `state` mismatch or a 4xx from the token endpoint (the
 *   code is expired, already used, or the IdP rejected it) — restart the
 *   whole flow.
 * - `unavailable`: the IdP itself is down (5xx) — send the player to
 *   `/unavailable` rather than looping them through a dead sign-in. */
export type OAuthFailureKind = 'transient' | 'invalid' | 'unavailable';

export class OAuthExchangeError extends Error {
  constructor(public readonly kind: OAuthFailureKind, message: string, public readonly cause?: unknown) {
    super(message);
    this.name = 'OAuthExchangeError';
  }
}

// `client.exchangeCode` throws a plain `Error` in every failure shape: a
// literal "OAuth state mismatch" string, `Token exchange failed (<status>): …`
// for a non-2xx token response, or whatever a rejected `fetch` throws (a
// `TypeError` with no status) for a network blip. This recovers the only
// signal available to classify the failure.
const IDP_STATUS_PATTERN = /Token exchange failed \((\d{3})\)/;

function classifyExchangeFailure(error: unknown, deadlineExceeded: boolean): OAuthFailureKind {
  if (deadlineExceeded) return 'transient';
  const message = error instanceof Error ? error.message : String(error);
  if (message === 'OAuth state mismatch') return 'invalid';
  const status = Number(message.match(IDP_STATUS_PATTERN)?.[1]);
  if (Number.isFinite(status)) return status >= 500 ? 'unavailable' : 'invalid';
  // An unrecognized shape (typically a rejected fetch/TypeError) is treated
  // as a transient blip rather than forcing a full re-auth on every surprise.
  return 'transient';
}

export async function exchangeCode(code: string, state: string) {
  const {promise, controller} = withTokenDeadline(client.exchangeCode(code, state));
  try {
    const r = await promise;
    return {
      accessToken: r.accessToken,
      username: usernameFrom(r.idToken),
      returnTo: r.returnTo
    };
  } catch (error) {
    const kind = classifyExchangeFailure(error, controller.signal.aborted);
    const message = error instanceof Error ? error.message : 'Token exchange failed';
    throw new OAuthExchangeError(kind, message, error);
  }
}

export async function doRefresh() {
  const {promise} = withTokenDeadline(client.refresh());
  const r = await promise;
  return r ? {accessToken: r.accessToken, username: usernameFrom(r.idToken)} : null;
}

export function endSession(returnTo = '/') {
  client.endSessionRedirect(returnTo);
}

// Logout sequence per @aoctech/auth-client's README: revoke the refresh
// token, then redirect through the IdP's RP-initiated end-session endpoint.
export async function logout(returnTo = '/') {
  const {promise} = withTokenDeadline(client.revoke());
  await promise.catch(() => undefined);
  endSession(returnTo);
}
