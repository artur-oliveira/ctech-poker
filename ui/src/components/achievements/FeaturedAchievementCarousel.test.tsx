import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import type {AchievementSummaryEntry} from '@/lib/api/achievements';
import {FeaturedAchievementCarousel, orderedForCarousel} from './FeaturedAchievementCarousel';

vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: {card: string}) => <span data-testid="playing-card">{card}</span>,
}));

function entry(key: string, progress: number, thresholds: number[]): AchievementSummaryEntry {
  return {
    key, metric: key, progress,
    tiers: thresholds.map((threshold, index) => ({stars: index + 1, threshold})),
    stars: 0, unlocked: true, completed: false, next_target: null, max_target: thresholds[thresholds.length - 1],
  };
}

// wins: 3 stars · all_in: 2 stars · bluff: 1 star · tied: 1 star
const entries = [
  entry('bluff', 12, [1, 10, 100]),
  entry('wins', 40, [1, 10, 25, 100]),
  entry('tied', 4, [1, 10, 100]),
  entry('all_in', 30, [1, 25, 500]),
];

function optionNames() {
  return screen.getAllByRole('option').map(option => option.getAttribute('aria-label'));
}

describe('orderedForCarousel', () => {
  test('sorts by stars, then by raw progress, then alphabetically', () => {
    expect(orderedForCarousel(entries, []).map(item => item.key))
      .toEqual(['wins', 'all_in', 'bluff', 'tied']);
  });

  test('leads with the highlighted ones, in the player own order', () => {
    expect(orderedForCarousel(entries, ['tied', 'bluff']).map(item => item.key))
      .toEqual(['tied', 'bluff', 'wins', 'all_in']);
  });

  test('ignores a highlighted key that is not in the list', () => {
    expect(orderedForCarousel(entries, ['gone']).map(item => item.key))
      .toEqual(['wins', 'all_in', 'bluff', 'tied']);
  });
});

describe('FeaturedAchievementCarousel', () => {
  test('is one composite widget, not one tab stop per achievement', () => {
    render(<FeaturedAchievementCarousel entries={entries} selected={[]} max={3} onToggleAction={vi.fn()}/>);

    const listbox = screen.getByRole('listbox', {name: 'Conquistas disponíveis para destaque, no máximo 3'});
    expect(listbox).toHaveAttribute('aria-multiselectable', 'true');
    const options = screen.getAllByRole('option');
    expect(options.map(option => option.getAttribute('tabindex'))).toEqual(['0', '-1', '-1', '-1']);
    expect(options[0]).toHaveAttribute('aria-label', 'Vitórias, 40 registrados, 3 de 4 estrelas');
  });

  test('highlighting an achievement moves it to the front of the rail', async () => {
    const toggle = vi.fn();
    const {rerender} = render(
      <FeaturedAchievementCarousel entries={entries} selected={[]} max={3} onToggleAction={toggle}/>);
    expect(optionNames()[0]).toMatch(/^Vitórias/);

    await userEvent.click(screen.getByRole('option', {name: /^Dividindo o Pote/}));
    expect(toggle).toHaveBeenCalledWith('tied');

    rerender(<FeaturedAchievementCarousel entries={entries} selected={['tied']} max={3} onToggleAction={toggle}/>);
    const options = screen.getAllByRole('option');
    expect(options[0]).toHaveAttribute('aria-label', expect.stringMatching(/^Dividindo o Pote/));
    expect(options[0]).toHaveAttribute('aria-selected', 'true');
    // The rail keeps its place in the tab order on the option the player acted on.
    expect(options[0]).toHaveAttribute('tabindex', '0');
    expect(options[1]).toHaveAttribute('aria-selected', 'false');
  });

  test('moves focus with the arrow keys, Home and End', async () => {
    render(<FeaturedAchievementCarousel entries={entries} selected={[]} max={3} onToggleAction={vi.fn()}/>);
    const options = screen.getAllByRole('option');
    options[0].focus();

    await userEvent.keyboard('{ArrowRight}');
    expect(options[1]).toHaveFocus();
    await userEvent.keyboard('{ArrowLeft}');
    expect(options[0]).toHaveFocus();
    // Clamped at the edges instead of wrapping past them.
    await userEvent.keyboard('{ArrowLeft}');
    expect(options[0]).toHaveFocus();
    await userEvent.keyboard('{End}');
    expect(options[3]).toHaveFocus();
    await userEvent.keyboard('{ArrowRight}');
    expect(options[3]).toHaveFocus();
    await userEvent.keyboard('{Home}');
    expect(options[0]).toHaveFocus();
  });

  test('toggles from the keyboard and leaves other keys to the browser', async () => {
    const toggle = vi.fn();
    render(<FeaturedAchievementCarousel entries={entries} selected={[]} max={3} onToggleAction={toggle}/>);
    screen.getAllByRole('option')[0].focus();

    await userEvent.keyboard('{Enter}');
    await userEvent.keyboard(' ');
    expect(toggle.mock.calls).toEqual([['wins'], ['wins']]);

    await userEvent.keyboard('{Tab}');
    expect(toggle.mock.calls).toHaveLength(2);
  });

  test('scroll buttons page the rail without moving focus', async () => {
    render(<FeaturedAchievementCarousel entries={entries} selected={[]} max={3} onToggleAction={vi.fn()}/>);
    const track = screen.getByRole('listbox');
    const scrollBy = vi.fn();
    Object.defineProperty(track, 'scrollBy', {value: scrollBy, configurable: true});

    await userEvent.click(screen.getByRole('button', {name: 'Rolar conquistas para frente'}));
    await userEvent.click(screen.getByRole('button', {name: 'Rolar conquistas para trás'}));
    expect(scrollBy.mock.calls.map(([options]) => (options as {left: number}).left)).toEqual([240, -240]);
    expect(screen.getAllByRole('option')[0]).not.toHaveFocus();
  });

  test('renders nothing to focus when the player has no scored achievements', () => {
    render(<FeaturedAchievementCarousel entries={[]} selected={[]} max={3} onToggleAction={vi.fn()}/>);
    expect(screen.queryAllByRole('option')).toHaveLength(0);
    expect(screen.getByRole('listbox')).toBeEmptyDOMElement();
  });
});
