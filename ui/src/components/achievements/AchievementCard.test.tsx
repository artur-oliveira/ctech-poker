import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, test, vi} from 'vitest';

import type {Achievement} from '@/lib/api/achievements';
import {AchievementCard} from './AchievementCard';

vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: { card: string }) => <span data-testid="playing-card">{card}</span>,
}));

const achievement: Achievement = {
  key: 'wins',
  metric: 'wins',
  tiers: [
    {stars: 1, threshold: 1},
    {stars: 2, threshold: 10},
    {stars: 3, threshold: 25},
    {stars: 4, threshold: 100},
    {stars: 5, threshold: 1000},
  ],
};

describe('AchievementCard', () => {
  test('shows catalog metadata and locked tier ladder without player progress', () => {
    render(<AchievementCard achievement={achievement}/>);
    
    expect(screen.getByRole('heading', {name: 'Vitórias'})).toBeInTheDocument();
    expect(screen.getByText('Toda mão vencida conta um ponto.')).toBeInTheDocument();
    expect(screen.getAllByTestId('playing-card')).toHaveLength(2);
    expect(screen.getByLabelText('Progresso disponível após entrar na conta')).toBeInTheDocument();
    expect(screen.getByText('1 · 10 · 25 · 100 · 1.000')).toBeInTheDocument();
  });
  
  test('renders earned stars and progress toward the next tier', () => {
    render(<AchievementCard achievement={achievement} count={25}/>);
    
    expect(screen.getByLabelText('3 de 5 estrelas, 25 registrados')).toBeInTheDocument();
    expect(screen.getByText('25/100')).toBeInTheDocument();
    expect(document.querySelectorAll('.achievement-star.is-filled')).toHaveLength(3);
  });
  
  test('marks a fully completed achievement', () => {
    render(<AchievementCard achievement={achievement} count={1200}/>);
    expect(screen.getByText('Completo')).toBeInTheDocument();
    expect(document.querySelectorAll('.achievement-star.is-filled')).toHaveLength(5);
  });
  
  test('previews a tier by mouse or keyboard and restores current progress afterward', () => {
    render(<AchievementCard achievement={achievement} count={2}/>);
    const fourthTier = screen.getByRole('button', {name: 'Nível 4: 100'});
    
    fireEvent.mouseEnter(fourthTier);
    expect(screen.getByRole('tooltip')).toHaveTextContent('2/100');
    expect(document.querySelectorAll('.achievement-star.is-filled')).toHaveLength(4);
    
    fireEvent.mouseLeave(fourthTier.closest('.achievement-stars')!);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    expect(document.querySelectorAll('.achievement-star.is-filled')).toHaveLength(1);
    
    fireEvent.focus(screen.getByRole('button', {name: 'Nível 2: 10'}));
    expect(screen.getByRole('tooltip')).toHaveTextContent('2/10');
    fireEvent.blur(screen.getByRole('button', {name: 'Nível 2: 10'}));
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });
  
  test('previews a locked tier using only its threshold', () => {
    render(<AchievementCard achievement={achievement}/>);
    fireEvent.mouseEnter(screen.getByRole('button', {name: 'Nível 5: 1.000'}));
    expect(screen.getByRole('tooltip')).toHaveTextContent('1.000');
  });
});
