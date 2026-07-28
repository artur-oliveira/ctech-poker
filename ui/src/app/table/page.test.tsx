import {render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {TableSnapshot} from '@/lib/api/table';

const mocks = vi.hoisted(() => ({
  params: new Map<string, string>(),
  push: vi.fn(),
  query: vi.fn(),
  setQueryData: vi.fn(),
  invalidateQueries: vi.fn(),
  realtime: {} as Record<string, unknown>,
  realtimeHook: vi.fn(),
  actionProps: null as Record<string, unknown> | null,
  stageProps: null as Record<string, unknown> | null,
  notification: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: mocks.push}),
  useSearchParams: () => ({get: (key: string) => mocks.params.get(key) ?? null}),
}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({
    setQueryData: mocks.setQueryData,
    invalidateQueries: mocks.invalidateQueries,
  }),
}));
vi.mock('@/lib/utils', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/utils')>(),
  getViewerId: () => 'viewer',
}));
vi.mock('@/lib/hooks/useTableRealtime', () => ({
  useTableRealtime: (...args: unknown[]) => mocks.realtimeHook(...args),
}));
vi.mock('@/lib/tablePreferences', () => ({
  useTablePreferences: () => ({preferences: {theme: 'classic', dealerVoice: false, voiceCommands: false}}),
}));
vi.mock('@/lib/hooks/useDealerVoice', () => ({useDealerVoice: vi.fn()}));
vi.mock('@/lib/notify', () => ({pushNotification: mocks.notification}));
vi.mock('@/lib/mock', () => ({USE_MOCK: false}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: {children: React.ReactNode}) => children}));

vi.mock('@/components/table/BuyInPanel', () => ({
  BuyInPanel: ({roomId, shareCode, onSeatedAction}: {
    roomId: string; shareCode?: string; onSeatedAction: () => void
  }) => <button onClick={onSeatedAction}>buy-in:{roomId}:{shareCode}</button>,
}));
vi.mock('@/components/table/TableStage', () => ({
  TableStage: (props: Record<string, unknown>) => {
    mocks.stageProps = props;
    return <button onClick={() => (props.onEditPlayerNoteAction as (seat: object) => void)(
      {player_id: 'opponent', name: 'Bia'}
    )}>table-stage</button>;
  },
}));
vi.mock('@/components/table/ActionBar', () => ({
  ActionBar: (props: Record<string, unknown>) => {
    mocks.actionProps = props;
    return <button onClick={() => (props.onActAction as (action: string) => void)('check')}>action-bar</button>;
  },
}));
vi.mock('@/components/table/Chat', () => ({
  Chat: ({open, onOpenChange}: {open: boolean; onOpenChange: (open: boolean) => void}) =>
    <button aria-pressed={open} onClick={() => onOpenChange(!open)}>chat</button>,
}));
vi.mock('@/components/table/TableReactions', () => ({
  TableReactions: ({open, onOpenChange}: {open: boolean; onOpenChange: (open: boolean) => void}) =>
    <button aria-pressed={open} onClick={() => onOpenChange(!open)}>reactions</button>,
}));
vi.mock('@/components/table/HandRankingsDialog', () => ({
  HandRankingsDialog: ({open, onOpenChange}: {open: boolean; onOpenChange: (open: boolean) => void}) =>
    <button aria-pressed={open} onClick={() => onOpenChange(!open)}>rankings</button>,
}));
vi.mock('@/components/table/InviteDialog', () => ({
  InviteDialog: ({url}: {url: string}) => <span>invite:{url}</span>,
}));
vi.mock('@/components/table/LeaveDialog', () => ({
  LeaveDialog: ({onLeftAction}: {onLeftAction: (amount: number) => void}) =>
    <button onClick={() => onLeftAction(1234)}>leave</button>,
}));
vi.mock('@/components/table/RebuyDialog', () => ({
  RebuyDialog: ({onRebuyAction}: {onRebuyAction: () => void}) =>
    <button onClick={onRebuyAction}>rebuy</button>,
}));
vi.mock('@/components/table/PlayerNoteDialog', () => ({
  PlayerNoteDialog: ({open}: {open: boolean}) => open ? <span>note-dialog</span> : null,
}));
vi.mock('@/components/table/PerimeterTimer', () => ({PerimeterTimer: () => <span>next-hand-timer</span>}));
vi.mock('@/components/table/TablePreferencesDialog', () => ({TablePreferencesDialog: () => null}));
vi.mock('@/components/table/RealityCheck', () => ({RealityCheck: () => null}));
vi.mock('@/components/table/BotChallenge', () => ({BotChallenge: () => null}));
vi.mock('@/components/table/LastWinners', () => ({LastWinners: () => null}));
vi.mock('@/components/table/MockControls', () => ({MockControls: () => null}));
vi.mock('@/components/AchievementToast', () => ({AchievementToast: () => null}));

