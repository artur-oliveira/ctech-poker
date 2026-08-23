import {describe, expect, test, vi, beforeEach} from 'vitest';
import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactElement} from 'react';
import {RevealWinnerButton} from './RevealWinnerButton';
import * as playerApi from '@/lib/api/player';

vi.mock('@/lib/api/player', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/player')>('@/lib/api/player');
  return {...actual, getHandRevealWinner: vi.fn(), revealHandWinner: vi.fn()};
});

function renderWithClient(ui: ReactElement) {
  const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

describe('RevealWinnerButton', () => {
  beforeEach(() => vi.clearAllMocks());

  test('renders nothing while the check is loading or on 404', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockRejectedValue({response: {status: 404}});
    const onRevealed = vi.fn();
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false}
                                          onRevealedAction={onRevealed}/>);
    await waitFor(() => expect(playerApi.getHandRevealWinner).toHaveBeenCalledWith('hand-1'));
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('renders nothing when alreadyRevealed is true', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: false});
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed
                                          onRevealedAction={vi.fn()}/>);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('shows the buy button with the fee once the check resolves as unpaid', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: false});
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false}
                                          onRevealedAction={vi.fn()}/>);
    expect(await screen.findByRole('button')).toHaveTextContent('200');
  });

  test('clicking the button purchases the reveal and surfaces the cards', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: false});
    vi.mocked(playerApi.revealHandWinner).mockResolvedValue({cards: ['Ah', 'Kd']});
    const onRevealed = vi.fn();
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false}
                                          onRevealedAction={onRevealed}/>);
    const button = await screen.findByRole('button');
    await userEvent.click(button);
    await waitFor(() => expect(onRevealed).toHaveBeenCalledWith(['Ah', 'Kd']));
  });

  test('surfaces cards automatically when the check reports already_paid', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: true, cards: ['2c', '7s']});
    const onRevealed = vi.fn();
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false}
                                          onRevealedAction={onRevealed}/>);
    await waitFor(() => expect(onRevealed).toHaveBeenCalledWith(['2c', '7s']));
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('disables the button while the purchase is in flight', async () => {
    vi.mocked(playerApi.getHandRevealWinner).mockResolvedValue({fee: 200, already_paid: false});
    let resolvePurchase: (v: { cards: [string, string] }) => void = () => {};
    vi.mocked(playerApi.revealHandWinner).mockReturnValue(new Promise(resolve => {
      resolvePurchase = resolve;
    }));
    renderWithClient(<RevealWinnerButton handId="hand-1" winnerName="Ana" alreadyRevealed={false}
                                          onRevealedAction={vi.fn()}/>);
    const button = await screen.findByRole('button');
    userEvent.click(button);
    await waitFor(() => expect(button).toBeDisabled());
    resolvePurchase({cards: ['Ah', 'Kd']});
  });
});
