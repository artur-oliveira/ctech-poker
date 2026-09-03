import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {Button} from './button';

// Issue #107: every async action used to hand-roll `disabled={isPending}` plus
// a manual label swap. The pending affordance belongs to the primitive.
describe('Button pending state', () => {
  test('disables, announces busy and renders a spinner while loading', () => {
    render(<Button loading>Saindo…</Button>);
    const button = screen.getByRole('button', {name: 'Saindo…'});
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button.querySelector('.spin')).not.toBeNull();
  });

  test('swallows a second activation while loading', async () => {
    const onClick = vi.fn();
    const {rerender} = render(<Button onClick={onClick}>Sair da conta</Button>);
    await userEvent.click(screen.getByRole('button'));
    expect(onClick).toHaveBeenCalledOnce();

    rerender(<Button loading onClick={onClick}>Saindo…</Button>);
    await userEvent.click(screen.getByRole('button'));
    expect(onClick).toHaveBeenCalledOnce();
  });

  test('is a plain button with no spinner and no aria-busy when idle', () => {
    render(<Button>Tentar novamente</Button>);
    const button = screen.getByRole('button', {name: 'Tentar novamente'});
    expect(button).toBeEnabled();
    expect(button).not.toHaveAttribute('aria-busy');
    expect(button.querySelector('.spin')).toBeNull();
  });

  test('stays disabled when the caller disables it for its own reasons', () => {
    render(<Button disabled>Aceitar e continuar</Button>);
    expect(screen.getByRole('button')).toBeDisabled();
    expect(screen.getByRole('button')).not.toHaveAttribute('aria-busy');
  });

  test('keeps the label so the button does not resize mid-press', () => {
    const {rerender} = render(<Button>Buscar</Button>);
    rerender(<Button loading>Buscando…</Button>);
    expect(screen.getByRole('button')).toHaveAccessibleName('Buscando…');
  });
});
