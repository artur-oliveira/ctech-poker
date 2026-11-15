import type {JSX} from 'react';
import {render} from '@testing-library/react';
import {expect} from 'vitest';

/*
 * Shared checks for hat art (DESIGN.md "Avatar cosmetics art contract").
 * `elementBox` is conservative: a path's box is the hull of its points and
 * Bezier control points (absolute commands only), widened by half the stroke,
 * so a piece that passes can never paint outside it. `elementSamples` walks the
 * real outline (curves evaluated, straight edges included), which is what the
 * "photo stays uncovered" radius check needs.
 */

type Box = { x0: number; y0: number; x1: number; y1: number };
type Pt = [number, number];

const num = (el: Element, a: string) => Number(el.getAttribute(a) ?? 0);
const halfStroke = (el: Element) =>
  el.getAttribute('stroke') && el.getAttribute('stroke') !== 'none' ? num(el, 'stroke-width') / 2 : 0;

/** Segments of an absolute M/L/H/V/Q/C/Z path, each as its control polygon. */
function pathSegments(d: string): Pt[][] {
  expect(d, 'absolute commands only, no arcs').not.toMatch(/[a-zA]/);
  const segments: Pt[][] = [];
  let at: Pt = [0, 0], start: Pt = [0, 0];
  for (const [, cmd, args] of d.matchAll(/([MLCQSTHVZ])([^MLCQSTHVZ]*)/g)) {
    const n = (args.match(/-?\d*\.?\d+/g) ?? []).map(Number);
    const pts: Pt[] = cmd === 'H' ? n.map(v => [v, at[1]]) : cmd === 'V' ? n.map(v => [at[0], v])
      : Array.from({length: Math.floor(n.length / 2)}, (_, i) => [n[2 * i], n[2 * i + 1]]);
    if (cmd === 'M') {
      at = start = pts[0];
      pts.slice(1).forEach(p => { segments.push([at, p]); at = p; });
    } else if (cmd === 'Z') {
      segments.push([at, start]);
      at = start;
    } else {
      const step = cmd === 'Q' ? 2 : cmd === 'C' ? 3 : 1;
      for (let i = 0; i + step <= pts.length; i += step) {
        segments.push([at, ...pts.slice(i, i + step)]);
        at = pts[i + step - 1];
      }
    }
  }
  return segments;
}

/** De Casteljau point at t on a control polygon of any degree. */
function bezier(poly: Pt[], t: number): Pt {
  let p = poly;
  while (p.length > 1) p = p.slice(1).map((q, i) => [p[i][0] + (q[0] - p[i][0]) * t, p[i][1] + (q[1] - p[i][1]) * t]);
  return p[0];
}

export function elementBox(el: Element): Box {
  const sw = halfStroke(el);
  let pts: Pt[];
  switch (el.tagName) {
    case 'circle': {
      const r = num(el, 'r');
      pts = [[num(el, 'cx') - r, num(el, 'cy') - r], [num(el, 'cx') + r, num(el, 'cy') + r]];
      break;
    }
    case 'rect':
      pts = [[num(el, 'x'), num(el, 'y')], [num(el, 'x') + num(el, 'width'), num(el, 'y') + num(el, 'height')]];
      break;
    case 'path':
      pts = pathSegments(el.getAttribute('d') ?? '').flat();
      break;
    default:
      throw new Error(`unexpected element <${el.tagName}>`);
  }
  return {x0: Math.min(...pts.map(p => p[0])) - sw, x1: Math.max(...pts.map(p => p[0])) + sw,
    y0: Math.min(...pts.map(p => p[1])) - sw, y1: Math.max(...pts.map(p => p[1])) + sw};
}

/** The closest an element's painted outline (stroke included) comes to the avatar centre (32,32). */
export function closestToCentre(el: Element): number {
  const dist = ([x, y]: Pt) => Math.hypot(x - 32, y - 32);
  const sw = halfStroke(el);
  if (el.tagName === 'circle') return dist([num(el, 'cx'), num(el, 'cy')]) - num(el, 'r') - sw;
  const segments = el.tagName === 'rect'
    ? (([x, y, w, h]) => [[[x, y], [x + w, y]], [[x + w, y], [x + w, y + h]], [[x + w, y + h], [x, y + h]], [[x, y + h], [x, y]]] as Pt[][])(
      ['x', 'y', 'width', 'height'].map(a => num(el, a)))
    : pathSegments(el.getAttribute('d') ?? '');
  let min = Infinity;
  for (const poly of segments) for (let i = 0; i <= 40; i++) min = Math.min(min, dist(bezier(poly, i / 40)));
  return min - sw;
}

/** Renders one art piece, asserts the token/a11y contract and returns its svg, shapes and painted box. */
export function checkArt(Art: () => JSX.Element) {
  const {container} = render(<Art/>);
  const svg = container.querySelector('svg')!;
  expect(svg).not.toBeNull();
  expect(svg.getAttribute('aria-hidden')).toBe('true');
  expect(svg.getAttribute('focusable')).toBe('false');
  const markup = container.innerHTML;
  expect(markup).not.toMatch(/#[0-9a-f]{3,8}\b/i);
  expect(markup).not.toMatch(/outline|transform|filter|animate|gradient/i);
  const shapes = [...svg.querySelectorAll('*')].filter(el => el.tagName !== 'g');
  expect(shapes.length).toBeGreaterThan(0);
  for (const el of shapes) {
    for (const attr of ['stroke', 'fill']) {
      const v = el.getAttribute(attr);
      if (v && v !== 'none') expect(v).toMatch(/^var\(--cosmetic-[a-z-]+\)$/);
    }
  }
  const box = shapes.map(elementBox).reduce((a, b) => ({
    x0: Math.min(a.x0, b.x0), x1: Math.max(a.x1, b.x1), y0: Math.min(a.y0, b.y0), y1: Math.max(a.y1, b.y1),
  }));
  return {svg, shapes, box};
}
