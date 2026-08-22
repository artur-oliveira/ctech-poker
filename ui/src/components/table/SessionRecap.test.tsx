import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {Page} from '@/lib/api/client';
import type {HandItem} from '@/lib/api/player';
import {SessionRecap} from './SessionRecap';

const mocks = vi.hoisted(() => ({getHands: vi.fn()}));
vi.mock('@/lib/api/player', () => ({getHands: (...args: unknown[]) => mocks.getHands(...args)}));

const hand = (overrides: Partial<HandItem>): HandItem => ({
  pk: 'player', sk: 'hand', table_id: 't1', hand_id: 'hand-1', outcome: 'won', net_change: 0, ended_at: 0,
  ...overrides,
});

const page = (data: HandItem[], hasNext = false, nextCursor: string | null = null): Page<HandItem> => ({
  data, has_next: hasNext, next_cursor: nextCursor, has_previous: false, previous_cursor: null,
});

describe('SessionRecap', () => {
  beforeEach(() => {
    mocks.getHands.mockReset();
  });

  test('renders duration, buy-in, and result without waiting on the hands fetch', () => {
    mocks.getHands.mockReturnValue(new Promise(() => {
    }));
    const joinedAt = Date.now() - 65 * 60_000;
    render(<SessionRecap joinedAt={joinedAt} buyIn={500} finalStack={800} tableId="t1" mode="sandbox"
                          onCloseAction={vi.fn()}/>);
    expect(screen.getByText('Resumo da sessão')).toBeInTheDocument();
    expect(screen.getByText('1h 5min')).toBeInTheDocument();
    expect(screen.getByText('500')).toBeInTheDocument();
    expect(screen.getByText('+300')).toBeInTheDocument();
    expect(screen.queryByText(/Mãos jogadas/)).not.toBeInTheDocument();
  });

  test('shows hands played and biggest pot once the fetch resolves, excluding hands before the session', async () => {
    mocks.getHands.mockResolvedValueOnce(page([
      hand({hand_id: 'a', ended_at: 1_500_000, net_change: 120}),
      hand({hand_id: 'b', ended_at: 1_400_000, net_change: -50}),
      hand({hand_id: 'old', ended_at: 900_000, net_change: 999}),
    ]));
    render(<SessionRecap joinedAt={1_000_000} buyIn={500} finalStack={800} tableId="t1" mode="sandbox"
                          onCloseAction={vi.fn()}/>);
    await waitFor(() => expect(screen.getByText('Mãos jogadas')).toBeInTheDocument());
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('+120')).toBeInTheDocument();
    expect(mocks.getHands).toHaveBeenCalledWith({tableId: 't1', mode: 'sandbox', cursor: undefined});
  });

  test('caps pagination at 150 hands and labels the stat accordingly', async () => {
    const makePage = (prefix: string, cursor: string | null) => page(
      Array.from({length: 50}, (_, i) => hand({hand_id: `${prefix}${i}`, ended_at: 2_000_000 - i, net_change: 10})),
      true, cursor
    );
    mocks.getHands
      .mockResolvedValueOnce(makePage('a', 'c1'))
      .mockResolvedValueOnce(makePage('b', 'c2'))
      .mockResolvedValueOnce(makePage('c', 'c3'));
    render(<SessionRecap joinedAt={0} buyIn={0} finalStack={0} tableId="t1" mode="sandbox" onCloseAction={vi.fn()}/>);
    await waitFor(() => expect(screen.getByText('Mãos jogadas (últimas 150)')).toBeInTheDocument());
    expect(screen.getByText('150')).toBeInTheDocument();
    expect(mocks.getHands).toHaveBeenCalledTimes(3);
  });

  test('omits the biggest-pot stat when no hand in the sample was won', async () => {
    mocks.getHands.mockResolvedValueOnce(page([
      hand({hand_id: 'a', ended_at: 10, net_change: -20}),
      hand({hand_id: 'b', ended_at: 9, net_change: 0}),
    ]));
    render(<SessionRecap joinedAt={0} buyIn={0} finalStack={0} tableId="t1" mode="sandbox" onCloseAction={vi.fn()}/>);
    await waitFor(() => expect(screen.getByText('Mãos jogadas')).toBeInTheDocument());
    expect(screen.queryByText('Maior pote ganho')).not.toBeInTheDocument();
  });

  test('renders the recap with fetch-derived stats absent when getHands rejects', async () => {
    mocks.getHands.mockRejectedValueOnce(new Error('boom'));
    render(<SessionRecap joinedAt={0} buyIn={500} finalStack={500} tableId="t1" mode="sandbox"
                          onCloseAction={vi.fn()}/>);
    expect(screen.getByText('Resumo da sessão')).toBeInTheDocument();
    await waitFor(() => expect(mocks.getHands).toHaveBeenCalled());
    expect(screen.queryByText(/Mãos jogadas/)).not.toBeInTheDocument();
  });

  test('calls onCloseAction from the primary button', async () => {
    mocks.getHands.mockResolvedValueOnce(page([]));
    const onCloseAction = vi.fn();
    const user = userEvent.setup();
    render(<SessionRecap joinedAt={0} buyIn={0} finalStack={0} tableId="t1" mode="sandbox"
                          onCloseAction={onCloseAction}/>);
    await user.click(screen.getByRole('button', {name: 'Voltar ao lobby'}));
    expect(onCloseAction).toHaveBeenCalled();
  });
});