import TablePage from './page';

const ROOM_ID = '01ARZ3NDEKTSV4RRFFQ69G5FAV';
const room = {
  room_id: ROOM_ID, visibility: 'public', currency_mode: 'sandbox', small_blind: 10,
  big_blind: 20, max_seats: 6, buy_in_min: 100, buy_in_max: 2000, status: 'open', seats_taken: 2,
};

function snapshot(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
  return {
    stage: 'pre_flop',
    board: [],
    hand_id: 'hand-1',
    protocol_version: 8,
    current_player_id: 'viewer',
    legal_actions: {
      actions: ['fold', 'call', 'raise'], call_amount: 40, min_raise_to: 80,
      max_raise_to: 500, step: 20, half_pot_raise_to: 120, pot_raise_to: 200,
    },
    seats: [
      {
        player_id: 'viewer', name: 'Você', stack: 500, state: 'active', ready: true,
        dealt_in: true, contributed: 20, hole_cards: ['AH', 'KD'], time_bank_ms: 30000,
      },
      {player_id: 'opponent', name: 'Bia', stack: 800, state: 'active', dealt_in: true, contributed: 40},
    ],
    ...overrides,
  };
}

function setQueries({seated = true, loading = false}: {seated?: boolean; loading?: boolean} = {}) {
  mocks.query.mockImplementation(({queryKey}: {queryKey: string[]}) => {
    if (queryKey[0] === 'room') return {data: room};
    if (queryKey[0] === 'seated') return {data: {seated, stack: 500}, isLoading: loading};
    return {data: []};
  });
}

function realtime(overrides: Record<string, unknown> = {}) {
  mocks.realtime = {
    snapshot: snapshot(), snapshotAt: Date.now(), status: 'connected', reconnectAttempt: 0,
    announcement: '', removed: null, retryNow: vi.fn(), act: vi.fn(() => true),
    ready: vi.fn(), readyPending: false, showCards: vi.fn(), showCardsPending: false,
    preselectAction: vi.fn(() => true), pendingAction: null, actionError: null,
    clearActionError: vi.fn(), keepSeat: vi.fn(() => true), chat: [], sendChat: vi.fn(),
    reactions: [], sendReaction: vi.fn(), botChallengeRequired: false, submitBotChallenge: vi.fn(),
    unlock: null, ...overrides,
  };
  mocks.realtimeHook.mockImplementation(() => mocks.realtime);
}

