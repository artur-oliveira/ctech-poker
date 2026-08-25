import type {MetadataRoute} from 'next';
import {SITE_URL} from '@/lib/routeMetadata';

/** Routes a crawler can read without a session — the only ones worth a sitemap entry. */
export const INDEXABLE_ROUTES = [
  '/',
  '/poker-rules',
  '/guide',
  '/guide/basics',
  '/guide/table',
  '/guide/hands',
  '/guide/achievements',
  '/guide/store',
  '/guide/profile',
  '/guide/community'
] as const;

// Static export: this route is a build-time file, not a request handler.
export const dynamic = 'force-static';

export default function sitemap(): MetadataRoute.Sitemap {
  return INDEXABLE_ROUTES.map(route => ({
    url: new URL(route, SITE_URL).toString(),
    changeFrequency: route === '/' ? 'weekly' : 'monthly',
    priority: route === '/' ? 1 : route === '/guide' || route === '/poker-rules' ? 0.8 : 0.6
  }));
}
