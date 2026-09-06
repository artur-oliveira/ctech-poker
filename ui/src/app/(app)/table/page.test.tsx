import {act, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {expectNoAxeViolations} from '@/test/axe';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import type {TableSnapshot} from '@/lib/api/table';
import TablePage from './page';

const mocks = vi.hoisted(() => ({
  params: new Map<string, string>(),
  push: vi.fn(),
  query: vi.fn(),
  setQueryData: vi.fn(),
  invalidateQueries: vi.fn(),
  replace: vi.fn(),
  realtime: {} as Record<string, unknown>,
  realtimeHook: vi.fn(),
  actionProps: null as Record<string, unknown> | null,
  stageProps: null as Record<string, unknown> | null,
  reactionProps: null as Record<string, unknown> | null,
  purchaseProps: null as Record<string, unknown> | null,
  noteProps: null as Record<string, unknown> | null,
  inviteProps: null as Record<string, unknown> | null,
  preferencesProps: null as Record<string, unknown> | null,
  notification: vi.fn(),
  updateMe: vi.fn(),
  blockPlayer: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: mocks.push, replace: mocks.replace}),
  useSearchParams: () => ({get: (key: string) => mocks.params.get(key) ?? null}),
}));
vi.mock('@tanstack/react-query', () => ({
  useQuery: mocks.query,
  useQueryClient: () => ({
    setQueryData: mocks.setQueryData,
    invalidateQueries: mocks.invalidateQueries,
    // The post-hand retry asks the cache whether the projection already
    // landed; nothing is cached in this suite, so every read is spent.
    getQueryData: () => undefined,
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
vi.mock('@/lib/api/social', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/social')>(),
  getRelationships: vi.fn().mockResolvedValue([]),
  blockPlayer: (...args: unknown[]) => mocks.blockPlayer(...args),
}));
vi.mock('@/lib/mockConfig', () => ({USE_MOCK: false}));
vi.mock('@/components/TermsGate', () => ({TermsGate: ({children}: { children: React.ReactNode }) => children}));

vi.mock('@/components/table/BuyInPanel', () => ({
  BuyInPanel: ({roomId, bucket, shareCode, onSeatedAction}: {
    roomId?: string;
    bucket?: {smallBlind: number; bigBlind: number; maxSeats: number};
    shareCode?: string;
    onSeatedAction: (roomId: string) => void
  }) => bucket
    ? <button onClick={() => onSeatedAction('01ARZ3NDEKTSV4RRFFQ69G5FAV')}>
      buy-in-bucket:{bucket.smallBlind}/{bucket.bigBlind}/{bucket.maxSeats}
    </button>
    : <button onClick={() => onSeatedAction(roomId ?? '')}>buy-in:{roomId}:{shareCode}</button>,
}));
vi.mock('@/components/table/TableStage', () => ({
  TableStage: (props: Record<string, unknown>) => {
    mocks.stageProps = props;
    const renderActions = props.renderPlayerActionsAction as ((seat: object) => React.ReactNode) | undefined;
    return <>
      <button onClick={() => (props.onEditPlayerNoteAction as (seat: object) => void)(
        {player_id: 'opponent', name: 'Bia'}
      )}>table-stage</button>
      {renderActions?.({player_id: 'opponent', name: 'Bia'})}
    </>;
  },
  STAGE_LABELS: {
    waiting_for_players: 'Aguardando jogadores', pre_flop: 'Pré-flop', flop: 'Flop', turn: 'Turn', river: 'River',
    showdown: 'Showdown', complete: 'Mão encerrada'
  },
}));
vi.mock('@/components/table/ActionBar', () => ({
  ActionBar: (props: Record<string, unknown>) => {
    mocks.actionProps = props;
    return <button onClick={() => (props.onActAction as (action: string) => void)('check')}>action-bar</button>;
  },
}));
vi.mock('@/components/table/Chat', () => ({
  Chat: ({open, onOpenChangeAction}: { open: boolean; onOpenChangeAction: (open: boolean) => void }) =>
    <button aria-pressed={open} onClick={() => onOpenChangeAction(!open)}>chat</button>,
}));
vi.mock('@/components/table/TableReactions', () => ({
  TableReactions: (props: Record<string, unknown>) => {
    mocks.reactionProps = props;
    return <button aria-pressed={Boolean(props.open)}
                   onClick={() => (props.onOpenChangeAction as (open: boolean) => void)(!props.open)}>reactions</button>;
  },
}));
vi.mock('@/components/table/HandRankingsDialog', () => ({
  HandRankingsDialog: ({open, onOpenChangeAction}: { open: boolean; onOpenChangeAction: (open: boolean) => void }) =>
    <button aria-pressed={open} onClick={() => onOpenChangeAction(!open)}>rankings</button>,
}));
vi.mock('@/components/table/InviteDialog', () => ({
  InviteDialog: (props: Record<string, unknown>) => {
    mocks.inviteProps = props;
    return <span>invite:{String(props.url)}</span>;
  },
}));
vi.mock('@/components/table/LeaveDialog', () => ({
  LeaveDialog: ({onRequestExitAction}: { onRequestExitAction: () => boolean }) =>
    <button onClick={() => onRequestExitAction()}>leave</button>,
}));
vi.mock('@/components/table/RebuyDialog', () => ({
  RebuyDialog: ({onRebuyAction}: { onRebuyAction: () => void }) =>
    <button onClick={onRebuyAction}>rebuy</button>,
}));
vi.mock('@/components/table/PlayerNoteDialog', () => ({
  PlayerNoteDialog: (props: Record<string, unknown>) => {
    mocks.noteProps = props;
    return props.open ? <span>note-dialog</span> : null;
  },
}));
vi.mock('@/components/table/PerimeterTimer', () => ({PerimeterTimer: () => <span>next-hand-timer</span>}));
vi.mock('@/components/table/TablePreferencesDialog', () => ({
  TablePreferencesDialog: (props: Record<string, unknown>) => {
    mocks.preferencesProps = props;
    return null;
  },
}));
vi.mock('@/components/table/RealityCheck', () => ({RealityCheck: () => null}));
vi.mock('@/components/table/SessionRecap', () => ({
  SessionRecap: ({onCloseAction}: { onCloseAction: () => void }) =>
    <button onClick={onCloseAction}>close-recap</button>,
}));
vi.mock('@/components/table/BotChallenge', () => ({BotChallenge: () => null}));
vi.mock('@/components/table/LastWinners', () => ({
  LastWinners: ({open, onOpenChangeAction}: { open: boolean; onOpenChangeAction: (open: boolean) => void }) =>
    <button aria-pressed={open} onClick={() => onOpenChangeAction(!open)}>winners</button>,
}));
vi.mock('@/components/table/MockControls', () => ({MockControls: () => null}));
vi.mock('@/components/AchievementToast', () => ({AchievementToast: () => null}));
vi.mock('@/components/reactions/ReactionPurchaseDialog', () => ({
  ReactionPurchaseDialog: (props: Record<string, unknown>) => {
    mocks.purchaseProps = props;
    return props.entry ? <button onClick={() => {
      (props.onConfirmedAction as () => void)();
      (props.onCloseAction as () => void)();
    }}>purchase-dialog</button> : null;
  },
}));
vi.mock('@/lib/api/player', () => ({
  getHands: vi.fn(), getMe: vi.fn(), getSessions: vi.fn(), updateMe: mocks.updateMe,
}));

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

function setQueries({seated = true, loading = false, data = {}, roomData = room}: {
  seated?: boolean; loading?: boolean; data?: Record<string, unknown>; roomData?: Record<string, unknown>;
} = {}) {
  mocks.query.mockImplementation(({queryKey}: { queryKey: string[] }) => {
    if (queryKey[0] === 'room') return {data: roomData};
    if (queryKey[0] === 'seated') return {data: {seated, stack: 500}, isLoading: loading};
    const key = queryKey.join(':');
    if (key in data) return {data: data[key], isLoading: false};
    return {data: []};
  });
}

function realtime(overrides: Record<string, unknown> = {}) {
  mocks.realtime = {
    snapshot: snapshot(), snapshotAt: Date.now(), status: 'connected', reconnectAttempt: 0,
    announcement: '', removed: null, retryNow: vi.fn(), act: vi.fn(() => true),
    ready: vi.fn(), readyPending: false, showCards: vi.fn(), showCardsPending: false,
    requestRabbitHunt: vi.fn(() => true), requestRabbitHuntPending: false,
    requestWinnerCards: vi.fn(() => true), requestWinnerCardsPending: false,
    reportRabbitHuntVerifyFailed: vi.fn(() => true),
    requestExit: vi.fn(() => true), requestExitPending: false, cancelExit: vi.fn(() => true),
    preselectAction: vi.fn(() => true), pendingAction: null, actionError: null,
    clearActionError: vi.fn(), keepSeat: vi.fn(() => true), chat: [], sendChat: vi.fn(),
    reactions: [], sendReaction: vi.fn(), botChallengeRequired: false, submitBotChallenge: vi.fn(),
    unlock: null, clearUnlock: vi.fn(), migrating: '', ...overrides,
  };
  mocks.realtimeHook.mockImplementation(() => mocks.realtime);
}

describe('table page integration', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.params = new Map([['id', ROOM_ID]]);
    mocks.actionProps = null;
    mocks.stageProps = null;
    mocks.reactionProps = null;
    setQueries();
    realtime();
  });
  
  test('rejects malformed room ids without opening backend or realtime resources', () => {
    mocks.params = new Map([['id', 'not-a-room']]);
    render(<TablePage/>);
    expect(screen.getByRole('heading', {name: 'Mesa inválida'})).toBeInTheDocument();
    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({enabled: false}));
    expect(mocks.realtimeHook).toHaveBeenCalledWith('', 'viewer', undefined, undefined, new Set());
  });
  
  // A lobby pick carries a bucket, not a room id: the ceremony confirms it
  // with join-or-create, which is what decides the table (#205).
  test('opens the buy-in ceremony for a bucket entry and adopts the resolved table', async () => {
    mocks.params = new Map([['sb', '25'], ['bb', '50'], ['seats', '6']]);
    render(<TablePage/>);

    expect(mocks.query).toHaveBeenCalledWith(expect.objectContaining({enabled: false}));
    await userEvent.click(screen.getByRole('button', {name: 'buy-in-bucket:25/50/6'}));

    expect(mocks.setQueryData).toHaveBeenCalledWith(
      ['seated', '01ARZ3NDEKTSV4RRFFQ69G5FAV'], {seated: true, stack: 0});
    expect(mocks.replace).toHaveBeenCalledWith('/table?id=01ARZ3NDEKTSV4RRFFQ69G5FAV');
  });

  test('rejects an incomplete bucket entry the same way it rejects a bad id', () => {
    mocks.params = new Map([['sb', '25'], ['bb', '50']]);
    render(<TablePage/>);
    expect(screen.getByRole('heading', {name: 'Mesa inválida'})).toBeInTheDocument();
  });

  test('waits for seat status and then offers the explicit buy-in ceremony', async () => {
    setQueries({loading: true});
    const view = render(<TablePage/>);
    expect(view.container.querySelector('.loader')).toBeInTheDocument();
    
    setQueries({seated: false});
    view.rerender(<TablePage/>);
    await userEvent.click(screen.getByRole('button', {name: `buy-in:${ROOM_ID}:`}));
    expect(mocks.setQueryData).toHaveBeenCalledWith(['seated', ROOM_ID], {seated: true, stack: 0});
    expect(mocks.realtimeHook).toHaveBeenLastCalledWith('', 'viewer', undefined, undefined, new Set());
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
  
  test('keeps chat, reactions, rankings and recent winners mutually exclusive', async () => {
    const user = userEvent.setup();
    render(<TablePage/>);
    const chat = screen.getByRole('button', {name: 'chat'});
    const reactions = screen.getByRole('button', {name: 'reactions'});
    const rankings = screen.getByRole('button', {name: 'rankings'});
    const winners = screen.getByRole('button', {name: 'winners'});
    
    await user.click(chat);
    expect(chat).toHaveAttribute('aria-pressed', 'true');
    await user.click(reactions);
    expect(chat).toHaveAttribute('aria-pressed', 'false');
    expect(reactions).toHaveAttribute('aria-pressed', 'true');
    await user.click(rankings);
    expect(reactions).toHaveAttribute('aria-pressed', 'false');
    expect(rankings).toHaveAttribute('aria-pressed', 'true');
    await user.click(winners);
    expect(rankings).toHaveAttribute('aria-pressed', 'false');
    expect(winners).toHaveAttribute('aria-pressed', 'true');
  });

  test('ignores an aside closing itself after another aside already took the slot', async () => {
    const user = userEvent.setup();
    render(<TablePage/>);
    const chat = screen.getByRole('button', {name: 'chat'});
    const reactions = screen.getByRole('button', {name: 'reactions'});

    // Crossing the reactions toggle on the way to chat: reactions opens on
    // hover, chat takes the slot, then reactions' deferred close finally fires.
    await user.click(reactions);
    await user.click(chat);
    await act(async () => {
      (mocks.reactionProps?.onOpenChangeAction as (open: boolean) => void)(false);
    });
    expect(chat).toHaveAttribute('aria-pressed', 'true');
    expect(reactions).toHaveAttribute('aria-pressed', 'false');
  });

  test('E and T toggle reactions and chat, and stay out of a focused text field', async () => {
    const user = userEvent.setup();
    render(<TablePage/>);
    const chat = screen.getByRole('button', {name: 'chat'});
    const reactions = screen.getByRole('button', {name: 'reactions'});

    await user.keyboard('e');
    expect(reactions).toHaveAttribute('aria-pressed', 'true');
    await user.keyboard('t');
    expect(reactions).toHaveAttribute('aria-pressed', 'false');
    expect(chat).toHaveAttribute('aria-pressed', 'true');
    await user.keyboard('t');
    expect(chat).toHaveAttribute('aria-pressed', 'false');

    // Typing the letters into a text field must never move the panels.
    const field = document.createElement('input');
    document.body.append(field);
    field.focus();
    await user.keyboard('et');
    expect(chat).toHaveAttribute('aria-pressed', 'false');
    expect(reactions).toHaveAttribute('aria-pressed', 'false');
    field.remove();
  });

  test('selects a reaction, targets a seat, and clears targeting after a successful throw', async () => {
    realtime({sendReaction: vi.fn(() => true)});
    const {rerender} = render(<TablePage/>);
    act(() => (mocks.reactionProps?.onPendingReactionChangeAction as (reaction: string) => void)('tomato'));
    rerender(<TablePage/>);
    expect(mocks.stageProps).toEqual(expect.objectContaining({targetedReactionLabel: 'Jogar tomate'}));
    act(() => (mocks.stageProps?.onTargetPlayerAction as (playerId: string) => void)('opponent'));
    rerender(<TablePage/>);
    expect(mocks.realtime.sendReaction).toHaveBeenCalledWith('tomato', 'opponent');
    expect(mocks.stageProps?.targetedReactionLabel).toBeUndefined();
  });
  
  test('handles pause, leave and player notes through their backend callbacks', async () => {
    const user = userEvent.setup();
    render(<TablePage/>);
    await user.click(screen.getByRole('button', {name: 'Sentar fora'}));
    expect(mocks.realtime.ready).toHaveBeenCalledWith(false);

    await user.click(screen.getByRole('button', {name: 'table-stage'}));
    expect(screen.getByText('note-dialog')).toBeInTheDocument();

    await user.click(screen.getByRole('button', {name: 'leave'}));
    expect(mocks.realtime.requestExit).toHaveBeenCalledOnce();
  });

  test('an exit_requested removal shows the same recap as an immediate leave', async () => {
    realtime({removed: {code: 'exit_requested', amount: 1234}});
    render(<TablePage/>);
    await waitFor(() => expect(mocks.notification).toHaveBeenCalledWith('Você saiu com 1.234 fichas.', 'info'));
    expect(mocks.push).not.toHaveBeenCalledWith('/lobby');

    await userEvent.click(screen.getByRole('button', {name: 'close-recap'}));
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
    realtime({
      snapshot: snapshot({
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
      })
    });
    render(<TablePage/>);
    await waitFor(() => expect(mocks.stageProps?.outcome).toEqual(expect.objectContaining({
      kind: 'fold', couldHaveWon: undefined,
    })));
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['hands', ROOM_ID]});
  });

  test.each([
    ['forbidden', 'Você não tem acesso a esta mesa.'],
    ['gone', 'Essa sala não está mais disponível.'],
  ])('leaves the table for good on a %s terminal error', async (terminalError, message) => {
    realtime({terminalError});
    render(<TablePage/>);
    await waitFor(() => expect(mocks.push).toHaveBeenCalledWith('/lobby'));
    expect(mocks.notification).toHaveBeenCalledWith(message, 'info');
    expect(mocks.setQueryData).toHaveBeenCalledWith(['seated', ROOM_ID], {seated: false, stack: 0});
  });

  test('announces a win with the beaten runner-up hand and the pot breakdown', async () => {
    realtime({
      snapshot: snapshot({
        stage: 'complete', board: ['2H', '3D', '4C', '5S', '9H'],
        payouts: {viewer: 120}, winners: ['viewer'],
        pot_results: [{
          amount: 120, payout_amount: 120, eligible_player_ids: ['viewer', 'opponent'],
          winner_player_ids: ['viewer'], payouts: {viewer: 120},
        }],
        seats: [
          {
            player_id: 'viewer', name: 'Você', stack: 600, stack_at_hand_start: 500, state: 'active',
            dealt_in: true, contributed: 20, hole_cards: ['AH', 'KD'], hand_category: 'pair', hand_score: 200,
          },
          {
            player_id: 'opponent', name: 'Bia', stack: 780, state: 'active', dealt_in: true,
            contributed: 40, hole_cards: ['QS', 'JC'], hole_cards_revealed: [true, true],
            hand_category: 'high_card', hand_score: 100,
          },
        ],
      })
    });
    render(<TablePage/>);
    await waitFor(() => expect(mocks.stageProps?.outcome).toEqual(expect.objectContaining({
      kind: 'win', winnerName: 'Você', stackBefore: 500, stackAfter: 600, wonAmount: 120,
      beatenCategory: 'high_card',
    })));
    expect((mocks.stageProps?.outcome as {viewerCards: string[]}).viewerCards).toHaveLength(5);
    expect(mocks.stageProps).toEqual(expect.objectContaining({holdOutcomeOpen: true, viewerStackBefore: 500}));
  });

  test('clears the hand outcome once the next hand deals, unblocking achievement toasts and the winner-cards offer', async () => {
    const resolvedHand = snapshot({
      stage: 'complete', board: ['2H', '3D', '4C', '5S', '9H'],
      payouts: {viewer: 120}, winners: ['viewer'],
      seats: [
        {
          player_id: 'viewer', name: 'Você', stack: 600, stack_at_hand_start: 500, state: 'active',
          dealt_in: true, contributed: 20, hole_cards: ['AH', 'KD'], hand_category: 'pair', hand_score: 200,
        },
        {
          player_id: 'opponent', name: 'Bia', stack: 780, state: 'active', dealt_in: true,
          contributed: 40, hole_cards: ['QS', 'JC'], hole_cards_revealed: [true, true],
          hand_category: 'high_card', hand_score: 100,
        },
      ],
    });
    realtime({snapshot: resolvedHand});
    const {rerender} = render(<TablePage/>);
    await waitFor(() => expect(mocks.stageProps?.outcome).toEqual(expect.objectContaining({kind: 'win'})));

    // The next hand deals under a fresh hand_id with no payouts yet: the
    // outcome from the resolved hand above must not linger and keep gating
    // AchievementToast/WinnerCards forever (issue #78).
    realtime({snapshot: snapshot({hand_id: 'hand-2', stage: 'pre_flop'})});
    rerender(<TablePage/>);
    await waitFor(() => expect(mocks.stageProps?.outcome).toBeNull());
  });

  test('produces a fresh outcome for a second resolved hand instead of latching the first', async () => {
    realtime({
      snapshot: snapshot({
        stage: 'complete', board: ['2H', '3D', '4C', '5S', '9H'],
        payouts: {viewer: 50}, winners: ['viewer'],
        seats: [
          {
            player_id: 'viewer', name: 'Você', stack: 550, stack_at_hand_start: 500, state: 'active',
            dealt_in: true, contributed: 20, hole_cards: ['AH', 'KD'], hand_category: 'pair', hand_score: 200,
          },
          {player_id: 'opponent', name: 'Bia', stack: 750, state: 'active', dealt_in: true, contributed: 40},
        ],
      })
    });
    const {rerender} = render(<TablePage/>);
    await waitFor(() => expect(mocks.stageProps?.outcome).toEqual(expect.objectContaining({kind: 'win'})));

    // Next hand deals (clearing the first hand's outcome), then resolves too.
    realtime({snapshot: snapshot({hand_id: 'hand-2', stage: 'pre_flop'})});
    rerender(<TablePage/>);
    await waitFor(() => expect(mocks.stageProps?.outcome).toBeNull());

    realtime({
      snapshot: snapshot({
        hand_id: 'hand-2', stage: 'complete', board: ['2H', '3D', '4C', '5S', '9H'],
        payouts: {opponent: 60}, winners: ['opponent'],
        seats: [
          {
            player_id: 'viewer', name: 'Você', stack: 490, stack_at_hand_start: 550, state: 'active',
            dealt_in: true, contributed: 20, hole_cards: ['2C', '3S'], hand_category: 'high_card', hand_score: 10,
          },
          {
            player_id: 'opponent', name: 'Bia', stack: 810, state: 'active', dealt_in: true,
            contributed: 40, hole_cards: ['AS', 'AC'], hole_cards_revealed: [true, true],
            hand_category: 'pair', hand_score: 300,
          },
        ],
      })
    });
    rerender(<TablePage/>);
    await waitFor(() => expect(mocks.stageProps?.outcome).toEqual(expect.objectContaining({kind: 'lose'})));
  });

  test('details every contested pot when the viewer wins one and loses another', async () => {
    realtime({
      snapshot: snapshot({
        stage: 'complete', board: ['2H', '3D', '4C', '5S', '9H'],
        payouts: {viewer: 100, opponent: 300}, winners: ['viewer', 'opponent'],
        pot_results: [
          {
            amount: 100, payout_amount: 100, eligible_player_ids: ['viewer', 'opponent'],
            winner_player_ids: ['viewer'], payouts: {viewer: 100},
          },
          {
            amount: 300, payout_amount: 300, eligible_player_ids: ['viewer', 'opponent'],
            winner_player_ids: ['opponent'], payouts: {opponent: 300},
          },
        ],
        seats: [
          {
            player_id: 'viewer', name: 'Você', stack: 600, stack_at_hand_start: 500, state: 'active',
            dealt_in: true, contributed: 100, hole_cards: ['AH', 'KD'], hand_category: 'pair', hand_score: 200,
          },
          {
            player_id: 'opponent', name: 'Bia', stack: 900, state: 'active', dealt_in: true,
            contributed: 100, hole_cards: ['9S', '9C'], hand_category: 'three_of_a_kind', hand_score: 900,
          },
        ],
      })
    });
    render(<TablePage/>);
    await waitFor(() => expect((mocks.stageProps?.outcome as {kind: string}).kind).toBe('mixed'));
    const pots = (mocks.stageProps?.outcome as {pots: {won: boolean; winnerName?: string}[]}).pots;
    expect(pots.map(pot => pot.won)).toEqual([true, false]);
    expect(pots[1]).toEqual(expect.objectContaining({winnerName: 'Bia', category: 'three_of_a_kind'}));
  });

  test('names the other hands in a split pot', async () => {
    realtime({
      snapshot: snapshot({
        stage: 'complete', board: ['2H', '3D', '4C', '5S', '9H'],
        payouts: {viewer: 100, opponent: 100}, winners: ['viewer', 'opponent'],
        pot_results: [{
          amount: 200, payout_amount: 200, eligible_player_ids: ['viewer', 'opponent'],
          winner_player_ids: ['viewer', 'opponent'], payouts: {viewer: 100, opponent: 100},
        }],
        seats: [
          {
            player_id: 'viewer', name: 'Você', stack: 600, stack_at_hand_start: 600, state: 'active',
            dealt_in: true, contributed: 100, hole_cards: ['AH', 'KD'], hand_category: 'pair', hand_score: 200,
          },
          {
            player_id: 'opponent', name: 'Bia', stack: 600, state: 'active', dealt_in: true,
            contributed: 100, hole_cards: ['AS', 'KC'], hand_category: 'pair', hand_score: 200,
          },
        ],
      })
    });
    render(<TablePage/>);
    await waitFor(() => expect((mocks.stageProps?.outcome as {kind: string}).kind).toBe('tie'));
    const tiedWith = (mocks.stageProps?.outcome as {tiedWith: {name: string; cards: string[]}[]}).tiedWith;
    expect(tiedWith).toHaveLength(1);
    expect(tiedWith[0]).toEqual(expect.objectContaining({name: 'Bia'}));
    expect(tiedWith[0].cards).toHaveLength(5);
  });

  test('remembers the pre-blind stack when the server does not publish it', async () => {
    const live = snapshot({
      seats: [
        {player_id: 'viewer', name: 'Você', stack: 480, state: 'active', dealt_in: true, contributed: 20},
        {player_id: 'opponent', name: 'Bia', stack: 800, state: 'active', dealt_in: true, contributed: 40},
      ],
    });
    realtime({snapshot: live});
    const {rerender} = render(<TablePage/>);
    realtime({
      snapshot: {
        ...live, stage: 'complete', board: ['2H', '3D', '4C', '5S', '9H'],
        payouts: {viewer: 100}, winners: ['viewer'],
        seats: [{...live.seats[0], stack: 580}, live.seats[1]],
      }
    });
    rerender(<TablePage/>);
    await waitFor(() => expect((mocks.stageProps?.outcome as {stackBefore?: number}).stackBefore).toBe(500));
  });

  test('freezes the next-hand countdown against the snapshot that armed it', () => {
    const deadline = Date.now() + 5000;
    realtime({snapshot: snapshot({next_hand_unix_ms: deadline}), snapshotAt: deadline - 5000});
    const {rerender} = render(<TablePage/>);
    expect(mocks.stageProps?.nextHandDurationMs).toBe(5000);

    realtime({snapshot: snapshot({next_hand_unix_ms: deadline}), snapshotAt: deadline - 1000});
    rerender(<TablePage/>);
    expect(mocks.stageProps?.nextHandDurationMs).toBe(5000);
  });

  test('hides the next-hand deadline while the connection is unstable', () => {
    realtime({snapshot: snapshot({next_hand_unix_ms: Date.now() + 5000}), status: 'reconnecting', reconnectAttempt: 2});
    render(<TablePage/>);
    expect(mocks.stageProps?.nextHandDeadlineMs).toBeUndefined();
    expect(screen.getByText(/Reconectando à mesa…\s*Tentativa 2\./)).toBeInTheDocument();
  });

  test('shows the migration notice without a retry button while the socket is still up', () => {
    realtime({
      snapshot: snapshot({next_hand_unix_ms: Date.now() + 5000}),
      migrating: 'Esta mesa está migrando de servidor.', status: 'connected',
    });
    render(<TablePage/>);
    expect(screen.getAllByText('Esta mesa está migrando de servidor.').length).toBeGreaterThan(0);
    expect(screen.queryByRole('button', {name: /Tentar agora/})).not.toBeInTheDocument();
    expect(mocks.stageProps?.nextHandDeadlineMs).toBeUndefined();
  });

  test('keeps the migration copy and restores the retry button once the socket drops', () => {
    realtime({migrating: 'Esta mesa está migrando de servidor.', status: 'reconnecting', reconnectAttempt: 1});
    render(<TablePage/>);
    expect(screen.getAllByText('Esta mesa está migrando de servidor.').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', {name: /Tentar agora/})).toBeInTheDocument();
  });

  test('offers card reveal only for a participating seat still holding a hidden card', async () => {
    realtime({
      snapshot: snapshot({
        stage: 'complete', protocol_version: 8, won_without_showdown: true,
        seats: [
          {
            player_id: 'viewer', name: 'Você', stack: 500, state: 'active', dealt_in: true,
            contributed: 20, hole_cards: ['AH', 'KD'], hole_cards_revealed: [true, false],
          },
          {player_id: 'opponent', name: 'Bia', stack: 800, state: 'active', dealt_in: true, contributed: 40},
        ],
      })
    });
    render(<TablePage/>);
    expect(mocks.stageProps?.canRevealCards).toBe(true);
    act(() => (mocks.stageProps?.onRevealCardAction as (index: number) => void)(1));
    expect(mocks.realtime.showCards).toHaveBeenCalledWith(1);
  });

  test('wires the rabbit hunt price and request/refund callbacks into TableStage', async () => {
    realtime({snapshot: snapshot({stage: 'complete', won_without_showdown: true})});
    render(<TablePage/>);
    expect(mocks.stageProps?.bigBlind).toBeGreaterThan(0);
    act(() => (mocks.stageProps?.onRequestRabbitHuntAction as () => void)());
    expect(mocks.realtime.requestRabbitHunt).toHaveBeenCalledOnce();
    act(() => (mocks.stageProps?.onRabbitHuntVerifyFailedAction as () => void)());
    expect(mocks.realtime.reportRabbitHuntVerifyFailed).toHaveBeenCalledOnce();
    expect(mocks.stageProps?.rabbitHuntPending).toBe(false);
  });

  test('wires the winner-card request into TableStage', () => {
    realtime({snapshot: snapshot({stage: 'complete', won_without_showdown: true, winners: ['opponent']})});
    render(<TablePage/>);
    act(() => (mocks.stageProps?.onRequestWinnerCardsAction as () => void)());
    expect(mocks.realtime.requestWinnerCards).toHaveBeenCalledOnce();
    expect(mocks.stageProps?.winnerCardsPending).toBe(false);
  });

  test('a paused seat resumes play, and a busted one rebuys instead', async () => {
    const user = userEvent.setup();
    realtime({
      snapshot: snapshot({
        seats: [
          {player_id: 'viewer', name: 'Você', stack: 500, state: 'sitting_out', dealt_in: false, contributed: 0},
          {player_id: 'opponent', name: 'Bia', stack: 800, state: 'active', dealt_in: true, contributed: 40},
        ],
      })
    });
    const {rerender} = render(<TablePage/>);
    await user.click(screen.getByRole('button', {name: 'Voltar a jogar'}));
    expect(mocks.realtime.ready).toHaveBeenCalledWith(true);

    realtime({
      snapshot: snapshot({
        seats: [
          {player_id: 'viewer', name: 'Você', stack: 0, state: 'sitting_out', dealt_in: false, contributed: 0},
          {player_id: 'opponent', name: 'Bia', stack: 800, state: 'active', dealt_in: true, contributed: 40},
        ],
      })
    });
    rerender(<TablePage/>);
    expect(screen.queryByRole('button', {name: 'Voltar a jogar'})).not.toBeInTheDocument();
    await user.click(await screen.findByRole('button', {name: 'rebuy'}));
    expect(mocks.realtime.ready).toHaveBeenLastCalledWith(true);
  });

  test('exposes a private table invite link built from the room share code', () => {
    setQueries({roomData: {...room, visibility: 'private', share_code: 'ABC123'}});
    render(<TablePage/>);
    expect(screen.getByText(`invite:${window.location.origin}/table?id=${ROOM_ID}&invite=ABC123`))
      .toBeInTheDocument();
  });

  test('keeps the preferences and invite triggers on their own, for layouts without the utility menu', () => {
    // .table-utility-menu-slot is display:none outside portrait ≤1023px, so
    // these dialogs are the only way into preferences and invites on desktop:
    // suppressing their triggers left those actions unreachable there.
    setQueries({roomData: {...room, visibility: 'private', share_code: 'ABC123'}});
    render(<TablePage/>);
    expect(mocks.preferencesProps?.showTrigger).not.toBe(false);
    expect(mocks.inviteProps?.showTrigger).not.toBe(false);
  });

  test('hides the invite affordance on a private table the viewer did not create', () => {
    setQueries({roomData: {...room, visibility: 'private'}});
    render(<TablePage/>);
    expect(screen.queryByText(/^invite:/)).not.toBeInTheDocument();
  });

  test('rate-limits quick reactions and refuses to send them while offline', async () => {
    vi.useFakeTimers();
    try {
      const sendReaction = vi.fn(() => true);
      realtime({sendReaction});
      const {rerender} = render(<TablePage/>);
      act(() => (mocks.reactionProps?.onQuickSendAction as (id: string) => void)('clap'));
      expect(sendReaction).toHaveBeenCalledWith('clap');

      rerender(<TablePage/>);
      expect(mocks.reactionProps?.coolingDown).toBe(true);
      act(() => (mocks.reactionProps?.onQuickSendAction as (id: string) => void)('laugh'));
      expect(sendReaction).toHaveBeenCalledTimes(1);

      act(() => void vi.advanceTimersByTime(2000));
      rerender(<TablePage/>);
      expect(mocks.reactionProps?.coolingDown).toBe(false);
      act(() => (mocks.reactionProps?.onQuickSendAction as (id: string) => void)('laugh'));
      expect(sendReaction).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });

  test('does not start a cooldown when the socket rejects the reaction', () => {
    realtime({sendReaction: vi.fn(() => false)});
    const {rerender} = render(<TablePage/>);
    act(() => (mocks.reactionProps?.onQuickSendAction as (id: string) => void)('clap'));
    rerender(<TablePage/>);
    expect(mocks.reactionProps?.coolingDown).toBe(false);
  });

  test('never sends a reaction over a disconnected socket', () => {
    const sendReaction = vi.fn(() => true);
    realtime({status: 'reconnecting', sendReaction});
    render(<TablePage/>);
    act(() => (mocks.reactionProps?.onQuickSendAction as (id: string) => void)('clap'));
    act(() => (mocks.reactionProps?.onPendingReactionChangeAction as (id: string) => void)('tomato'));
    expect(sendReaction).not.toHaveBeenCalled();
  });

  test('saves reaction shortcuts and confirms them to the player', async () => {
    const updated = {favorite_reactions: ['clap', 'fire']};
    mocks.updateMe.mockResolvedValue(updated);
    setQueries({data: {'player:me': {favorite_reactions: ['clap', 'not-a-reaction'], sandbox_balance: 900}}});
    render(<TablePage/>);
    expect(mocks.reactionProps?.favorites).toEqual(['clap']);

    await act(() => (mocks.reactionProps?.onFavoriteReactionsChangeAction as (f: string[]) => Promise<void>)(
      ['clap', 'fire']
    ));
    expect(mocks.updateMe).toHaveBeenCalledWith({favorite_reactions: ['clap', 'fire']});
    expect(mocks.setQueryData).toHaveBeenCalledWith(['player', 'me'], updated);
    expect(mocks.notification).toHaveBeenCalledWith('Atalhos de reação atualizados.', 'info');
  });

  test('opens the purchase dialog for a locked reaction with its in-flight purchase and balance', async () => {
    const purchases = [{purchase_id: 'p1', reaction_id: 'fire', method: 'pix', status: 'pending'}];
    setQueries({
      data: {
        'player:me': {sandbox_balance: 4200},
        'wallet:reaction-purchases:first-page': purchases,
      }
    });
    const {rerender} = render(<TablePage/>);
    act(() => (mocks.reactionProps?.onLockedReactionAction as (entry: object) => void)({id: 'fire', premium: true}));
    rerender(<TablePage/>);

    expect(mocks.purchaseProps).toEqual(expect.objectContaining({
      entry: {id: 'fire', premium: true}, initialPurchase: purchases[0], sandboxBalance: 4200,
    }));
    expect(mocks.reactionProps?.open).toBe(false);

    await userEvent.click(screen.getByRole('button', {name: 'purchase-dialog'}));
    rerender(<TablePage/>);
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({queryKey: ['wallet']});
    expect(mocks.purchaseProps?.entry).toBeNull();
  });

  test('keeps the local player-note cache in sync when a note is saved and cleared', async () => {
    const note = {opponent_id: 'opponent', note: 'agressivo'};
    setQueries({data: {'player-notes': [{opponent_id: 'other', note: 'passivo'}]}});
    render(<TablePage/>);
    await userEvent.click(screen.getByRole('button', {name: 'table-stage'}));

    act(() => (mocks.noteProps?.onSaved as (note: object) => void)(note));
    const [, updater] = mocks.setQueryData.mock.calls.at(-1)!;
    expect((updater as (current: object[]) => object[])([{opponent_id: 'opponent', note: 'velho'}])).toEqual([note]);

    act(() => (mocks.noteProps?.onSaved as (note: object | null) => void)(null));
    const [, clearUpdater] = mocks.setQueryData.mock.calls.at(-1)!;
    expect((clearUpdater as (current: object[]) => object[])([{opponent_id: 'opponent', note: 'velho'}])).toEqual([]);
  });

  test('sums the pot from server pots and falls back to seat contributions', () => {
    realtime({snapshot: snapshot({pots: [
      {amount: 60, eligible_player_ids: ['viewer', 'opponent']},
      {amount: 40, eligible_player_ids: ['viewer']},
    ]})});
    const {rerender} = render(<TablePage/>);
    expect(mocks.stageProps?.pot).toBe(100);

    realtime({snapshot: snapshot()});
    rerender(<TablePage/>);
    expect(mocks.stageProps?.pot).toBe(60);
  });

  test('prefers the open session buy-in and joined_at over the local table clock', () => {
    setQueries({
      data: {
        'sessions:me': [
          {table_id: ROOM_ID, ended_at: 12, joined_at: 1, buyin_amount: 100},
          {table_id: ROOM_ID, ended_at: 0, joined_at: 999, buyin_amount: 700},
        ],
      }
    });
    render(<TablePage/>);
    expect(screen.getByRole('button', {name: 'action-bar'})).toBeInTheDocument();
  });

  test('warns before an idle removal and lets the player keep the seat', () => {
    vi.useFakeTimers();
    try {
      realtime({snapshot: snapshot({idle_removal_unix_ms: Date.now() + 90_000})});
      render(<TablePage/>);
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();

      act(() => void vi.advanceTimersByTime(31_000));
      const warning = screen.getByRole('alert');
      expect(warning).toHaveTextContent(/Você será removido por inatividade em \d+s\./);

      act(() => void vi.advanceTimersByTime(1000));
      fireEvent.click(screen.getByRole('button', {name: 'Continuar na mesa'}));
      expect(mocks.realtime.keepSeat).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });

  test('stays silent when no idle removal is armed', () => {
    render(<TablePage/>);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
  test('feeds the muted and blocked seats into the realtime suppression set', async () => {
    setQueries({data: {'social:relationships:opponent': [
      {player_id: 'opponent', relationship: 'friend', muted: true, blocked: false},
    ]}});
    render(<TablePage/>);
    await waitFor(() => expect(mocks.realtimeHook).toHaveBeenLastCalledWith(ROOM_ID, 'viewer', undefined, undefined,
      new Set(['opponent'])));
  });

  test('hides a blocked seat content immediately and restores it if the block fails', async () => {
    mocks.blockPlayer.mockRejectedValue(new Error('offline'));
    render(<TablePage/>);
    await userEvent.click(screen.getByRole('button', {name: 'Ações para Bia'}));
    await userEvent.click(await screen.findByRole('button', {name: 'Bloquear'}));
    await userEvent.click(screen.getByRole('button', {name: 'Bloquear'}));

    // Suppressed before the round-trip finished, then rolled back on failure.
    expect(mocks.realtimeHook.mock.calls.some(call => (call[4] as Set<string>)?.has('opponent'))).toBe(true);
    await waitFor(() => expect((mocks.realtimeHook.mock.calls.at(-1)?.[4] as Set<string>).has('opponent'))
      .toBe(false));
    expect(mocks.notification).toHaveBeenCalledWith('Não foi possível concluir essa ação. Tente novamente.');
  });

  test('opens the private note from the seat menu', async () => {
    render(<TablePage/>);
    await userEvent.click(screen.getByRole('button', {name: 'Ações para Bia'}));
    await userEvent.click(await screen.findByRole('button', {name: 'Editar nota privada'}));
    expect(mocks.noteProps?.opponent).toEqual({player_id: 'opponent', name: 'Bia'});
  });


  // Issue #60: an automated floor under the a11y intent in ui/CLAUDE.md — a new
  // serious or critical axe violation on this route fails CI.
  test('is axe-clean', async () => {
    const {container} = render(<TablePage/>);
    await expectNoAxeViolations(container);
  });

});
