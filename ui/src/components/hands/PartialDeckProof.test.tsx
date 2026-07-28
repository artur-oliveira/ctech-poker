import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';

const verifyWirePartialDeck = vi.hoisted(() => vi.fn());
vi.mock('@/lib/deckVerify', () => ({verifyWirePartialDeck}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: {card?: string}) => <span>{card ?? 'back'}</span>,
}));

import {PartialDeckProof} from './PartialDeckProof';

const revealed = {0: {card: 'AH', salt_hex: 'aa'}, 5: {card: 'KD', salt_hex: 'bb'}};
const unrevealed = Object.fromEntries(
  Array.from({length: 52}, (_, i) => i).filter(i => i !== 0 && i !== 5).map(i => [i, `hash-${i}`])
);

describe('PartialDeckProof', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('verifies the seed-less proof and only flips positions the viewer may see', async () => {
    verifyWirePartialDeck.mockResolvedValue({rootCommit: 'root', matches: true});
    render(<PartialDeckProof rootCommitHash="root" revealed={revealed} unrevealed={unrevealed}/>);
    expect(screen.getByText(/Recalculando os hashes/)).toBeInTheDocument();
    expect(await screen.findByText(/baralho não foi alterado/)).toBeInTheDocument();
    expect(verifyWirePartialDeck).toHaveBeenCalledWith('root', revealed, unrevealed);

    await userEvent.click(screen.getByRole('button', {name: 'Posição 1: revelar carta'}));
    expect(screen.getByText('AH')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', {name: 'Revelar tudo'}));
    expect(screen.getByText('AH')).toBeInTheDocument();
    expect(screen.getByText('KD')).toBeInTheDocument();
    // The other 50 stay committed hashes — "revelar tudo" must never turn a
    // mucked card face-up just because the proof covers its position.
    expect(screen.getAllByText('back')).toHaveLength(50);
  });

  test('reports a mismatching proof instead of implying the deck is sound', async () => {
    verifyWirePartialDeck.mockResolvedValue({rootCommit: 'other', matches: false});
    render(<PartialDeckProof rootCommitHash="root" revealed={revealed} unrevealed={unrevealed}/>);
    expect(await screen.findByText(/não conferem com o root commit/)).toBeInTheDocument();
  });

  test('shows a browser-side failure instead of a silent pass', async () => {
    verifyWirePartialDeck.mockRejectedValue(new Error('WebCrypto unavailable'));
    render(<PartialDeckProof rootCommitHash="root" revealed={revealed} unrevealed={unrevealed}/>);
    expect(await screen.findByText(/Não foi possível verificar/)).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText(/baralho não foi alterado/)).not.toBeInTheDocument());
  });
});
