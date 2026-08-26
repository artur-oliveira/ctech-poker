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

  test('does not show a cancel action once it is the viewer\'s own turn (an imminent auto-fold)', () => {
    render(<ExitStatus pendingExit isViewerTurn onCancelAction={vi.fn()}/>);
    expect(screen.queryByRole('button', {name: 'Cancelar saída'})).not.toBeInTheDocument();
    expect(screen.getByText(/Saindo/)).toBeInTheDocument();
  });
});
