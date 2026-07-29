import {render, screen} from '@testing-library/react';
import {describe, expect, test, vi} from 'vitest';

import {HAND_RANKINGS} from '@/lib/pokerRules';
import {HandRankings} from './HandRankings';

vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card, index, size}: { card: string; index: number; size: string }) =>
    <i data-testid="ranking-card" data-card={card} data-index={index} data-size={size}/>,
}));

describe('HandRankings', () => {
  test('renders every hand in strongest-to-weakest order with examples', () => {
    render(<HandRankings/>);
    const entries = screen.getAllByRole('listitem');
    
    expect(entries).toHaveLength(HAND_RANKINGS.length);
    expect(entries[0]).toHaveTextContent('1');
    expect(entries[0]).toHaveTextContent(HAND_RANKINGS[0].label);
    expect(entries.at(-1)).toHaveTextContent(HAND_RANKINGS.at(-1)!.label);
    expect(screen.getAllByTestId('ranking-card')).toHaveLength(
      HAND_RANKINGS.reduce((total, hand) => total + hand.example.length, 0),
    );
    expect(screen.getAllByTestId('ranking-card')[0]).toHaveAttribute('data-size', 'hole');
  });
  
  test('adds the compact presentation used by the table dialog', () => {
    const {container} = render(<HandRankings compact/>);
    expect(container.querySelector('ol')).toHaveClass('hand-ranking-list', 'compact');
    expect(screen.getByText(HAND_RANKINGS[0].description)).toBeInTheDocument();
  });
});
