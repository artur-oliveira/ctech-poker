import {render} from '@testing-library/react';
import {describe, expect, test, vi} from 'vitest';
import {Button} from './ui/button';
import {RecoveryState} from './RecoveryState';
import {SystemState} from './SystemState';
import {expectNoAxeViolations} from '@/test/axe';

vi.mock('next/link', () => ({default: ({children, href}: {children: React.ReactNode; href: string}) =>
  <a href={href}>{children}</a>}));

// Issue #60: the recovery vocabulary is what a player meets on the worst day,
// so it is the surface an automated a11y gate is worth the most on.
describe('recovery surfaces are axe-clean', () => {
  test.each(['404', '500', '503'] as const)('SystemState %s', async code => {
    const {container} = render(<SystemState code={code} title="Título" description="Descrição"
                                            detail="Detalhe" onRetryAction={vi.fn()}/>);
    await expectNoAxeViolations(container);
  });

  test('SystemState with a pending retry', async () => {
    const {container} = render(<SystemState code="503" title="Título" description="Descrição"
                                            detail="Verificando…" retryPending onRetryAction={vi.fn()}/>);
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    await expectNoAxeViolations(container);
  });

  test('RecoveryState', async () => {
    const {container} = render(<RecoveryState title="Título" description="Descrição"
                                              action={<Button onClick={vi.fn()}>Tentar novamente</Button>}/>);
    await expectNoAxeViolations(container);
  });
});
