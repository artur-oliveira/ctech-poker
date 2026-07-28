import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const verifyDeck = vi.hoisted(() => vi.fn());
vi.mock('@/lib/deckVerify', () => ({verifyDeck}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: {card?: string}) => <span>{card ?? 'back'}</span>,
}));

import {DeckReveal} from './DeckReveal';

const deck = Array.from({length: 52}, (_, index) => ({
  rank: 2 + index % 13,
  suit: Math.floor(index / 13),
  code: `card-${index + 1}`,
}));

describe('DeckReveal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('recalculates and lets the player reveal one card or the complete deck', async () => {
    verifyDeck.mockResolvedValue({deck, computedHash: 'same', matches: true});
    render(<DeckReveal serverSeed="seed" commitHash="commit"/>);
    expect(screen.getByText(/Recalculando o baralho/)).toBeInTheDocument();
    expect(await screen.findByText(/baralho não foi alterado/)).toBeInTheDocument();
    expect(verifyDeck).toHaveBeenCalledWith('seed', 'commit');

    await userEvent.click(screen.getByRole('button', {name: 'Posição 1: revelar carta'}));
    expect(screen.getByRole('button', {name: 'Posição 1: revelada'})).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('card-1')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Posição 1: revelada'}));
    expect(screen.getByRole('button', {name: 'Posição 1: revelar carta'})).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', {name: 'Revelar tudo'}));
    expect(screen.getByRole('button', {name: 'Ocultar tudo'})).toBeInTheDocument();
    expect(screen.getAllByText(/^card-/)).toHaveLength(52);
    await userEvent.click(screen.getByRole('button', {name: 'Ocultar tudo'}));
    expect(screen.getAllByText('back')).toHaveLength(52);
  });

  test('shows a truncated computed hash when verification does not match', async () => {
    verifyDeck.mockResolvedValue({
      deck,
      computedHash: '0123456789abcdefghijklmnopqrstuvwxyz98765432',
      matches: false,
    });
    render(<DeckReveal serverSeed="seed" commitHash="wrong"/>);
    expect(await screen.findByText('Hash recalculado não confere: 0123456789…98765432')).toBeInTheDocument();
  });

  test('shows a browser-side failure without rendering a misleading deck', async () => {
    verifyDeck.mockRejectedValue(new Error('WebCrypto unavailable'));
    render(<DeckReveal serverSeed="seed" commitHash="commit"/>);
    expect(await screen.findByText(/Não foi possível recalcular/)).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole('button', {name: 'Revelar tudo'})).not.toBeInTheDocument());
  });
});
