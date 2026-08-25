'use client';
import {useEffect, useRef, useState} from 'react';
import {usePathname} from 'next/navigation';

/** SPA route changes never trigger a browser navigation, so screen readers
 * stay silent and keyboard focus stays on a link/button that may no longer
 * make sense (or vanished). On every path change after the first, move focus
 * to the new page's heading and announce its title, mirroring what a full
 * page load would do. */
export function RouteAnnouncer() {
  const pathname = usePathname();
  // The path this effect last acted on, not a "have I mounted" boolean: under
  // StrictMode the mount effect runs twice, and the second pass used to fall
  // through and mutate `main`/`h1` while a Suspense boundary below was still
  // hydrating — which React reports as a hydration mismatch on /profile and
  // /share. Comparing the path makes the re-run a no-op, so the DOM is only
  // ever touched on a real route change, after hydration.
  const lastPath = useRef<string | null>(null);
  const [announcement, setAnnouncement] = useState('');

  useEffect(() => {
    if (lastPath.current === pathname) return;
    const first = lastPath.current === null;
    lastPath.current = pathname;
    if (first) return;
    const heading = document.querySelector<HTMLElement>('main h1, h1');
    const target = heading ?? document.querySelector<HTMLElement>('main');
    if (target) {
      if (!target.hasAttribute('tabindex')) target.setAttribute('tabindex', '-1');
      target.focus();
    }
    setAnnouncement(document.title);
  }, [pathname]);

  return <div className="sr-only" role="status" aria-live="polite">{announcement}</div>;
}
