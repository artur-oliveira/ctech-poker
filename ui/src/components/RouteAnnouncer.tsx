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
  const mounted = useRef(false);
  const [announcement, setAnnouncement] = useState('');

  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      return;
    }
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
