// The client-error beacon. `reportClientError` is the single sink every
// reporter goes through — the OAuth callback path (Issue #86), the two error
// boundaries and the global `error`/`unhandledrejection` listeners installed by
// `installGlobalErrorReporter` (Issue #53). It POSTs to `/v1.0/client-errors`.
//
// Best-effort only: telemetry must never throw or block the feature it watches,
// so every path swallows its own failures and nothing is awaited.
//
// Deliberately NOT in the payload: player id, display name, token, table id,
// any snapshot, and the query string (a table link carries a room id). Route is
// `location.pathname` only.

export interface ClientErrorReport {
  message: string;
  stack?: string;
  route?: string;
  release: string;
  correlationId: string;
  context?: Record<string, string>;
}

/** Beacons per page load. A render loop can trip a boundary hundreds of times
 * a second; the first few reports carry all the information the later ones do. */
const MAX_REPORTS_PER_SESSION = 10;
/** A repeated message inside this window is the same fault, not a new one. */
const DEDUPE_MS = 10_000;
/** Enough frames to place the fault, short enough to stay a log line. */
const STACK_CHARS = 2_000;

const RELEASE = process.env.NEXT_PUBLIC_RELEASE || 'dev';

let sent = 0;
const recent = new Map<string, number>();

/** Test seam: a reporter that has spent its budget would silently swallow the
 * next suite's assertions. */
export function resetClientErrorBudgetForTests() {
  sent = 0;
  recent.clear();
  installed = false;
}

function withinBudget(message: string) {
  if (sent >= MAX_REPORTS_PER_SESSION) return false;
  const now = Date.now();
  const last = recent.get(message);
  if (last !== undefined && now - last < DEDUPE_MS) return false;
  recent.set(message, now);
  sent += 1;
  return true;
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
  if (!withinBudget(message)) return correlationId;
  const payload: ClientErrorReport = {
    message,
    stack: options.stack?.slice(0, STACK_CHARS),
    route: window.location.pathname,
    release: RELEASE,
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

let installed = false;

/**
 * Routes uncaught errors and unhandled promise rejections to the same sink the
 * boundaries use. Idempotent, and a no-op outside the browser (static export:
 * this module is also imported during the build). Listeners are never removed —
 * the reporter should outlive every component on the page.
 */
export function installGlobalErrorReporter() {
  if (installed || typeof window === 'undefined') return;
  installed = true;
  window.addEventListener('error', event => {
    // A failed <img>/<script> load also fires `error` on the element and
    // bubbles here with no `error` object; that is not a crash.
    if (!(event.error instanceof Error) && !event.message) return;
    reportClientError(event.error instanceof Error ? event.error.message : event.message, {
      stack: event.error instanceof Error ? event.error.stack : undefined,
      context: {kind: 'uncaught'},
    });
  });
  window.addEventListener('unhandledrejection', event => {
    const reason: unknown = event.reason;
    reportClientError(reason instanceof Error ? reason.message : String(reason), {
      stack: reason instanceof Error ? reason.stack : undefined,
      context: {kind: 'unhandledrejection'},
    });
  });
}

/** What the two error boundaries report: the boundary that caught it, plus
 * Next's build-stable `digest` so a minified message can still be grouped. */
export function reportBoundaryError(error: Error & {digest?: string}, boundary: 'route' | 'global') {
  return reportClientError(error.message || 'Boundary error', {
    stack: error.stack,
    context: {kind: 'boundary', boundary, ...(error.digest ? {digest: error.digest} : {})},
  });
}
