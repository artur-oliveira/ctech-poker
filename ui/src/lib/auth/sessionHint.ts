/**
 * A local, non-authoritative hint that this browser has had a session.
 *
 * The problem it solves is the public landing page on a hard reload: the
 * silent refresh (`getOrRefreshSession`) is a network round trip, so for a few
 * hundred milliseconds nothing on the client knows whether the visitor is
 * signed in, and the only honest label is the logged-out one. A returning
 * player then watches "Entrar" turn into "Lobby" after the fact.
 *
 * So this stores one boolean, written where a session is established and
 * erased where it is cleared, and it is read for exactly one purpose: picking
 * the optimistic *label* of a call to action while the refresh is in flight.
 * It is never authorization, it never gates a route, and it never carries an
 * identity — the token in memory and the API are still the only truth. A
 * missing or stale hint costs at most one label correction once the refresh
 * resolves.
 *
 * Same storage discipline as `oauth.ts`'s RETURN_TO_KEY: every access is
 * wrapped, because private mode and disabled storage throw on access and must
 * not take the page down with them. localStorage rather than sessionStorage on
 * purpose — the whole point is surviving a reload and a brand new tab.
 */
const HINT_KEY = 'ctech-poker:has-session';

const listeners = new Set<() => void>();

/** Records that a session exists in this browser. */
export function rememberSessionHint() {
  try {
    window.localStorage.setItem(HINT_KEY, '1');
  } catch {
    // Private mode / storage disabled: the CTA just starts from its
    // logged-out label and corrects itself when the refresh resolves.
  }
  listeners.forEach(listener => listener());
}

/** Erases the hint. Called wherever the local identity is cleared. */
export function forgetSessionHint() {
  try {
    window.localStorage.removeItem(HINT_KEY);
  } catch {
    // Nothing to clean up when nothing could be written.
  }
  listeners.forEach(listener => listener());
}

/** Whether this browser has had a session. Label-only, never authorization. */
export function hasSessionHint() {
  try {
    return window.localStorage.getItem(HINT_KEY) === '1';
  } catch {
    return false;
  }
}

/** `useSyncExternalStore` subscription. The `storage` event is what keeps a
 * second tab honest: signing out in one tab drops the other tab's optimistic
 * label without a reload. */
export function subscribeSessionHint(listener: () => void) {
  listeners.add(listener);
  const onStorage = (event: StorageEvent) => {
    // `key === null` is a whole-storage clear.
    if (event.key === null || event.key === HINT_KEY) listener();
  };
  window.addEventListener('storage', onStorage);
  return () => {
    listeners.delete(listener);
    window.removeEventListener('storage', onStorage);
  };
}
