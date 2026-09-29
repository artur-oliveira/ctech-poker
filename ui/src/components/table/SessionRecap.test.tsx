import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {SessionRecapData} from '@/lib/api/player';
import {SessionRecap} from './SessionRecap';

const mocks = vi.hoisted(() => ({getSessionRecap: vi.fn()}));
vi.mock('@/lib/api/player', () => ({
  getSessionRecap: (...args: unknown[]) => mocks.getSessionRecap(...args)
}));

const recap = (overrides: Partial<SessionRecapData> = {}): SessionRecapData => ({
  session_id: 's1', table_id: 't1', joined_at: 0, ended_at: 0, duration_ms: 0,
  buyin_amount: 0, cashout_amount: 0, net_pnl: 0, hands_played: 0, hands_won: 0,
  truncated: false, ...overrides,
});

describe('SessionRecap', () => {
  beforeEach(() => {
    mocks.getSessionRecap.mockReset();
  });

  test('renders duration, buy-in, and result without waiting on the recap fetch', () => {
    mocks.getSessionRecap.mockReturnValue(new Promise(() => {
    }));
    const joinedAt = Date.now() - 65 * 60_000;
    render(<SessionRecap sessionId="s1" joinedAt={joinedAt} buyIn={500} finalStack={800} mode="sandbox"
                         onCloseAction={vi.fn()}/>);
    expect(screen.getByText('Resumo da sessão')).toBeInTheDocument();
    expect(screen.getByText('1h 5min')).toBeInTheDocument();
    expect(screen.getByText('500')).toBeInTheDocument();
    expect(screen.getByText('+300')).toBeInTheDocument();
    expect(screen.queryByText(/Mãos jogadas/)).not.toBeInTheDocument();
  });

  // The bug this component shipped with, reported from prod on 2026-09-29:
  // the caller's buy-in comes from a cached `getSessions` page, and the
  // auto-rebuy that fires when a player busts out lands after that cache was
  // filled. Reducing finalStack - staleBuyIn reported a session that lost
  // nothing when it had in fact lost a whole buy-in.
  test('uses the recap buy-in, not the stale cached one, for the result', async () => {
    mocks.getSessionRecap.mockResolvedValueOnce(recap({buyin_amount: 2_000_000}));
    render(<SessionRecap sessionId="s1" joinedAt={0} buyIn={1_000_000} finalStack={1_000_000} mode="sandbox"
                         onCloseAction={vi.fn()}/>);
    // The optimistic render off the stale prop.
    expect(screen.getByText('0')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('-1.000.000')).toBeInTheDocument());
    expect(screen.getByText('2.000.000')).toBeInTheDocument();
  });

  // net_pnl is only written by CloseSession, and the `removed` frame can
  // reach the client before that settlement lands — so an open row's 0 must
  // never be shown as the result.
  test('prefers net_pnl once the session row is closed', async () => {
    mocks.getSessionRecap.mockResolvedValueOnce(
      recap({ended_at: 1_700_000, buyin_amount: 2_000_000, cashout_amount: 1_000_000, net_pnl: -1_000_000})
    );
    render(<SessionRecap sessionId="s1" joinedAt={0} buyIn={1_000_000} finalStack={999} mode="sandbox"
                         onCloseAction={vi.fn()}/>);
    await waitFor(() => expect(screen.getByText('-1.000.000')).toBeInTheDocument());
  });

  test('shows hands played and biggest pot once the fetch resolves', async () => {
    mocks.getSessionRecap.mockResolvedValueOnce(recap({
      buyin_amount: 500, hands_played: 2, hands_won: 1,
      biggest_win: {hand_id: 'a', table_id: 't1', net_change: 120, ended_at: 1_500_000},
    }));
    render(<SessionRecap sessionId="s1" joinedAt={1_000_000} buyIn={500} finalStack={800} mode="sandbox"
                         onCloseAction={vi.fn()}/>);
    await waitFor(() => expect(screen.getByText('Mãos jogadas')).toBeInTheDocument());
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('+120')).toBeInTheDocument();
    expect(mocks.getSessionRecap).toHaveBeenCalledWith('s1', 'sandbox');
  });

  test('labels the stat when the server truncated its scan', async () => {
    mocks.getSessionRecap.mockResolvedValueOnce(recap({hands_played: 200, truncated: true}));
    render(<SessionRecap sessionId="s1" joinedAt={0} buyIn={0} finalStack={0} mode="sandbox"
                         onCloseAction={vi.fn()}/>);
    await waitFor(() => expect(screen.getByText('Mãos jogadas (últimas 200)')).toBeInTheDocument());
    expect(screen.getByText('200')).toBeInTheDocument();
  });

  test('omits the biggest-pot stat when no hand in the session was won', async () => {
    mocks.getSessionRecap.mockResolvedValueOnce(recap({hands_played: 2}));
    render(<SessionRecap sessionId="s1" joinedAt={0} buyIn={0} finalStack={0} mode="sandbox"
                         onCloseAction={vi.fn()}/>);
    await waitFor(() => expect(screen.getByText('Mãos jogadas')).toBeInTheDocument());
    expect(screen.queryByText('Maior pote ganho')).not.toBeInTheDocument();
  });

  test('falls back to the caller figures when the recap fetch rejects', async () => {
    mocks.getSessionRecap.mockRejectedValueOnce(new Error('boom'));
    render(<SessionRecap sessionId="s1" joinedAt={0} buyIn={500} finalStack={800} mode="sandbox"
                         onCloseAction={vi.fn()}/>);
    expect(screen.getByText('Resumo da sessão')).toBeInTheDocument();
    await waitFor(() => expect(mocks.getSessionRecap).toHaveBeenCalled());
    expect(screen.getByText('500')).toBeInTheDocument();
    expect(screen.getByText('+300')).toBeInTheDocument();
    expect(screen.queryByText(/Mãos jogadas/)).not.toBeInTheDocument();
  });

  // A removal that arrives before the sessions query ever resolved has no
  // session id to ask about; the dialog must still render off the props
  // rather than firing a request for `undefined`.
  test('skips the fetch entirely without a session id', () => {
    render(<SessionRecap joinedAt={0} buyIn={500} finalStack={800} mode="sandbox" onCloseAction={vi.fn()}/>);
    expect(screen.getByText('+300')).toBeInTheDocument();
    expect(mocks.getSessionRecap).not.toHaveBeenCalled();
  });

  test('calls onCloseAction from the primary button', async () => {
    mocks.getSessionRecap.mockResolvedValueOnce(recap());
    const onCloseAction = vi.fn();
    const user = userEvent.setup();
    render(<SessionRecap sessionId="s1" joinedAt={0} buyIn={0} finalStack={0} mode="sandbox"
                         onCloseAction={onCloseAction}/>);
    await user.click(screen.getByRole('button', {name: 'Voltar ao lobby'}));
    expect(onCloseAction).toHaveBeenCalled();
  });
});
