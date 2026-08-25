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

function withTokenDeadline<T>(request: Promise<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('Token request timed out after 3000ms')), TOKEN_REQUEST_TIMEOUT_MS);
    request.then(resolve, reject).finally(() => clearTimeout(timeout));
  });
}

export async function startOAuthFlow(returnTo = '/lobby') {
  await client.startOAuthFlow(returnTo);
}

function usernameFrom(idToken?: string | null) {
  return idToken ? decodeIdToken(idToken)?.username ?? null : null;
}

export async function exchangeCode(code: string, state: string) {
  const r = await withTokenDeadline(client.exchangeCode(code, state));
  return {
    accessToken: r.accessToken,
    username: usernameFrom(r.idToken),
    returnTo: r.returnTo
  };
}

export async function doRefresh() {
  const r = await withTokenDeadline(client.refresh());
  return r ? {accessToken: r.accessToken, username: usernameFrom(r.idToken)} : null;
}

export function endSession(returnTo = '/') {
  client.endSessionRedirect(returnTo);
}

// Logout sequence per @aoctech/auth-client's README: revoke the refresh
// token, then redirect through the IdP's RP-initiated end-session endpoint.
export async function logout(returnTo = '/') {
  await withTokenDeadline(client.revoke()).catch(() => undefined);
  endSession(returnTo);
}
