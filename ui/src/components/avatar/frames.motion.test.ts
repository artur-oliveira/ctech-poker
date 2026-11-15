import {readFileSync} from 'node:fs';
import {join} from 'node:path';
import {describe, expect, test} from 'vitest';

// Animated frames (DESIGN.md "Avatar cosmetics art contract"): whole <svg
// data-fx> layers moved by transform/opacity only (compositor work for 9
// seats), standing still under reduced motion and on seats that are not playing.
const stylesheet = readFileSync(join(process.cwd(), 'src/app/renderer.css'), 'utf8');
const FX = ['flame', 'flame-core', 'breathe', 'orbit', 'orbit-back', 'arc', 'arc-back', 'sweep', 'twinkle', 'twinkle-late'];

describe('animated frame motion contract', () => {
  test.each(FX)('svg[data-fx=%s] has an animation', fx => {
    expect(stylesheet).toMatch(new RegExp(`svg\\[data-fx='${fx}'\\] \\{[^}]*animation: cosmetic-`));
  });

  test('cosmetic keyframes animate transform and opacity only (no filter, no layout)', () => {
    const blocks = [...stylesheet.matchAll(/@keyframes (cosmetic-[a-z-]+) \{([\s\S]*?)\n\}/g)];
    expect(blocks.map(b => b[1]).sort()).toEqual(['cosmetic-breathe', 'cosmetic-flame', 'cosmetic-flame-core', 'cosmetic-flicker',
      'cosmetic-spin', 'cosmetic-twinkle']);
    for (const [, name, body] of blocks) {
      const props = [...body.matchAll(/([a-z-]+)\s*:/g)].map(m => m[1]);
      expect(props.length, name).toBeGreaterThan(0);
      for (const prop of props) expect(['transform', 'opacity'], `${name}: ${prop}`).toContain(prop);
    }
  });

  test('reduced motion and seats that are not playing show the static frame', () => {
    expect(stylesheet).toMatch(/@media \(prefers-reduced-motion: reduce\) \{\s*svg\[data-fx\] \{\s*animation: none\s*\}/);
    expect(stylesheet).toMatch(
      /\.game-seat:is\(\.folded, \.sitting_out, \.disconnected, \.is-pending-name\) svg\[data-fx\] \{\s*animation: none\s*\}/);
  });
});
