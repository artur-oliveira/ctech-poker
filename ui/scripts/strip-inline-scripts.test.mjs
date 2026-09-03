import {describe, expect, it} from 'vitest';
import {INLINE_DIR, externalizeInlineScripts} from './strip-inline-scripts.mjs';

describe('externalizeInlineScripts', () => {
  it('replaces each inline script in place and keeps document order', () => {
    const html = [
      '<body>',
      '<script>(self.__next_f=self.__next_f||[]).push([0])</script>',
      '<script src="/_next/static/chunks/a.js" async></script>',
      '<script>self.__next_f.push([1,"payload"])</script>',
      '</body>',
    ].join('');
    const {html: out, assets} = externalizeInlineScripts(html);

    expect(assets.size).toBe(2);
    expect([...assets.values()]).toEqual([
      '(self.__next_f=self.__next_f||[]).push([0])',
      'self.__next_f.push([1,"payload"])',
    ]);
    const srcs = [...out.matchAll(/<script src="([^"]+)"/g)].map((m) => m[1]);
    expect(srcs[0]).toMatch(new RegExp(`^/${INLINE_DIR}/[0-9a-f]{32}\\.js$`));
    expect(srcs[1]).toBe('/_next/static/chunks/a.js');
    expect(out).not.toMatch(/<script>/);
    // No async/defer: a bare `src` keeps the two pushes ordered relative to each other.
    expect(out).not.toMatch(/inline\/[0-9a-f]{32}\.js" (async|defer)/);
  });

  it('is content-addressed, so identical bodies share one file', () => {
    const one = externalizeInlineScripts('<script>a=1</script>');
    const two = externalizeInlineScripts('<p>x</p><script>a=1</script>');
    expect([...one.assets.keys()]).toEqual([...two.assets.keys()]);
  });

  it('leaves external and empty scripts alone', () => {
    const html = '<script src="/x.js"></script><script></script>';
    const {html: out, assets} = externalizeInlineScripts(html);
    expect(assets.size).toBe(0);
    expect(out).toBe(html);
  });

  it('refuses an inline script carrying attributes', () => {
    expect(() => externalizeInlineScripts('<script type="application/ld+json">{}</script>', 'a.html'))
      .toThrow(/a\.html: inline <script type="application\/ld\+json">/);
  });
});
