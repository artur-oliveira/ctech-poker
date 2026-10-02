import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {Board} from './Board';

vi.mock('@/lib/hooks/useDeckVariant', () => ({useDeckVariant: () => 'classic'}));

describe('Board', () => {
  test('keeps the single-board presentation unchanged without a second board', () => {
    const {container} = render(<Board cards={['Ah', 'Kd', 'Qc']} pot={0}/>);
    expect(screen.getAllByRole('img', {name: /Carta comunitária/})).toHaveLength(3);
    expect(container.querySelector('.board-runouts')).not.toBeInTheDocument();
    expect(container.querySelectorAll('.board > div > span:not(.playing-card)')).toHaveLength(2);
    expect(container.querySelectorAll('.board-slot.is-next')).toHaveLength(1);
    expect(container.querySelector('.board-slot.is-next')).toHaveAttribute('data-suit', '♦');
  });
  
  test('readies all three flop slots on an empty board, one slot after that', () => {
    const {container, rerender} = render(<Board cards={[]} pot={0}/>);
    expect([...container.querySelectorAll('.board-slot.is-next')].map(slot => slot.getAttribute('data-suit')))
      .toEqual(['♠', '♥', '♣']);
    rerender(<Board cards={['Ah', 'Kd', 'Qc', '2s']} pot={0}/>);
    expect(container.querySelectorAll('.board-slot.is-next')).toHaveLength(1);
  });

  test('turns each undealt slot into its own rabbit-hunt purchase with every state on the slot', async () => {
    const onBuy = vi.fn();
    render(<Board cards={[]} pot={0} rabbit={{
      fee: 10, onBuy, slots: {
        0: {status: 'revealed', card: 'As'}, 1: {status: 'verifying'}, 2: {status: 'pending'},
        3: {status: 'failed'}, 4: {status: 'offer'},
      }
    }}/>);
    expect(screen.getByRole('img', {name: /Carta comunitária: ás de espadas/i})).toBeInTheDocument();
    expect(screen.getByRole('status', {name: /Verificando a 2ª carta do flop/})).toBeInTheDocument();
    expect(screen.getByRole('status', {name: /Comprando a 3ª carta do flop/})).toBeInTheDocument();
    expect(screen.getByRole('status', {name: /Não foi possível verificar o turn. Taxa devolvida/})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Ver o river \(rabbit hunt\) por 10/}));
    expect(onBuy).toHaveBeenCalledWith(4);
    expect(screen.getAllByRole('button')).toHaveLength(1);
  });

  test('renders the shared prefix once and labels both divergent runouts', () => {
    render(<Board cards={['Ah', 'Kd', 'Qc', '2s', '3h']} boardTwo={['4c', '5d']}
                  splitAt={3} pot={100}/>);
    expect(screen.getByText('Comum')).toBeInTheDocument();
    expect(screen.getByText('Rodando duas vezes')).toBeInTheDocument();
    expect(screen.getByText('Dois boards no mesmo all-in')).toBeInTheDocument();
    expect(screen.getByLabelText('1ª distribuição')).toBeInTheDocument();
    expect(screen.getByLabelText('2ª distribuição')).toBeInTheDocument();
    expect(screen.getAllByRole('img', {name: /Carta comunitária/})).toHaveLength(7);
    expect(screen.getAllByLabelText(/Carta comunitária: ás de copas/i)).toHaveLength(1);
  });
  test('stacks rake and the pot split on a secondary line under the pot figure', () => {
    const {container} = render(<Board cards={[]} pot={1500} rake={30}
                                      pots={[{amount: 1200, eligible_player_ids: []},
                                        {amount: 300, eligible_player_ids: []}]}/>);
    const main = container.querySelector('.game-pot > .pot-main');
    const detail = container.querySelector('.game-pot > .pot-detail');
    expect(main).toContainElement(screen.getByLabelText('Pote de 1.500 fichas'));
    expect(detail).toContainElement(screen.getByLabelText('Comissão da casa: 30 fichas'));
    expect(detail).toContainElement(screen.getByLabelText('Divisão dos potes'));
    expect(screen.getByLabelText('Principal: 1.200 fichas')).toHaveTextContent('Principal: 1.200');
    expect(screen.getByLabelText('Lateral 1: 300 fichas')).toHaveTextContent('Lateral 1: 300');
  });

  test('omits the secondary line when there is neither rake nor a side pot', () => {
    const {container} = render(<Board cards={[]} pot={100} pots={[{amount: 100, eligible_player_ids: []}]}/>);
    expect(container.querySelector('.pot-detail')).not.toBeInTheDocument();
  });
});
