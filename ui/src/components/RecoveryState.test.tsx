import {render, screen} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {RecoveryState} from './RecoveryState';

describe('RecoveryState', () => {
  test('names the problem in a page heading and offers exactly one way back', () => {
    render(<RecoveryState
      title="Este link de mão está incompleto"
      description="O endereço não diz qual mesa abrir."
      action={<a href="/hands">Ver minhas mãos</a>}/>);

    expect(screen.getByRole('heading', {level: 1, name: 'Este link de mão está incompleto'})).toBeInTheDocument();
    expect(screen.getByText('O endereço não diz qual mesa abrir.')).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Ver minhas mãos'})).toHaveAttribute('href', '/hands');
  });

  test('the mark is decoration, not something a screen reader has to count', () => {
    const {container} = render(<RecoveryState title="t" description="d" action={null}/>);
    expect(container.querySelector('.recovery-state-mark')).toHaveAttribute('aria-hidden', 'true');
  });

  test('nested drops the full-page treatment for a shell that already owns the viewport', () => {
    const {container, rerender} = render(<RecoveryState title="t" description="d" action={null}/>);
    expect(container.firstChild).toHaveClass('recovery-state');
    expect(container.firstChild).not.toHaveClass('is-nested');

    rerender(<RecoveryState nested title="t" description="d" action={null}/>);
    expect(container.firstChild).toHaveClass('is-nested');
  });
});
