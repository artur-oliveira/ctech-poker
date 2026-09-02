// Minimal client-error beacon for the OAuth callback path (Issue #86). This is
// the same `/v1.0/client-errors` sink Issue #53 will wire up globally
// (window.onerror / the error boundaries); scoping it to one call site first
// keeps this change small while giving that endpoint a real caller today.
// Best-effort only: telemetry must never throw or block the feature it watches.

export interface ClientErrorReport {
  message: string;
  stack?: string;
  route?: string;
  correlationId: string;
  context?: Record<string, string>;
}

function apiURL(path: string) {
  const base = (process.env.NEXT_PUBLIC_API_URL || '').replace(/\/$/, '');
  return `${base}${path}`;
}

function newCorrelationId() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/**
 * Fires a compact `{message, stack, route, correlationId, context}` beacon at
 * the client-error sink and returns the correlation id immediately (the
 * network call is never awaited). No PII, no app/table snapshot — matches
 * Issue #53's documented payload shape. Safe to call outside the browser or
 * when `fetch` fails: the id is still returned so the caller can surface it.
 */
export function reportClientError(message: string, options: {stack?: string; context?: Record<string, string>} = {}): string {
  const correlationId = newCorrelationId();
  if (typeof window === 'undefined' || typeof fetch !== 'function') return correlationId;
  const payload: ClientErrorReport = {
    message,
    stack: options.stack,
    route: window.location.pathname,
    correlationId,
    context: options.context,
  };
  try {
    void fetch(apiURL('/v1.0/client-errors'), {
      method: 'POST',
      credentials: 'omit',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload),
      keepalive: true,
    }).catch(() => undefined);
  } catch {
    // Synchronous throw from a mocked/broken fetch — still return the id.
  }
  return correlationId;
}
