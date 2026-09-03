import {describe, expect, test} from 'vitest';
import robots, {PRIVATE_ROUTES} from './robots';
import sitemap, {INDEXABLE_ROUTES} from './sitemap';
import {SITE_URL} from '@/lib/routeMetadata';

describe('robots.txt', () => {
  test('lets crawlers in but keeps session-gated routes out', () => {
    const [rule] = robots().rules as {userAgent: string; allow: string; disallow: string[]}[];
    expect(rule.userAgent).toBe('*');
    expect(rule.allow).toBe('/');
    expect(rule.disallow).toEqual(PRIVATE_ROUTES);
  });

  test('points at the absolute sitemap URL', () => {
    expect(robots().sitemap).toBe(new URL('/sitemap.xml', SITE_URL).toString());
  });

  // #118: unfurl bots (WhatsApp, Slack, Discord, X) honour robots.txt, so a
  // disallowed /share is a link that never gets a preview card. It stays
  // crawlable and out of the sitemap; the noindex lives in the route's meta.
  test('leaves /share crawlable so link unfurlers can read its OG card', () => {
    expect(PRIVATE_ROUTES).not.toContain('/share');
    expect(INDEXABLE_ROUTES as readonly string[]).not.toContain('/share');
  });

  test('never disallows a route the sitemap advertises', () => {
    for (const route of INDEXABLE_ROUTES) {
      expect(PRIVATE_ROUTES.some(blocked => route === blocked || route.startsWith(`${blocked}/`))).toBe(false);
    }
  });
});

describe('sitemap.xml', () => {
  test('lists every public route as an absolute URL', () => {
    expect(sitemap().map(entry => entry.url))
      .toEqual(INDEXABLE_ROUTES.map(route => new URL(route, SITE_URL).toString()));
  });

  test('ranks the landing page above the guide, and the guide above its chapters', () => {
    const priority = Object.fromEntries(sitemap().map((entry, index) => [INDEXABLE_ROUTES[index], entry.priority]));
    expect(priority['/']).toBe(1);
    expect(priority['/guide']).toBe(0.8);
    expect(priority['/poker-rules']).toBe(0.8);
    expect(priority['/guide/basics']).toBe(0.6);
  });

  test('only the landing page claims a weekly refresh', () => {
    const [home, ...rest] = sitemap();
    expect(home.changeFrequency).toBe('weekly');
    expect(rest.every(entry => entry.changeFrequency === 'monthly')).toBe(true);
  });
});
