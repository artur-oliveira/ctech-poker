import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {TableSnapshot} from '@/lib/api/table';
import {RabbitHunt} from './RabbitHunt';

const verifyWirePartialDeck = vi.fn();
const verifyDeck = vi.fn();
const rabbitRunout = vi.fn();

vi.mock('@/lib/deckVerify', () => ({
  verifyWirePartialDeck: (...args: unknown[]) => verifyWirePartialDeck(...args),
  verifyDeck: (...args: unknown[]) => verifyDeck(...args),
}));
vi.mock('@/lib/rabbitHunt', () => ({
  rabbitRunout: (...args: unknown[]) => rabbitRunout(...args),
}));
vi.mock('@/components/table/PlayingCard', () => ({
  PlayingCard: ({card}: { card: string }) => <span data-testid="rabbit-card">{card}</span>,
}));

function snapshot(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
  return {
    stage: 'complete',
    board: ['AH', 'KD', 'QS'],
    won_without_showdown: true,
    shuffle_server_seed_hex: 'seed',
    shuffle_commit_hash: 'commit',
    seats: [
      {player_id: 'viewer', stack: 1000, state: 'active', contributed: 0, dealt_in: true},
      {player_id: 'other', stack: 1000, state: 'folded', contributed: 0, dealt_in: true},
    ],
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  verifyDeck.mockResolvedValue({deck: ['deck'], matches: true});
  rabbitRunout.mockReturnValue(['2C', '3D']);
  verifyWirePartialDeck.mockResolvedValue({matches: true});
});

