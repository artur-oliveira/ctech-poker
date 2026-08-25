import type {MetadataRoute} from 'next';
import {SITE_URL} from '@/lib/routeMetadata';

/** Session-gated or player-specific routes: nothing for a crawler to read, so keep them out of the crawl budget. */
export const PRIVATE_ROUTES = [
  '/achievements',
  '/callback',
  '/hands',
  '/leaderboard',
  '/lobby',
  '/people',
  '/share',
  '/store',
  '/table',
  '/unavailable'
];

// Static export: this route is a build-time file, not a request handler.
export const dynamic = 'force-static';

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [{userAgent: '*', allow: '/', disallow: PRIVATE_ROUTES}],
    sitemap: new URL('/sitemap.xml', SITE_URL).toString()
  };
}
