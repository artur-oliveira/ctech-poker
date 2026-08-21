/**
 * Origin for both WebSocket gateways. NEXT_PUBLIC_WS_URL is read rather than
 * derived from NEXT_PUBLIC_API_URL so it appears literally in the build
 * environment: the deploy workflow builds the CSP's connect-src from the
 * origins it finds there, and connect-src is scheme-exact — https://poker-api
 * does not permit wss://poker-api. Deriving it here would have produced a
 * policy that blocks every socket. The derivation stays as the fallback for
 * local work, where the dev server proxies /v1.0 and the page origin is right.
 */
export function wsOrigin(): string {
  if (process.env.NEXT_PUBLIC_WS_URL) return process.env.NEXT_PUBLIC_WS_URL;
  const http = process.env.NEXT_PUBLIC_API_URL
    || (typeof window !== 'undefined' ? window.location.origin : '');
  return http.replace(/^http/, 'ws');
}
