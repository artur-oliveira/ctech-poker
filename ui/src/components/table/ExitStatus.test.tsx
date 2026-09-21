import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {ExitStatus} from './ExitStatus';

describe('ExitStatus', () => {
  test('renders nothing when the viewer has no pending exit', () => {
    const {container} = render(<ExitStatus pendingExit={false} isViewerTurn={false} onCancelAction={vi.fn()}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test('shows the indefinite waiting copy when it is not the viewer\'s turn', () => {
    render(<ExitStatus pendingExit isViewerTurn={false} onCancelAction={vi.fn()}/>);
    expect(screen.getByText('Saindo assim que a mão terminar')).toBeInTheDocument();
  });

  test('cancel clears the pending exit', async () => {
    const onCancel = vi.fn();
    render(<ExitStatus pendingExit isViewerTurn={false} onCancelAction={onCancel}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar saída'}));
    expect(onCancel).toHaveBeenCalledOnce();
  });

  test('keeps cancel reachable on the viewer\'s own turn, when they have no other control left', async () => {
    // A pending exit hides the action buttons. If the server's auto-fold sweep
    // is late, withholding cancel too left the player unable to do anything at
    // all while their turn and time bank ran out (live report, 2026-09-21).
    const onCancel = vi.fn();
    render(<ExitStatus pendingExit isViewerTurn onCancelAction={onCancel}/>);
    expect(screen.getByText(/Saindo/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar saída'}));
    expect(onCancel).toHaveBeenCalledOnce();
  });
});
