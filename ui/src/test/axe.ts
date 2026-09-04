import {act} from '@testing-library/react';
import axe from 'axe-core';
import {expect} from 'vitest';

// ponytail: axe-core is not declared in package.json — it arrives pinned
// (4.13.0 in package-lock.json) as eslint-plugin-jsx-a11y's dependency, and no
// install step was available when this gate was added. Promote it to an
// explicit devDependency (`npm i -D axe-core`) the next time the lockfile is
// regenerated; nothing else here changes.

/**
 * Rules jsdom cannot answer, so their result is noise either way:
 * `color-contrast` needs real layout and a canvas, and `region` needs the whole
 * document (component tests render a fragment, not a page).
 */
const UNRUNNABLE_IN_JSDOM = ['color-contrast', 'region'];

/** Fail the build on these; `moderate`/`minor` findings are reported, not fatal. */
const BLOCKING = new Set(['serious', 'critical']);

function describeViolation(violation: axe.Result) {
  const nodes = violation.nodes.map(node => `      ${node.html}`).join('\n');
  return `  [${violation.impact}] ${violation.id}: ${violation.help}\n${nodes}`;
}

/**
 * Asserts the rendered subtree has no serious or critical axe violation
 * (Issue #60). Pass the `container` from `render()`, or any attached element.
 */
export async function expectNoAxeViolations(container: Element) {
  // Wrapped in act(): the scan awaits, and any React update that settles
  // during it (an effect, a resolved query) would otherwise land outside act
  // and warn once per update — the noise this helper used to generate across
  // every page test.
  let violations: axe.Result[] = [];
  await act(async () => {
    ({violations} = await axe.run(container, {
      resultTypes: ['violations'],
      rules: Object.fromEntries(UNRUNNABLE_IN_JSDOM.map(id => [id, {enabled: false}])),
    }));
  });
  const blocking = violations.filter(violation => BLOCKING.has(violation.impact ?? ''));
  expect(blocking.map(describeViolation).join('\n\n'), 'axe violations').toBe('');
}
