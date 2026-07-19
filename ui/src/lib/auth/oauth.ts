import {decodeIdToken as sdkDecodeIdToken, OAuthClient} from '@aoctech/auth-client'

const client = new OAuthClient({
  baseUrl: process.env.NEXT_PUBLIC_CTECH_URL || '',
  clientId: process.env.NEXT_PUBLIC_CTECH_CLIENT_ID || '',
  redirectUri: typeof window !== 'undefined' ? `${window.location.origin}/callback` : '',
  scope: 'openid profile'
})
export const decodeIdToken = sdkDecodeIdToken

export async function startOAuthFlow(returnTo = '/lobby') {
  await client.startOAuthFlow(returnTo)
}

export async function exchangeCode(code: string, state: string) {
  const r = await client.exchangeCode(code, state);
  return {accessToken: r.accessToken, returnTo: r.returnTo}
}

export async function doRefresh() {
  const r = await client.refresh();
  return r ? {accessToken: r.accessToken} : null
}
