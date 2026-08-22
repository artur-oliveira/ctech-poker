import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {TableUtilityMenu} from './TableUtilityMenu';

describe('TableUtilityMenu', () => {
  test('consolidates narrow-screen table tools and reports the selected panel', async () => {
    const onSelectAction = vi.fn();
    render(<TableUtilityMenu active="chat" winnersAvailable={false} onSelectAction={onSelectAction}/>);

    await userEvent.click(screen.getByRole('button', {name: 'Ferramentas da mesa'}));
    expect(screen.getByRole('button', {name: 'Chat da mesa'})).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', {name: 'Últimos vencedores'})).toBeDisabled();
    await userEvent.click(screen.getByRole('button', {name: 'Reações'}));
    expect(onSelectAction).toHaveBeenCalledWith('reactions');
    expect(screen.queryByRole('button', {name: 'Chat da mesa'})).not.toBeInTheDocument();
  });
});
