import type {ReactNode} from 'react';
import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {ApiError} from '@/lib/api/client';
import type {TableHighlight} from '@/lib/api/highlights';
import {shouldRetryHighlightFetch, TodayHighlight} from './TodayHighlight';

const {getTodayHighlight} = vi.hoisted(() => ({getTodayHighlight: vi.fn()}));
vi.mock('@/lib/api/highlights', () => ({getTodayHighlight}));

function highlight(overrides: Partial<TableHighlight> = {}): TableHighlight {
  return {
    table_id: 't1', date: '2026-08-23', hand_id: 'hand-1', pot: 1500, recorded_at: 0,
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

describe('shouldRetryHighlightFetch', () => {
  test('never retries a 404 (no highlight yet / not your table)', () => {
    expect(shouldRetryHighlightFetch(0, new ApiError('not found', 404))).toBe(false);
  });

  test('retries a transient failure up to the count cap', () => {
    const err = new ApiError('boom', 500);
    expect(shouldRetryHighlightFetch(0, err)).toBe(true);
    expect(shouldRetryHighlightFetch(3, err)).toBe(false);
  });
});

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
    await waitFor(() => expect(screen.getByText('Maior pote de hoje')).toBeInTheDocument());
    expect(screen.getByText('25.000')).toBeInTheDocument();
  });

  test('renders revealed hole cards when present', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      revealed: [{player_id: 'p1', name: 'Alice', hole_cards: ['Ah', 'Kd']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Alice AhKd')).toBeInTheDocument());
  });

  test('falls back to "Jogador" when a revealed hand has no name', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({
      revealed: [{player_id: 'p1', hole_cards: ['2c', '7s']}],
    }));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Jogador 2c7s')).toBeInTheDocument());
  });

  test('omits card text when nothing was revealed', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({revealed: []}));
    renderHighlight();
    await waitFor(() => expect(screen.getByText('Maior pote de hoje')).toBeInTheDocument());
    expect(screen.queryByText(/Ah|Kd/)).not.toBeInTheDocument();
  });

  test('is collapsed by default and expands on click (mobile\'s icon-only badge)', async () => {
    const user = userEvent.setup();
    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 25000}));
    renderHighlight();
    const button = await screen.findByRole('button', {name: /Maior pote de hoje/});
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
    const button = await screen.findByRole('button', {name: /Maior pote de hoje/});

    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'true');

    await user.click(document.body);
    expect(button).toHaveAttribute('aria-expanded', 'false');
  });

  test('closes on Escape', async () => {
    const user = userEvent.setup();
    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 25000}));
    renderHighlight();
    const button = await screen.findByRole('button', {name: /Maior pote de hoje/});

    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'true');

    await user.keyboard('{Escape}');
    expect(button).toHaveAttribute('aria-expanded', 'false');
  });

  test('refetches once a hand this viewer watched completes', async () => {
    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 100}));
    const {rerender} = renderHighlight({handId: 'hand-1', handComplete: false});
    await waitFor(() => expect(screen.getByText('100')).toBeInTheDocument());

    getTodayHighlight.mockResolvedValueOnce(highlight({pot: 900}));
    rerender(<TodayHighlight tableId="t1" handId="hand-1" handComplete/>);
    await waitFor(() => expect(screen.getByText('900')).toBeInTheDocument());
    expect(getTodayHighlight).toHaveBeenCalledTimes(2);
  });
});
