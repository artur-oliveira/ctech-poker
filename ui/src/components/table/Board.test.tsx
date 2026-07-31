import {render, screen} from '@testing-library/react';
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
  
  test('renders the shared prefix once and labels both divergent runouts', () => {
    render(<Board cards={['Ah', 'Kd', 'Qc', '2s', '3h']} boardTwo={['4c', '5d']}
                  splitAt={3} pot={100}/>);
    expect(screen.getByText('Comum')).toBeInTheDocument();
    expect(screen.getByLabelText('1ª distribuição')).toBeInTheDocument();
    expect(screen.getByLabelText('2ª distribuição')).toBeInTheDocument();
    expect(screen.getAllByRole('img', {name: /Carta comunitária/})).toHaveLength(7);
    expect(screen.getAllByLabelText(/Carta comunitária: ás de copas/i)).toHaveLength(1);
  });
});
