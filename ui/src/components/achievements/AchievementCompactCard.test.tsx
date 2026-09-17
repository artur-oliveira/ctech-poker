import {render, screen} from '@testing-library/react';
import {describe, expect, test, vi} from 'vitest';
import type {Achievement} from '@/lib/api/achievements';
import {
  AchievementCompactCard, achievementSummaryLabel, achievementSummaryView
} from './AchievementCompactCard';

vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: {card: string}) => <span data-testid="playing-card">{card}</span>,
}));

const wins: Achievement = {
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

describe('AchievementCompactCard', () => {
  test('carries the same vocabulary as the full card: art, name, value, stars', () => {
    render(<AchievementCompactCard achievement={wins} count={1234}/>);

    expect(screen.getAllByTestId('playing-card').map(card => card.textContent)).toEqual(['AH', 'AD']);
    expect(screen.getByText('Vitórias')).toBeInTheDocument();
    expect(screen.getByText('1.234')).toBeInTheDocument();
    expect(screen.getByText('5/5')).toBeInTheDocument();
  });

  test('writes durations the way the catalogue writes them', () => {
    const noRush: Achievement = {key: 'no_rush', metric: 'time_bank_ms_consumed', tiers: [{stars: 1, threshold: 60_000}]};
    render(<AchievementCompactCard achievement={noRush} count={3_600_000}/>);
    expect(screen.getByText('1 hora')).toBeInTheDocument();
  });

  test('omits the art slot entirely for a key with no illustrative cards', () => {
    const unknown: Achievement = {key: 'from_a_newer_server', metric: 'x', tiers: [{stars: 1, threshold: 1}]};
    const {container} = render(<AchievementCompactCard achievement={unknown} count={0}/>);
    expect(container.querySelector('.achievement-compact-art')).toBeNull();
    expect(screen.getByText('0/1')).toBeInTheDocument();
  });

  test('derives the readout and the accessible name from one source', () => {
    const view = achievementSummaryView(wins, 25);
    expect(view).toMatchObject({label: 'Vitórias', value: '25', starsFilled: 3, starsTotal: 5, maxed: false});
    expect(achievementSummaryLabel(view)).toBe('Vitórias, 25 registrados, 3 de 5 estrelas');
  });

  test('marks a maxed achievement', () => {
    expect(achievementSummaryView(wins, 1000).maxed).toBe(true);
    const {container} = render(<AchievementCompactCard achievement={wins} count={1000}/>);
    expect(container.querySelector('.achievement-compact.is-complete')).not.toBeNull();
  });
});
