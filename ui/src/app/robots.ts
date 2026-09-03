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
  '/store',
  '/table',
  '/unavailable'
];

/**
 * `/share` is deliberately NOT in PRIVATE_ROUTES (#118). A shared-hand link is
 * meant to be pasted into WhatsApp, Discord, Slack or X, and every one of those
 * unfurlers honours robots.txt — disallowing the path is exactly what made a
 * pasted link render as a bare domain with no card. The route keeps its
 * `robots: {index: false}` meta tag (see `(app)/share/layout.tsx`) and stays out
 * of the sitemap, so it is crawlable-but-not-indexable: the bot can read the OG
 * tags, search engines still won't list it.
 */

// Static export: this route is a build-time file, not a request handler.
export const dynamic = 'force-static';

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [{userAgent: '*', allow: '/', disallow: PRIVATE_ROUTES}],
    sitemap: new URL('/sitemap.xml', SITE_URL).toString()
  };
}
