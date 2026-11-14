import type {JSX} from 'react';
import {render} from '@testing-library/react';
import {expect} from 'vitest';

/*
 * Shared checks for hat/accessory art (DESIGN.md "Avatar cosmetics art contract").
 * Bounds are conservative: a path's box is the hull of its points and Bezier
 * control points (absolute commands only), widened by half the stroke, so a
 * piece that passes can never paint outside its declared band.
 */

type Box = { x0: number; y0: number; x1: number; y1: number };

const num = (el: Element, a: string) => Number(el.getAttribute(a) ?? 0);

function pathPoints(d: string): Array<[number, number]> {
  expect(d, 'absolute commands only, no arcs').not.toMatch(/[a-zA]/);
  const pts: Array<[number, number]> = [];
  let x = 0, y = 0;
  for (const [, cmd, args] of d.matchAll(/([MLCQSTHVZ])([^MLCQSTHVZ]*)/g)) {
    const n = (args.match(/-?\d*\.?\d+/g) ?? []).map(Number);
    if (cmd === 'H') n.forEach(v => pts.push([x = v, y]));
    else if (cmd === 'V') n.forEach(v => pts.push([x, y = v]));
    else for (let i = 0; i + 1 < n.length; i += 2) pts.push([x = n[i], y = n[i + 1]]);
  }
  return pts;
}

function elementBox(el: Element): Box {
  const sw = el.getAttribute('stroke') && el.getAttribute('stroke') !== 'none' ? num(el, 'stroke-width') / 2 : 0;
  let b: Box;
  switch (el.tagName) {
    case 'circle': {
      const r = num(el, 'r');
      b = {x0: num(el, 'cx') - r, x1: num(el, 'cx') + r, y0: num(el, 'cy') - r, y1: num(el, 'cy') + r};
      break;
    }
    case 'ellipse':
      b = {x0: num(el, 'cx') - num(el, 'rx'), x1: num(el, 'cx') + num(el, 'rx'),
        y0: num(el, 'cy') - num(el, 'ry'), y1: num(el, 'cy') + num(el, 'ry')};
      break;
    case 'rect':
      b = {x0: num(el, 'x'), x1: num(el, 'x') + num(el, 'width'), y0: num(el, 'y'), y1: num(el, 'y') + num(el, 'height')};
      break;
    case 'path': {
      const p = pathPoints(el.getAttribute('d') ?? '');
      b = {x0: Math.min(...p.map(q => q[0])), x1: Math.max(...p.map(q => q[0])),
        y0: Math.min(...p.map(q => q[1])), y1: Math.max(...p.map(q => q[1]))};
      break;
    }
    default:
      throw new Error(`unexpected element <${el.tagName}>`);
  }
  return {x0: b.x0 - sw, x1: b.x1 + sw, y0: b.y0 - sw, y1: b.y1 + sw};
}

/** Renders one art piece, asserts the token/a11y contract and returns its declared band and painted box. */
export function checkArt(Art: () => JSX.Element) {
  const {container} = render(<Art/>);
  const svg = container.querySelector('svg')!;
  expect(svg).not.toBeNull();
  expect(svg.getAttribute('viewBox')).toBe('0 0 64 64');
  expect(svg.getAttribute('aria-hidden')).toBe('true');
  expect(svg.getAttribute('focusable')).toBe('false');
  const markup = container.innerHTML;
  expect(markup).not.toMatch(/#[0-9a-f]{3,8}\b/i);
  expect(markup).not.toMatch(/outline|transform|filter|animate/i);
  const shapes = [...svg.querySelectorAll('*')].filter(el => el.tagName !== 'g');
  expect(shapes.length).toBeGreaterThan(0);
  for (const el of svg.querySelectorAll('*')) {
    for (const attr of ['stroke', 'fill']) {
      const v = el.getAttribute(attr);
      if (v && v !== 'none') expect(v).toMatch(/^var\(--cosmetic-[a-z-]+\)$/);
    }
  }
  const [lo, hi] = (svg.getAttribute('data-y-range') ?? '').split('-').map(Number);
  const box = shapes.map(elementBox).reduce((a, b) => ({
    x0: Math.min(a.x0, b.x0), x1: Math.max(a.x1, b.x1), y0: Math.min(a.y0, b.y0), y1: Math.max(a.y1, b.y1),
  }));
  // The declared range is truthful and nothing leaves the 64 box.
  expect(box.y0).toBeGreaterThanOrEqual(lo);
  expect(box.y1).toBeLessThanOrEqual(hi);
  expect(box.x0).toBeGreaterThanOrEqual(0);
  expect(box.x1).toBeLessThanOrEqual(64);
  return {band: svg.getAttribute('data-band'), lo, hi};
}
