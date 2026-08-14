import {useSyncExternalStore} from 'react';

const HOVER_QUERY = '(hover: hover) and (pointer: fine)';

function subscribe(onChange: () => void) {
  const query = window.matchMedia(HOVER_QUERY);
  query.addEventListener('change', onChange);
  return () => query.removeEventListener('change', onChange);
}

// Gates desktop-only hover affordances (e.g. peeking at your own hole cards
// on mouseenter) away from touch devices, where a tap fires mouseenter with
// no matching mouseleave and the affordance would stick shown.
export function useHoverCapable(): boolean {
  return useSyncExternalStore(subscribe, () => window.matchMedia(HOVER_QUERY).matches, () => false);
}
