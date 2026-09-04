import {defineConfig, devices} from '@playwright/test';

/** Browser-level regression suite.
 *
 * The vitest suite runs in jsdom, which has no layout engine: it cannot see a
 * badge clipped by an ancestor's `overflow: hidden`, a stage that overflows the
 * viewport, or an event ordering that differs between engines. Those are the
 * defects that keep reaching players despite 90% line coverage, so they get a
 * gate that runs in real engines instead.
 *
 * WebKit here is Safari's engine, not iOS Safari: it does not reproduce mobile
 * Safari's dynamic viewport, its rasterisation scale, or its flex intrinsic
 * sizing quirks. Treat a green WebKit run as necessary, never as sufficient —
 * table geometry still needs one pass on a real handset.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html', {open: 'never'}]] : [['list']],
  use: {
    baseURL: 'http://127.0.0.1:3003',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {name: 'chromium', use: {...devices['Desktop Chrome']}},
    {name: 'firefox', use: {...devices['Desktop Firefox']}},
    {name: 'webkit', use: {...devices['Desktop Safari']}},
  ],
  // Mock mode serves the whole app with no API behind it (see
  // lib/network/liveness.ts's USE_MOCK short-circuit), so the suite needs no
  // backend, no fixtures and no auth.
  webServer: {
    command: 'npm run dev:mock',
    url: 'http://127.0.0.1:3003/table',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
