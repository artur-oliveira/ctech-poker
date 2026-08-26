import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {LeaveDialog} from './LeaveDialog';

describe('LeaveDialog', () => {
  test('sends the exit request on confirm regardless of dealt-in state', async () => {
    const onRequestExit = vi.fn(() => true);
    render(<LeaveDialog stack={480} pending={false} onRequestExitAction={onRequestExit}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Sair da mesa'}));
    await userEvent.click(screen.getByRole('button', {name: 'Sair e sacar fichas'}));
    expect(onRequestExit).toHaveBeenCalledOnce();
  });

  test('disables the confirm button while the request is pending', async () => {
    render(<LeaveDialog stack={480} pending onRequestExitAction={() => true}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Sair da mesa'}));
    expect(screen.getByRole('button', {name: 'Saindo…'})).toBeDisabled();
  });
});
