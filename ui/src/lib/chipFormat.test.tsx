import {describe, expect, test} from 'vitest';
import {render, within} from '@testing-library/react';
import {
  ChipFormatContext, type ChipFormatMode, useChipExact, useChipFormat, useChipUnit, useSandboxChips
} from './chipFormat';

// One probe renders every hook at once, so a mode's four answers are asserted
// as the single coherent contract a surface actually consumes.
function Probe({amount}: {amount: number}) {
  const chips = useChipFormat();
  const exact = useChipExact();
  const unit = useChipUnit();
  const sandbox = useSandboxChips();
  return <dl>
    <dt>display</dt><dd data-testid="display">{chips(amount)}</dd>
    <dt>name</dt><dd data-testid="name">{`${exact(amount)}${unit}`}</dd>
    <dt>sandbox</dt><dd data-testid="sandbox">{String(sandbox)}</dd>
  </dl>;
}

function probe(amount: number, mode?: ChipFormatMode) {
  // Scoped to this render's own container: a test may probe two modes in a row,
  // and testing-library only cleans up between tests.
  const {container} = render(mode === undefined ? <Probe amount={amount}/> :
    <ChipFormatContext.Provider value={mode}><Probe amount={amount}/></ChipFormatContext.Provider>);
  const read = within(container);
  return {
    display: read.getByTestId('display').textContent,
    name: read.getByTestId('name').textContent,
    sandbox: read.getByTestId('sandbox').textContent,
  };
}

describe('useChipFormat and friends', () => {
  test('with no provider nothing is abbreviated and nothing is prefixed', () => {
    // `Seat`, `Board` and `TableStage` render inside `HandReplayer` (/hands/replay,
    // /share), which mounts no provider at all. Abbreviation and `R$` are both
    // opt-in, so the surface that never declared itself gets neither.
    expect(probe(1_250_000)).toEqual({
      display: '1.250.000', name: '1.250.000 fichas', sandbox: 'false',
    });
  });

  test("'chips' abbreviates on display and keeps the exact figure for the accessible name", () => {
    expect(probe(1_250_000, 'chips')).toEqual({
      display: '1,2M', name: '1.250.000 fichas', sandbox: 'true',
    });
  });

  test("'money' reads centavos, prefixes with R$, and never abbreviates — on screen or in the name", () => {
    expect(probe(125_000_000, 'money')).toEqual({
      display: 'R$\u00A01.250.000,00', name: 'R$\u00A01.250.000,00', sandbox: 'false',
    });
  });

  test("'money' drops the chip noun, because the R$ prefix already carries the unit", () => {
    expect(probe(500, 'money').name).toBe('R$\u00A05,00');
    expect(probe(500, 'chips').name).toBe('500 fichas');
  });
});