describe('RabbitHunt', () => {
  test.each([
    {stage: 'river'},
    {won_without_showdown: false},
    {board: ['AH', 'KD', 'QS', 'JC', 'TH']},
    {shuffle_server_seed_hex: undefined, shuffle_commit_hash: undefined, root_commit_hash: undefined},
    {seats: [{player_id: 'viewer', stack: 1, state: 'waiting', contributed: 0, dealt_in: false}]},
  ] satisfies Partial<TableSnapshot>[])('stays hidden when rabbit hunting is unavailable %#', overrides => {
    const {container} = render(<RabbitHunt snapshot={snapshot(overrides)} viewer="viewer" bigBlind={50}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test('offers the paid runout while the server still withholds the proof', async () => {
    // How production actually looks the moment a hand is won without showdown:
    // only the commitments are published, and the proof arrives after payment.
    const onRequest = vi.fn();
    const unpaid = snapshot({
      shuffle_server_seed_hex: undefined, shuffle_commit_hash: 'commit', root_commit_hash: 'root',
    });
    const {rerender} = render(<RabbitHunt snapshot={unpaid} viewer="viewer" bigBlind={50}
                                          onRequestRabbitHuntAction={onRequest}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver por 50 fichas/}));
    expect(onRequest).toHaveBeenCalled();
    expect(screen.getByText('Verificando o baralho…')).toBeInTheDocument();

    rerender(<RabbitHunt snapshot={{...unpaid, shuffle_server_seed_hex: 'seed'}} viewer="viewer" bigBlind={50}
                         onRequestRabbitHuntAction={onRequest}/>);
    await waitFor(() => expect(screen.getAllByTestId('rabbit-card')).toHaveLength(2));
    expect(verifyDeck).toHaveBeenCalledWith('seed', 'commit');
  });

  test('derives the remaining runout from the revealed shuffle seed after verifying it', async () => {
    render(<RabbitHunt snapshot={snapshot()} viewer="viewer" bigBlind={50}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver por 50 fichas/}));
    await waitFor(() => expect(screen.getAllByTestId('rabbit-card')).toHaveLength(2));
    expect(verifyDeck).toHaveBeenCalledWith('seed', 'commit');
    expect(rabbitRunout).toHaveBeenCalledWith(['deck'], 2, 3);
  });

  test.each<[string, Partial<TableSnapshot>]>([
    ['missing commit hash', {shuffle_commit_hash: undefined}],
    ['mismatched commit hash', {}],
  ])('reports verification failure for the shuffle seed path: %s', async (label, overrides) => {
    if (label === 'mismatched commit hash') verifyDeck.mockResolvedValue({deck: ['deck'], matches: false});
    render(<RabbitHunt snapshot={snapshot(overrides)} viewer="viewer" bigBlind={50}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver por 50 fichas/}));
    expect(await screen.findByText('Não foi possível verificar o runout. Taxa devolvida.')).toBeInTheDocument();
    expect(rabbitRunout).not.toHaveBeenCalled();
  });

  test('accepts a server runout only after its partial-deck proof matches', async () => {
    const value = snapshot({
      shuffle_server_seed_hex: undefined,
      runout_cards: ['JC', 'TH'],
      root_commit_hash: 'root',
      revealed_card_salts: {0: {card: 'AH', salt_hex: 'salt'}},
      unrevealed_card_hashes: {1: 'hash'},
    });
    render(<RabbitHunt snapshot={value} viewer="viewer" bigBlind={50}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver por 50 fichas/}));
    await waitFor(() => expect(screen.getByText('JC')).toBeInTheDocument());
    expect(verifyWirePartialDeck).toHaveBeenCalledWith(
      'root', value.revealed_card_salts, value.unrevealed_card_hashes,
    );
    expect(verifyDeck).not.toHaveBeenCalled();
  });

  test.each<[string, Partial<TableSnapshot>]>([
    ['missing proof', {runout_cards: ['JC'], shuffle_server_seed_hex: undefined}],
    ['invalid proof', {
      runout_cards: ['JC'], shuffle_server_seed_hex: undefined, root_commit_hash: 'root',
      revealed_card_salts: {}, unrevealed_card_hashes: {},
    }],
  ])('reports verification failure for %s', async (_label, overrides) => {
    verifyWirePartialDeck.mockResolvedValue({matches: false});
    render(<RabbitHunt snapshot={snapshot(overrides)} viewer="viewer" bigBlind={50}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver por 50 fichas/}));
    expect(await screen.findByText('Não foi possível verificar o runout. Taxa devolvida.')).toBeInTheDocument();
  });

  test('requests payment on click and reports a verification failure for a refund', async () => {
    const onRequest = vi.fn();
    const onVerifyFailed = vi.fn();
    verifyDeck.mockResolvedValue({deck: ['deck'], matches: false});
    render(<RabbitHunt snapshot={snapshot()} viewer="viewer" bigBlind={50}
                       onRequestRabbitHuntAction={onRequest} onRabbitHuntVerifyFailedAction={onVerifyFailed}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver por 50 fichas/}));
    expect(onRequest).toHaveBeenCalledOnce();
    expect(await screen.findByText('Não foi possível verificar o runout. Taxa devolvida.')).toBeInTheDocument();
    expect(onVerifyFailed).toHaveBeenCalledOnce();
  });

  test('disables the button while the payment is pending', () => {
    render(<RabbitHunt snapshot={snapshot()} viewer="viewer" bigBlind={50} pending/>);
    expect(screen.getByRole('button', {name: /Ver por 50 fichas/})).toBeDisabled();
  });

  test('brings the button back after the server rejects the payment', async () => {
    const onRequest = vi.fn();
    const {rerender} = render(<RabbitHunt snapshot={snapshot()} viewer="viewer" bigBlind={50}
                                           onRequestRabbitHuntAction={onRequest} failCount={0}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver por 50 fichas/}));
    expect(screen.queryByRole('button', {name: /Ver por 50 fichas/})).not.toBeInTheDocument();

    rerender(<RabbitHunt snapshot={snapshot()} viewer="viewer" bigBlind={50}
                          onRequestRabbitHuntAction={onRequest} failCount={1}/>);
    expect(await screen.findByRole('button', {name: /Ver por 50 fichas/})).toBeInTheDocument();
  });
});
