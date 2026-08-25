import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {TableUtilityMenu} from './TableUtilityMenu';

describe('TableUtilityMenu', () => {
  test('keeps quick chat and reactions out of More and reports a secondary selection', async () => {
    const onSelectAction = vi.fn();
    render(<TableUtilityMenu active={null} winnersAvailable={false} onSelectAction={onSelectAction}/>);

    await userEvent.click(screen.getByRole('button', {name: 'Mais ações da mesa'}));
    expect(screen.queryByRole('button', {name: 'Chat da mesa'})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Reações'})).not.toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Últimos vencedores'})).toBeDisabled();
    await userEvent.click(screen.getByRole('button', {name: 'Ranking de mãos'}));
    expect(onSelectAction).toHaveBeenCalledWith('rankings');
    expect(screen.queryByRole('button', {name: 'Ranking de mãos'})).not.toBeInTheDocument();
  });

  test('hides the equity trainer entry until it is explicitly visible, and disables it mid-turn', async () => {
    const onSelectAction = vi.fn();
    const {rerender} = render(<TableUtilityMenu active={null} winnersAvailable
      onSelectAction={onSelectAction}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Mais ações da mesa'}));
    expect(screen.queryByRole('button', {name: 'Treinador'})).not.toBeInTheDocument();

    rerender(<TableUtilityMenu active={null} winnersAvailable equityTrainerVisible
      equityTrainerAvailable={false} onSelectAction={onSelectAction}/>);
    expect(screen.getByRole('button', {name: 'Treinador'})).toBeDisabled();
  });
});
