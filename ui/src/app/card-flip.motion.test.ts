import {readFileSync} from 'node:fs';
import {join} from 'node:path';
import {describe, expect, test} from 'vitest';

// A revealed card's RESTING state must be its final, fully-visible face, with
// no dependence on an animation advancing: WebKit strands a freshly-created CSS
// animation `pending` on the frame the ~150 kB protobuf `state` decodes on, and
// `animation-fill-mode: both` would then hold the card at `opacity: 0`
// indefinitely — a blank space on the felt until F5 (user reports, 2026-09-10,
// desktop and Safari). The deal-in / flip runs only while `[data-card-revealing]`
// is present, set for one render span by the parent. Plus the Safari compositing
// contract from docs/2026-09-10-card-flip-safari-compositing.md.
// See docs/2026-09-10-card-reveal-visibility.md.
const stylesheet = readFileSync(join(process.cwd(), 'src/app/renderer.css'), 'utf8');

function keyframes(name: string): string {
  const body = stylesheet.match(new RegExp(`@keyframes ${name} \\{([\\s\\S]*?)\\n\\}`))?.[1];
  if (!body) throw new Error(`Missing ${name} keyframes`);
  return body;
}

function rule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const body = stylesheet.match(new RegExp(`(?:^|\\n)${escaped}(?:,[^{]*)?\\s*\\{([\\s\\S]*?)\\n\\}`))?.[1];
  if (!body) throw new Error(`Missing rule ${selector}`);
  return body;
}

describe('card reveal resting-state contract', () => {
  test('a revealed card is visible at rest: front face shown, no animation-name on the bare node', () => {
    expect(stylesheet).toMatch(/\.card-reveal \.card-front \{\s*z-index: 1;\s*opacity: 1\s*\}/);
    expect(stylesheet).toMatch(/\.card-reveal \.card-back \{\s*z-index: 2;\s*opacity: 0\s*\}/);
    // The flip keyframes may only be named under a `[data-card-revealing]`
    // selector — never on a bare `.card-reveal` face.
    const flipRules = [...stylesheet.matchAll(/([^\n}]*)\{([^}]*animation-name:\s*card-(?:back|front)-turn[^}]*)\}/g)];
    expect(flipRules.length).toBeGreaterThan(0);
    for (const [, selector] of flipRules) expect(selector).toContain('[data-card-revealing]');
    expect(rule('.card-reveal-inner')).not.toMatch(/animation:\s*(?!none)[a-z]/);
  });

  test('the entrance animation is gated, and dropped after a bounded window', () => {
    expect(stylesheet).toMatch(/\.board-card\[data-card-revealing\] \.card-reveal-inner \{\s*animation: board-card-reveal/);
    expect(stylesheet).toMatch(/\.card-reveal\[data-card-revealing\] \.card-front \{\s*animation-name: card-front-turn/);
    // CARD_REVEAL_MS in PlayingCard.tsx caps how long the mark (and any stalled
    // animation) lasts; it must clear the longest board reveal + stagger.
    const src = readFileSync(join(process.cwd(), 'src/components/table/PlayingCard.tsx'), 'utf8');
    const ms = Number(src.match(/CARD_REVEAL_MS\s*=\s*(\d+)/)?.[1]);
    expect(ms).toBeGreaterThanOrEqual(780 + 640);
  });

  test('a failed face fetch falls back to the card back, not a hole', () => {
    expect(rule('.card-reveal.face-fallback .card-back')).toContain('opacity: 1');
  });
});

describe('card-flip Safari compositing contract', () => {
  test('every flip keyframe frame carries a translateZ nudge to hold the layer', () => {
    for (const name of ['card-back-turn', 'card-front-turn']) {
      const frames = keyframes(name).match(/transform:[^;\n]+/g) ?? [];
      expect(frames.length).toBeGreaterThan(0);
      for (const frame of frames) expect(frame).toMatch(/translateZ\(0\)|translate3d\(/);
    }
  });

  test('the deal-in reveal keyframes stay on a 3d transform even at rest', () => {
    for (const name of ['board-card-reveal', 'hole-card-reveal']) {
      const frames = keyframes(name).match(/transform:[^;\n]+/g) ?? [];
      expect(frames.length).toBeGreaterThan(0);
      for (const frame of frames) expect(frame).toMatch(/translateZ\(0\)|translate3d\(/);
      expect(keyframes(name)).not.toMatch(/transform:\s*none/);
    }
  });

  test('flip layers are promoted while revealing, not at animation start', () => {
    const faces = rule('.card-reveal[data-card-revealing] .card-reveal-inner');
    expect(faces).toContain('will-change: transform, opacity');
  });
});
