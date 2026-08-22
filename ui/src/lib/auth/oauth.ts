import {decodeIdToken as sdkDecodeIdToken, OAuthClient} from '@aoctech/auth-client';
import {OAUTH_SCOPE} from './scopes';

const client = new OAuthClient({
  baseUrl: process.env.NEXT_PUBLIC_CTECH_URL || '',
  clientId: process.env.NEXT_PUBLIC_CTECH_CLIENT_ID || '',
  redirectUri: typeof window !== 'undefined' ? `${window.location.origin}/callback` : '',
  scope: OAUTH_SCOPE
});
export const decodeIdToken = sdkDecodeIdToken;

export async function startOAuthFlow(returnTo = '/lobby') {
  await client.startOAuthFlow(returnTo);
}

function usernameFrom(idToken?: string | null) {
  return idToken ? decodeIdToken(idToken)?.username ?? null : null;
}

export async function exchangeCode(code: string, state: string) {
  const r = await client.exchangeCode(code, state);
  console.log(`current code ${code} and state ${state}`);
  return {
    accessToken: r.accessToken,
    username: usernameFrom(r.idToken),
    returnTo: r.returnTo
  };
}

export async function doRefresh() {
  const r = await client.refresh();
  return r ? {accessToken: r.accessToken, username: usernameFrom(r.idToken)} : null;
}

// Logout sequence per @aoctech/auth-client's README: revoke the refresh
// token, then redirect through the IdP's RP-initiated end-session endpoint.
export async function logout(returnTo = '/') {
  await client.revoke();
  client.endSessionRedirect(returnTo);
}
