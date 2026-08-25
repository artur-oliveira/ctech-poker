import {readFileSync} from 'node:fs';
import {join} from 'node:path';
import {describe, expect, test} from 'vitest';

const stylesheet = readFileSync(join(process.cwd(), 'src/app/table-reactions.css'), 'utf8');

function keyframes(name: string): string {
  const body = stylesheet.match(new RegExp(`@keyframes ${name} \\{([\\s\\S]*?)\\n\\}`))?.[1];
  if (!body) throw new Error(`Missing ${name} keyframes`);
  return body;
}

describe('table reaction motion contracts', () => {
  test('self reactions leave crisply without a terminal blur', () => {
    const terminalFrame = keyframes('reaction-theater-emote').match(/100%\s*\{([^}]*)\}/)?.[1];
    expect(terminalFrame).toBeDefined();
    expect(terminalFrame).not.toContain('blur(');
    expect(terminalFrame).toContain('drop-shadow(');
  });

  test('dagger uses the restored bleed choreography', () => {
    expect(stylesheet).toContain('animation: reaction-bleed-drip');
    expect(keyframes('reaction-bleed-drip')).toContain('scaleY(1)');
    expect(stylesheet).not.toContain('reaction-clean-cut');
  });
});
