import {render, screen} from '@testing-library/react';
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
    expect(screen.getByRole('img', {
      name: 'Progresso disponível após entrar na conta. Níveis: 1 · 10 · 25 · 100 · 1.000',
    })).toBeInTheDocument();
    expect(screen.getByText('1 · 10 · 25 · 100 · 1.000')).toBeInTheDocument();
  });
  
  test('renders earned stars and progress toward the next tier', () => {
    render(<AchievementCard achievement={achievement} count={25}/>);
    
    expect(screen.getByRole('img', {
      name: '3 de 5 estrelas, 25 registrados. Níveis: 1 · 10 · 25 · 100 · 1.000',
    })).toBeInTheDocument();
    expect(screen.getByText('25/100')).toBeInTheDocument();
    expect(document.querySelectorAll('.achievement-star.is-filled')).toHaveLength(3);
    expect(screen.getByRole('progressbar', {name: 'Progresso de Vitórias'})).toHaveAttribute('aria-valuenow', '0');
  });
  
  test('marks a fully completed achievement', () => {
    render(<AchievementCard achievement={achievement} count={1200}/>);
    expect(screen.getByText('Completo')).toBeInTheDocument();
    expect(screen.getByText('Dominada')).toBeInTheDocument();
    const track = screen.getByRole('progressbar', {name: 'Progresso de Vitórias'});
    expect(track).toHaveAttribute('aria-valuenow', '100');
    // scaleX, not width: the fill must not animate a layout property.
    expect(track.firstElementChild).toHaveStyle({'--fill': '1'});
    expect(document.querySelectorAll('.achievement-star.is-filled')).toHaveLength(5);
  });
  
  // Issue #116: the ladder used to be five <button>s whose only handlers were
  // hover previews — activation did nothing and each card cost five tab stops.
  test('exposes the ladder as one labelled, focusable readout', () => {
    render(<AchievementCard achievement={achievement} count={2}/>);

    expect(screen.queryAllByRole('button')).toHaveLength(0);
    const ladder = screen.getByRole('img', {name: /1 de 5 estrelas/});
    expect(ladder).toHaveAttribute('tabindex', '0');
    // The whole card must cost at most one tab stop.
    expect(document.querySelectorAll('[tabindex="0"], button, a[href], input')).toHaveLength(1);
    // Every threshold stays in the accessible name, so nothing is hover-only.
    expect(ladder).toHaveAccessibleName(/Níveis: 1 · 10 · 25 · 100 · 1\.000/);
    expect(document.querySelectorAll('.achievement-star')).toHaveLength(5);
    expect(document.querySelectorAll('.achievement-star.is-filled')).toHaveLength(1);
  });

  test('omits the hover readout on the locked variant, which already lists the ladder', () => {
    render(<AchievementCard achievement={achievement}/>);
    expect(document.querySelector('.achievement-star-tooltip')).toBeNull();
    expect(screen.getByText('1 · 10 · 25 · 100 · 1.000')).toBeInTheDocument();
  });
});

describe('AchievementCard duration formatting', () => {
  const noRush: Achievement = {
    key: 'no_rush',
    metric: 'time_bank_ms_consumed',
    tiers: [
      {stars: 1, threshold: 60_000},
      {stars: 2, threshold: 3_600_000},
      {stars: 3, threshold: 86_400_000},
      {stars: 4, threshold: 604_800_000},
      {stars: 5, threshold: 2_592_000_000},
    ],
  };
  
  test('labels no_rush tiers as durations', () => {
    render(<AchievementCard achievement={noRush}/>);
    expect(screen.getByRole('img')).toHaveAccessibleName(
      /Níveis: 1 minuto · 1 hora · 1 dia · 1 semana · 1 mês/);
  });
  
  test('still renders plain counts for other achievements', () => {
    render(<AchievementCard achievement={{key: 'wins', metric: 'wins', tiers: [{stars: 1, threshold: 1000}]}}/>);
    expect(screen.getByRole('img')).toHaveAccessibleName(/Níveis: 1\.000/);
  });
});
