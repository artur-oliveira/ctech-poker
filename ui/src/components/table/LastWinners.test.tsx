import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import type {HandItem} from '@/lib/api/player';
import {deriveWinners, LastWinners} from './LastWinners';

vi.mock('./PlayingCard', () => ({
  PlayingCard: ({card}: {card?: string}) => <span data-testid={card ? 'card' : 'card-back'}>{card || '?'}</span>,
}));

const hand = (overrides: Partial<HandItem>): HandItem => ({
  pk: 'player',
  sk: 'hand',
  table_id: 'table-1',
  hand_id: 'hand-1',
  outcome: 'won',
  net_change: 100,
  ended_at: 1,
  ...overrides,
});

describe('deriveWinners', () => {
  test('derives the viewer winning combination and category from seven cards', () => {
    const [winner] = deriveWinners([hand({
      hole_cards: ['AH', 'AD'],
      board: ['AC', 'KD', 'KS', '2C', '3D'],
    })]);
    expect(winner.names).toEqual(['Você']);
    expect(winner.category).toBe('full_house');
    expect(winner.cards).toHaveLength(5);
  });

  test('lists split-pot opponents, fallback names, and uses revealed opponent cards', () => {
    const [winner] = deriveWinners([hand({
      outcome: 'lost',
      board: ['2H', '3D', '4S', '9C', 'KD'],
      opponents: [
        {player_id: 'p2', name: 'Bia', won: true, hole_cards: ['5H', '6H']},
        {player_id: 'p3', won: true},
        {player_id: 'p4', name: 'Não venceu', won: false},
      ],
    })]);
    expect(winner.names).toEqual(['Bia', 'Visitante']);
    expect(winner.category).toBe('straight');
  });

  test('filters losses without known winners and applies the limit before filtering', () => {
    const entries = deriveWinners([
      hand({hand_id: 'lost', outcome: 'lost'}),
      hand({hand_id: 'won'}),
      hand({hand_id: 'later'}),
    ], 2);
    expect(entries.map(entry => entry.key)).toEqual(['won']);
  });
});

describe('LastWinners', () => {
  test('renders nothing without a known winner', () => {
    const {container} = render(<LastWinners items={[hand({outcome: 'lost'})]}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test('opens and closes the panel, rendering known cards and concealed placeholders', async () => {
    const user = userEvent.setup();
    render(<LastWinners items={[
      hand({hand_id: 'visible', hole_cards: ['AH', 'AD']}),
      hand({hand_id: 'hidden', outcome: 'tied'}),
    ]}/>);

    const toggle = screen.getByRole('button', {name: 'Ver últimos vencedores'});
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getAllByTestId('card')).toHaveLength(2);
    expect(screen.getAllByTestId('card-back')).toHaveLength(2);

    await user.click(toggle);
    expect(screen.getByRole('button', {name: 'Fechar últimos vencedores'})).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getAllByText('Você')).toHaveLength(2);

    await user.click(screen.getByRole('button', {name: 'Fechar últimos vencedores'}));
    expect(screen.getByRole('button', {name: 'Ver últimos vencedores'})).toHaveAttribute('aria-expanded', 'false');
  });
});
