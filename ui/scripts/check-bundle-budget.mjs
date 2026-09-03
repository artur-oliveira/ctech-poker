#!/usr/bin/env node
// Per-route first-load-JS budget (Issue #60), plus proof that the dev mock
// runtime is not in the production bundle.
//
//   node scripts/check-bundle-budget.mjs            # check against the budget
//   node scripts/check-bundle-budget.mjs --update   # re-pin the budget to now
//
// It reads the shipped static export in `out/` — every `/_next/**.js` the
// route's own HTML references, which is exactly what a cold visit downloads —
// rather than a bundler manifest. That keeps the measurement independent of
// which builder produced it (`next build` and `next build --webpack` name and
// split chunks differently) and of Next's internal manifest shapes.
//
// Budgets are pinned at measured values with TOLERANCE headroom, so an honest
// refactor does not fail the build but a new dependency does. Re-pin
// deliberately, in the same commit as the change that moved the number.

import {readFileSync, readdirSync, statSync, writeFileSync} from 'node:fs';
import {join, relative} from 'node:path';

const OUT_DIR = 'out';
const BUDGET_FILE = 'bundle-budget.json';
/** Growth allowed over the pinned value before CI fails. */
const TOLERANCE = 0.08;
/** Fixture data that exists only inside `src/dev/mockRuntime.ts` — not in the
 * production stubs that replace it, which is what makes it a usable sentinel
 * (the stub modules deliberately keep the same export names). Finding one on a
 * route's critical path means the dev simulator shipped. */
const MOCK_SENTINELS = ['bia_sp', 'snapshotForScenario('];

function walk(dir) {
  const found = [];
  for (const entry of readdirSync(dir, {withFileTypes: true})) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) found.push(...walk(path));
    else found.push(path);
  }
  return found;
}

const files = walk(OUT_DIR);
const sizes = new Map(files.map(path => [`/${relative(OUT_DIR, path)}`, statSync(path).size]));

/** `/lobby.html` → `/lobby`, `/index.html` → `/`. */
function routeOf(htmlPath) {
  const route = `/${relative(OUT_DIR, htmlPath).replace(/\.html$/, '')}`;
  return route === '/index' ? '/' : route.replace(/\/index$/, '');
}

/** Every `/_next/**.js` any route's HTML references: the union of all
 * first-load critical paths. Lazily-imported chunks are deliberately excluded —
 * they cost nothing until something asks for them. */
const firstLoad = new Set();

function measure() {
  const routes = {};
  for (const path of files.filter(file => file.endsWith('.html'))) {
    const html = readFileSync(path, 'utf8');
    const referenced = new Set(html.match(/\/_next\/[^"']+?\.js/g) ?? []);
    let total = 0;
    for (const asset of referenced) {
      total += sizes.get(asset) ?? 0;
      firstLoad.add(asset);
    }
    routes[routeOf(path)] = total;
  }
  return routes;
}

const kb = value => `${(value / 1024).toFixed(1)} kB`;
const measured = measure();

if (process.argv.includes('--update')) {
  const sorted = Object.fromEntries(Object.entries(measured).sort(([a], [b]) => a.localeCompare(b)));
  writeFileSync(BUDGET_FILE, `${JSON.stringify({tolerance: TOLERANCE, routes: sorted}, null, 2)}\n`);
  console.log(`Pinned ${Object.keys(sorted).length} route budgets in ${BUDGET_FILE}.`);
  process.exit(0);
}

const {routes: budget} = JSON.parse(readFileSync(BUDGET_FILE, 'utf8'));
const failures = [];

for (const [route, size] of Object.entries(measured).sort(([a], [b]) => a.localeCompare(b))) {
  const limit = budget[route];
  if (limit === undefined) {
    failures.push(`${route}: no budget pinned — run \`npm run bundle:pin\` in the commit that added the route`);
    continue;
  }
  const ceiling = Math.round(limit * (1 + TOLERANCE));
  console.log(`${(size > ceiling ? 'OVER' : 'ok').padEnd(4)} ${route.padEnd(24)} ${kb(size).padStart(10)} / ${kb(ceiling)}`);
  if (size > ceiling) failures.push(`${route}: ${kb(size)} exceeds ${kb(ceiling)} (pinned ${kb(limit)})`);
}

for (const route of Object.keys(budget)) {
  if (!(route in measured)) console.log(`gone ${route} (stale budget entry — drop it)`);
}

for (const asset of firstLoad) {
  const source = readFileSync(join(OUT_DIR, asset), 'utf8');
  for (const sentinel of MOCK_SENTINELS) {
    if (source.includes(sentinel)) failures.push(`${asset} carries the dev-only marker ${sentinel}`);
  }
}

if (failures.length) {
  console.error(`\n${failures.length} budget failure(s):\n${failures.map(line => `  - ${line}`).join('\n')}`);
  process.exit(1);
}
console.log('\nAll route budgets within tolerance; no dev mock runtime in the bundle.');
