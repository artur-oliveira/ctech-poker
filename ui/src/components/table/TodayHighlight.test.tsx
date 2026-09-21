import type {ReactNode} from 'react';
import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ApiError} from '@/lib/api/client';
import type {TableHighlight} from '@/lib/api/highlights';
import {TodayHighlight} from './TodayHighlight';

const {getTodayHighlight} = vi.hoisted(() => ({getTodayHighlight: vi.fn()}));
vi.mock('@/lib/api/highlights', () => ({getTodayHighlight}));

function highlight(overrides: Partial<TableHighlight> = {}): TableHighlight {
  return {
    table_id: 't1', date: '2026-08-23', hand_id: 'hand-1', pot: 1500, recorded_at: 0,
    board: ['Ac', '7d', '2s', '9h', '3c'],
    ...overrides,
  };
}

let client: QueryClient;

function wrapper({children}: { children: ReactNode }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

function renderHighlight(props: Partial<React.ComponentProps<typeof TodayHighlight>> = {}) {
  return render(<TodayHighlight tableId="t1" handComplete={false} {...props}/>, {wrapper});
}

describe('TodayHighlight', () => {
  beforeEach(() => {
    getTodayHighlight.mockReset();
    client = new QueryClient({defaultOptions: {queries: {retry: false}}});
  });

  test('renders nothing while no highlight exists (404)', async () => {
    getTodayHighlight.mockRejectedValueOnce(new ApiError('not found', 404));
    const {container} = renderHighlight();
    await waitFor(() => expect(getTodayHighlight).toHaveBeenCalledWith('t1'));
    expect(container).toBeEmptyDOMElement();
  });

  test('renders the pot amount once a highlight is recorded', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 25000}));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Maior pote disputado hoje')).toBeInTheDocument());
    expect(screen.getByText('25.000')).toBeInTheDocument();
  });

  test('names the winner and made hand without exposing card codes', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      revealed: [{player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Kd']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice · Par')).toBeInTheDocument());
    expect(screen.queryByText(/AhKd/)).not.toBeInTheDocument();
  });

  test('falls back to "Jogador" when a revealed hand has no name', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      revealed: [{player_id: 'p1', hole_cards: ['2c', '7s']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Jogador · Dois pares')).toBeInTheDocument());
  });

  test('joins tied winners and names their shared made hand', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      board: ['Ah', 'Kd', 'Qs', 'Jc', 'Th'],
      revealed: [
        {player_id: 'p1', name: 'Alice', hole_cards: ['2c', '3d']},
        {player_id: 'p2', name: 'Bia', hole_cards: ['4c', '5d']},
      ],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice e Bia · Sequência')).toBeInTheDocument());
  });

  test('selects a later revealed player when their hand beats the first candidate', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      revealed: [
        {player_id: 'p1', name: 'Alice', hole_cards: ['Kh', 'Qd']},
        {player_id: 'p2', name: 'Bia', hole_cards: ['Ah', 'Ad']},
      ],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Bia · Trinca')).toBeInTheDocument());
    expect(screen.queryByText(/Alice/)).not.toBeInTheDocument();
  });

  test('omits the hand label when card data is incomplete or invalid', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      board: ['Ac', '7d', '2s'],
      revealed: [{player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Kd']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Maior pote disputado hoje')).toBeInTheDocument());
    expect(screen.queryByText(/Alice/)).not.toBeInTheDocument();
  });

  test('omits card text when nothing was revealed', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({revealed: []}));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Maior pote disputado hoje')).toBeInTheDocument());
    expect(screen.queryByText(/Ah|Kd/)).not.toBeInTheDocument();
  });

  test('names the winner of a hand that had no showdown', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      pot: 150875,
      board: ['As', '3s', '2s', '8h', 'Ac'],
      winners: [{player_id: 'p1', name: 'Artur 1234', payout: 150875}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Artur 1234')).toBeInTheDocument());
  });

  test('appends the made hand when the winner also showed their cards', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      board: ['Ac', '7d', '2s', '9h', '3c'],
      winners: [{player_id: 'p1', name: 'Alice', payout: 1500}],
      revealed: [{player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Kd']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice · Par')).toBeInTheDocument());
  });

  // #296: cross-player "best shown hand" comparisons must rank by this
  // table's own variant. Alice makes a flush, Bob a full house, off the same
  // board: short-deck ranks Alice's flush above Bob's full house (the
  // inverse of standard hold'em), so passing the wrong variant would credit
  // the wrong player's hand as "the best shown".
  test('ranks a cross-player comparison by the short-deck variant (flush beats full house)', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      board: ['Tc', 'Qc', 'Ac', 'Qd', 'Qh'],
      revealed: [
        {player_id: 'p1', name: 'Bob', hole_cards: ['Kc', 'Kd']},
        {player_id: 'p2', name: 'Alice', hole_cards: ['6c', '8c']},
      ],
    }));
    renderHighlight({variant: 'short_deck'});
    await waitFor(() => expect(screen.getByText('Alice · Flush')).toBeInTheDocument());
    expect(screen.queryByText(/Bob/)).not.toBeInTheDocument();
  });

  test('the same board ranks the full house above the flush under standard (sanity check)', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      board: ['Tc', 'Qc', 'Ac', 'Qd', 'Qh'],
      revealed: [
        {player_id: 'p1', name: 'Bob', hole_cards: ['Kc', 'Kd']},
        {player_id: 'p2', name: 'Alice', hole_cards: ['6c', '8c']},
      ],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Bob · Full house')).toBeInTheDocument());
    expect(screen.queryByText(/Alice/)).not.toBeInTheDocument();
  });

  test('names the paid winner, not the best hand shown, on a side pot', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      board: ['Ac', '7d', '2s', '9h', '3c'],
      winners: [{player_id: 'p2', name: 'Bob', payout: 9000}],
      revealed: [
        {player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Ad']},
        {player_id: 'p2', name: 'Bob', hole_cards: ['9c', '9s']},
      ],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText(/^Bob/)).toBeInTheDocument());
    expect(screen.queryByText(/Alice/)).not.toBeInTheDocument();
  });

  test('joins every winner of a split pot', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      winners: [
        {player_id: 'p1', name: 'Alice', payout: 750},
        {player_id: 'p2', name: 'Bob', payout: 750},
      ],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice e Bob')).toBeInTheDocument());
  });

  test('still reads the revealed hands on a row written before winners existed', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      revealed: [{player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Kd']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice · Par')).toBeInTheDocument());
  });

  test('is collapsed by default and expands on click (mobile\'s icon-only badge)', async () => {
    const user = userEvent.setup();
    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 25000}));
    renderHighlight();
    const button = await screen.findByRole('button', {name: /Maior pote disputado hoje/});
    expect(button).toHaveAttribute('aria-expanded', 'false');

    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'true');

    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'false');
  });

  test('closes when a click lands outside the badge', async () => {
    const user = userEvent.setup();
    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 25000}));
    renderHighlight();
    const button = await screen.findByRole('button', {name: /Maior pote disputado hoje/});

    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'true');

    await user.click(document.body);
    expect(button).toHaveAttribute('aria-expanded', 'false');
  });

  test('closes on Escape', async () => {
    const user = userEvent.setup();
    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 25000}));
    renderHighlight();
    const button = await screen.findByRole('button', {name: /Maior pote disputado hoje/});

    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'true');

    await user.keyboard('{Escape}');
    expect(button).toHaveAttribute('aria-expanded', 'false');
  });

  test('refetches once a hand this viewer watched completes', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 100}));
    const {rerender} = renderHighlight({handId: 'hand-2', handComplete: false});
    await waitFor(() => expect(screen.getByText('100')).toBeInTheDocument());

    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 900}));
    rerender(<TodayHighlight tableId="t1" handId="hand-2" handComplete/>);
    await waitFor(() => expect(screen.getByText('900')).toBeInTheDocument());
    expect(getTodayHighlight).toHaveBeenCalledTimes(2);
  });

  test('spends no read when the highlight on display is already this hand', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({hand_id: 'hand-1', pot: 100}));
    const {rerender} = renderHighlight({handId: 'hand-1', handComplete: false});
    await waitFor(() => expect(screen.getByText('100')).toBeInTheDocument());

    rerender(<TodayHighlight tableId="t1" handId="hand-1" handComplete/>);
    await waitFor(() => expect(screen.getByText('100')).toBeInTheDocument());
    expect(getTodayHighlight).toHaveBeenCalledTimes(1);
  });

  test('spends no read when the settled pot cannot beat the one on record', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({hand_id: 'hand-1', pot: 5000}));
    const {rerender} = renderHighlight({handId: 'hand-2', handComplete: false});
    await waitFor(() => expect(screen.getByText('5.000')).toBeInTheDocument());

    rerender(<TodayHighlight tableId="t1" handId="hand-2" handPot={400} handComplete/>);
    await waitFor(() => expect(screen.getByText('5.000')).toBeInTheDocument());
    expect(getTodayHighlight).toHaveBeenCalledTimes(1);
  });

  test('keeps re-checking after completion so a late-written highlight still lands', async () => {
    vi.useFakeTimers();
    try {
      // Server hasn't written the row yet at completion: the immediate refetch
      // returns the stale pot, a backoff refetch picks up the real one.
      getTodayHighlight.mockResolvedValueOnce(highlight({pot: 100}));
      const {rerender} = renderHighlight({handId: 'hand-2', handComplete: false});
      await vi.waitFor(() => expect(screen.getByText('100')).toBeInTheDocument());

      getTodayHighlight.mockResolvedValueOnce(highlight({pot: 100}));
      getTodayHighlight.mockResolvedValueOnce(highlight({hand_id: 'hand-2', pot: 4200}));
      rerender(<TodayHighlight tableId="t1" handId="hand-2" handComplete/>);

      await vi.advanceTimersByTimeAsync(2000);
      await vi.waitFor(() => expect(screen.getByText('4.200')).toBeInTheDocument());
      expect(getTodayHighlight).toHaveBeenCalledTimes(3);
    } finally {
      vi.useRealTimers();
    }
  });
});
