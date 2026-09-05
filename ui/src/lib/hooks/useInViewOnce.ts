'use client';
import {useCallback, useRef, useState} from 'react';

/** How early a section counts as "about to be read". One viewport-ish of lead
 * time, so the deferred read lands before the section is actually legible. */
export const IN_VIEW_ROOT_MARGIN = '400px';

/**
 * One-way latch that flips the first time the observed node comes near the
 * viewport, and never back — the same shape as the render-time latches in
 * `useTableProgressiveSession`, for the surfaces that can only learn "the
 * player got here" from layout.
 *
 * Where there is no `IntersectionObserver` (SSR, jsdom) the latch starts
 * armed: a missing observer may only cost an extra request, never hide a
 * section's content behind a latch nothing can open.
 */
export function useInViewOnce(rootMargin = IN_VIEW_ROOT_MARGIN) {
  const [seen, setSeen] = useState(() => typeof IntersectionObserver === 'undefined');
  const observerRef = useRef<IntersectionObserver | null>(null);

  const ref = useCallback((node: Element | null) => {
    observerRef.current?.disconnect();
    observerRef.current = null;
    if (!node || seen) return;
    const observer = new IntersectionObserver(entries => {
      if (entries.some(entry => entry.isIntersecting)) {
        observer.disconnect();
        setSeen(true);
      }
    }, {rootMargin});
    observer.observe(node);
    observerRef.current = observer;
  }, [rootMargin, seen]);

  return [ref, seen] as const;
}
