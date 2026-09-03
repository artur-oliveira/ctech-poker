import {render, screen} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {RecentUnlocksRail} from './RecentUnlocksRail';

const unlock = (key: string, stars = 2, agoMs = 3600_000) =>
  ({key, stars, unlockedAtMs: Date.now() - agoMs});

describe('RecentUnlocksRail (#119)', () => {
  test('renders nothing at all when the player has no dated unlocks', () => {
    const {container} = render(<RecentUnlocksRail unlocks={[]}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test('lists the unlocks with their star count and a relative date', () => {
    render(<RecentUnlocksRail unlocks={[unlock('wins', 1), unlock('hands_played', 3)]}/>);
    expect(screen.getByRole('heading', {name: /Recém-desbloqueadas/})).toBeInTheDocument();
    expect(screen.getByText('1 estrela')).toBeInTheDocument();
    expect(screen.getByText('3 estrelas')).toBeInTheDocument();
    expect(screen.getAllByText('há 1 hora')).toHaveLength(2);
    // No arrival: no celebration note and no gold ring on any row.
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.getAllByRole('listitem').some(li => li.className.includes('is-celebrating'))).toBe(false);
  });

  test('celebrates the unlock the player just earned at the table, once', () => {
    render(<RecentUnlocksRail unlocks={[unlock('wins'), unlock('hands_played')]} celebrating="wins"/>);
    expect(screen.getByRole('status')).toHaveTextContent(/entrou na sua coleção agora/);
    const celebrating = screen.getAllByRole('listitem').filter(li => li.className.includes('is-celebrating'));
    expect(celebrating).toHaveLength(1);
  });

  test('a pending key that is not on the rail gets no note and no ring', () => {
    // The unlock can be older than the five rows the rail shows, or belong to
    // the other wallet mode. Announcing it anyway would name a card the player
    // cannot see.
    render(<RecentUnlocksRail unlocks={[unlock('wins')]} celebrating="something_else"/>);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.getAllByRole('listitem').some(li => li.className.includes('is-celebrating'))).toBe(false);
  });
});