describe('table page integration', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.params = new Map([['id', ROOM_ID]]);
    mocks.actionProps = null;
    mocks.stageProps = null;
    setQueries();
    realtime();
  });

  test('rejects malformed room ids without opening backend or realtime resources', () => {
    mocks.params = new Map([['id', 'not-a-room']]);
    render(<TablePage/>);
    expect(screen.getByRole('heading', {name: 'Mesa inválida'})).toBeInTheDocument();
    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({enabled: false}));
    expect(mocks.realtimeHook).toHaveBeenCalledWith('', 'viewer', undefined, undefined);
  });

  test('waits for seat status and then offers the explicit buy-in ceremony', async () => {
    setQueries({loading: true});
    const view = render(<TablePage/>);
    expect(view.container.querySelector('.loader')).toBeInTheDocument();

    setQueries({seated: false});
    view.rerender(<TablePage/>);
    await userEvent.click(screen.getByRole('button', {name: `buy-in:${ROOM_ID}:`}));
    expect(mocks.setQueryData).toHaveBeenCalledWith(['seated', ROOM_ID], {seated: true, stack: 0});
    expect(mocks.realtimeHook).toHaveBeenLastCalledWith('', 'viewer', undefined, undefined);
  });

  test('shows truthful reconnect states, retry control and the exhausted retry copy', async () => {
    realtime({snapshot: null, status: 'disconnected', reconnectAttempt: 99});
    render(<TablePage/>);
    expect(screen.getByRole('status')).toHaveTextContent('Conexão perdida. Toque para tentar novamente.');
    await userEvent.click(screen.getByRole('button', {name: /Tentar agora/}));
    expect(mocks.realtime.retryNow).toHaveBeenCalledOnce();
  });

  test('maps server legal actions and timing to the action engine', async () => {
    render(<TablePage/>);
    expect(mocks.actionProps).toEqual(expect.objectContaining({
      available: {fold: true, check: false, call: true, raise: true},
      callAmount: 40, minRaise: 80, maxRaise: 500, raiseStep: 20,
      effectiveStack: 500, isTurn: true, timeBankMs: 30000,
      supportsCallPreselection: true, canPreselect: false, connected: true,
    }));
    await userEvent.click(screen.getByRole('button', {name: 'action-bar'}));
    expect(mocks.realtime.act).toHaveBeenCalledWith('check');
  });

  test('never marks an empty current player as the viewer turn and enables preselection when dealt in', () => {
    realtime({snapshot: snapshot({current_player_id: '', stage: 'flop'})});
    render(<TablePage/>);
    expect(mocks.actionProps).toEqual(expect.objectContaining({isTurn: false, canPreselect: true}));
  });

  test('keeps chat, reactions and rankings mutually exclusive', async () => {
    const user = userEvent.setup();
    render(<TablePage/>);
    const chat = screen.getByRole('button', {name: 'chat'});
    const reactions = screen.getByRole('button', {name: 'reactions'});
    const rankings = screen.getByRole('button', {name: 'rankings'});

    await user.click(chat);
    expect(chat).toHaveAttribute('aria-pressed', 'true');
    await user.click(reactions);
    expect(chat).toHaveAttribute('aria-pressed', 'false');
    expect(reactions).toHaveAttribute('aria-pressed', 'true');
    await user.click(rankings);
    expect(reactions).toHaveAttribute('aria-pressed', 'false');
    expect(rankings).toHaveAttribute('aria-pressed', 'true');
  });

  test('handles pause, leave and player notes through their backend callbacks', async () => {
    const user = userEvent.setup();
    render(<TablePage/>);
    await user.click(screen.getByRole('button', {name: 'Sentar fora'}));
    expect(mocks.realtime.ready).toHaveBeenCalledWith(false);

    await user.click(screen.getByRole('button', {name: 'table-stage'}));
    expect(screen.getByText('note-dialog')).toBeInTheDocument();

    await user.click(screen.getByRole('button', {name: 'leave'}));
    expect(mocks.notification).toHaveBeenCalledWith('Você saiu com 1.234 fichas.', 'info');
    expect(mocks.setQueryData).toHaveBeenCalledWith(['seated', ROOM_ID], {seated: false, stack: 0});
    expect(mocks.push).toHaveBeenCalledWith('/lobby');
  });

  test('reacts to server removal by clearing the seat and leaving the frozen table', async () => {
    realtime({removed: {code: 'idle'}});
    render(<TablePage/>);
    await waitFor(() => expect(mocks.push).toHaveBeenCalledWith('/lobby'));
    expect(mocks.notification).toHaveBeenCalledWith('Você foi removido da mesa por inatividade.', 'info');
    expect(mocks.setQueryData).toHaveBeenCalledWith(['seated', ROOM_ID], {seated: false, stack: 0});
  });

  test('builds a fold outcome without claiming the player could win when cards were not revealed', async () => {
    realtime({snapshot: snapshot({
      stage: 'complete',
      board: ['2H', '3D', '4C', '5S', '9H'],
      payouts: {opponent: 100},
      winners: ['opponent'],
      seats: [
        {
          player_id: 'viewer', stack: 480, stack_at_hand_start: 500, state: 'folded', dealt_in: true,
          contributed: 20, hole_cards: ['AH', 'KD'], hand_score: 100,
        },
        {
          player_id: 'opponent', name: 'Bia', stack: 900, state: 'active', dealt_in: true,
          contributed: 40, hole_cards: ['back', 'back'], hand_score: 50,
        },
      ],
    })});
    render(<TablePage/>);
    await waitFor(() => expect(mocks.stageProps?.outcome).toEqual(expect.objectContaining({
      kind: 'fold', couldHaveWon: undefined,
    })));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['hands', ROOM_ID]});
  });
});
