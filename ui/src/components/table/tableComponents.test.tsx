import {fireEvent, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {type ActionAvailability, ActionBar} from './ActionBar';
import {Board} from './Board';
import {Chat} from './Chat';
import {HandOutcomeBanner, type HandOutcomeState} from './HandOutcome';
import {TableStage} from './TableStage';
import {MOCK_PLAYER_ID, snapshotForScenario} from '@/dev/mockRuntime';

vi.mock('@/lib/hooks/useDeckVariant', () => ({
  useDeckVariant: () => 'four-color',
}));

const allActions: ActionAvailability = {fold: true, check: true, call: true, raise: true};

function renderActionBar(overrides: Partial<React.ComponentProps<typeof ActionBar>> = {}) {
  const onActAction = vi.fn(() => true);
  const props: React.ComponentProps<typeof ActionBar> = {
    onActAction,
    available: allActions,
    callAmount: 75,
    minRaise: 150,
    maxRaise: 1000,
    raiseStep: 25,
    effectiveStack: 1000,
    raisePresets: [{label: '½ pote', value: 250}, {label: 'Pote', value: 500}],
    actionKey: 'hand-1:pre_flop',
    isTurn: true,
    connected: true,
    pending: null,
    error: null,
    onDismissErrorAction: vi.fn(),
    canPreselect: false,
    supportsCallPreselection: false,
    selectionScope: '',
    preselection: null,
    preselectionAmount: 0,
    prospectiveCallAmount: 0,
    onPreselectAction: vi.fn(() => true),
    timeBankMs: 30_000,
    voiceCommands: false,
    ...overrides,
  };
  render(<ActionBar {...props}/>);
  return {onActAction, props};
}

describe('table controls', () => {
  test('submits every legal action with its correct amount', async () => {
    const user = userEvent.setup();
    const {onActAction} = renderActionBar();
    
    await user.click(screen.getByRole('button', {name: /Fold/}));
    await user.click(screen.getByRole('button', {name: /Check/}));
    await user.click(screen.getByRole('button', {name: /Pagar 75/}));
    await user.click(screen.getByRole('button', {name: /Aumentar/}));
    
    expect(onActAction).toHaveBeenNthCalledWith(1, 'fold');
    expect(onActAction).toHaveBeenNthCalledWith(2, 'check');
    expect(onActAction).toHaveBeenNthCalledWith(3, 'call');
    expect(onActAction).toHaveBeenNthCalledWith(4, 'raise', 150);
  });
  
  test('blocks actions while disconnected and explains why', () => {
    renderActionBar({connected: false});
    expect(screen.getByText('Reconectando antes de liberar as ações…')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Fold/})).toBeDisabled();
  });
  
  test('does not present an active time bank outside the viewer turn', () => {
    renderActionBar({isTurn: false, actionBaseDeadlineMs: Date.now() - 5_000, actionDeadlineMs: Date.now() + 10_000});
    expect(screen.getByText(/Sua reserva/)).toBeInTheDocument();
    expect(screen.getByRole('timer')).toHaveAttribute('aria-label', 'Time bank disponível: 30 segundos');
  });
  
  test('renders and dismisses a backend action error', async () => {
    const onDismiss = vi.fn();
    renderActionBar({
      error: {code: 'stale_state', message: 'A ação não é mais válida.'},
      onDismissErrorAction: onDismiss
    });
    expect(screen.getByRole('alert')).toHaveTextContent('A ação não é mais válida.');
    await userEvent.click(screen.getByRole('button', {name: 'Fechar aviso'}));
    expect(onDismiss).toHaveBeenCalledOnce();
  });
  
  test('executes a prepared check/fold as soon as the turn arrives', () => {
    const {onActAction} = renderActionBar({
      available: {fold: true, check: true, call: false, raise: true},
      selectionScope: 'hand-2',
      preselection: 'check_fold',
      canPreselect: true,
    });
    expect(onActAction).toHaveBeenCalledWith('check');
    expect(screen.getByText('Executando sua ação preparada…')).toBeInTheDocument();
  });
  
  test('supports action and bet-sizing keyboard shortcuts without stealing input keystrokes', async () => {
    const {onActAction} = renderActionBar();
    fireEvent.keyDown(window, {key: 'f'});
    fireEvent.keyDown(window, {key: 'c'});
    fireEvent.keyDown(window, {key: 'p'});
    fireEvent.keyDown(window, {key: 'ArrowRight'});
    fireEvent.keyDown(window, {key: 'r'});
    
    expect(onActAction).toHaveBeenNthCalledWith(1, 'fold');
    expect(onActAction).toHaveBeenNthCalledWith(2, 'check');
    expect(onActAction).toHaveBeenNthCalledWith(3, 'call');
    expect(onActAction).toHaveBeenNthCalledWith(4, 'raise', 175);
    
    const input = document.createElement('input');
    document.body.append(input);
    input.focus();
    fireEvent.keyDown(input, {key: 'f'});
    expect(onActAction).toHaveBeenCalledTimes(4);
    input.remove();
  });
  
  test('selects, deselects and executes supported pre-actions only when legal', async () => {
    const onPreselectAction = vi.fn(() => true);
    renderActionBar({
      isTurn: false,
      canPreselect: true,
      supportsCallPreselection: true,
      prospectiveCallAmount: 75,
      onPreselectAction,
    });
    
    await userEvent.click(screen.getByRole('button', {name: /Call 75/}));
    expect(onPreselectAction).toHaveBeenCalledWith('call', 75);
    const selected = screen.getByRole('button', {name: /Check \/ Fold/});
    await userEvent.click(selected);
    expect(onPreselectAction).toHaveBeenCalledWith('check_fold', 0);
  });
  
  test('collapses controls when the backend reports no legal action', () => {
    renderActionBar({available: {fold: false, check: false, call: false, raise: false}});
    expect(screen.queryByRole('group', {name: 'Ações rápidas'})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Aumentar/})).not.toBeInTheDocument();
  });
  
  test('shows pending action state and effective-stack context', () => {
    renderActionBar({pending: 'call', effectiveStack: 12_345});
    expect(screen.getAllByText('Pagando…')).toHaveLength(2);
    expect(screen.getByRole('group', {name: 'Ações da rodada'})).toHaveAttribute('aria-busy', 'true');
  });
});

describe('table presentation', () => {
  test('shows one public playstyle badge and leaves unbadged seats unchanged', () => {
    const snapshot = snapshotForScenario('pre_flop');
    snapshot.seats.forEach(seat => {
      seat.playstyle_badge = undefined;
    });
    snapshot.seats[0].playstyle_badge = 'initiative';
    const {container} = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID}
                                           pot={0} bigBlind={50} nowMs={Date.now()} outcome={null}
                                           holdOutcomeOpen={false}/>);
    expect(screen.getByText('Iniciativa')).toHaveAttribute('title', 'PFR representa pelo menos 70% do VPIP');
    expect(container.querySelectorAll('.seat-playstyle')).toHaveLength(1);
  });
  
  test('board renders pot, rake, side pots and missing card slots', () => {
    const {container} = render(<Board cards={['AH', 'KD', '2C']} pot={1_250}
                                      rake={25} bigBlind={50} pots={[
      {amount: 1000, eligible_player_ids: ['a', 'b']},
      {amount: 250, eligible_player_ids: ['a']},
    ]}/>);
    expect(screen.getByText('1.250')).toBeInTheDocument();
    expect(screen.getByLabelText('Comissão da casa: 25 fichas')).toBeInTheDocument();
    expect(screen.getByLabelText('Divisão dos potes')).toHaveTextContent('Lateral 1: 250');
    expect(container.querySelectorAll('.playing-card')).toHaveLength(3);
  });
  
  test.each(['waiting', 'pre_flop', 'flop', 'turn', 'river', 'showdown', 'complete'] as const)(
    'renders backend %s state without crashing',
    scenario => {
      const snapshot = snapshotForScenario(scenario);
      const {container} = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID}
                                             pot={snapshot.pots?.reduce((sum, pot) => sum + pot.amount, 0) || 0}
                                             bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}/>);
      expect(container.querySelector('.game-table')).toBeInTheDocument();
      expect(container.querySelectorAll('.game-seat')).toHaveLength(snapshot.seats.length);
    },
  );
});

describe('chat', () => {
  test('opens, resolves player names and sends a trimmed message', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    const onSend = vi.fn(() => true);
    render(<Chat open items={[{id: '1', player: 'p2', message: 'Boa mão'}]}
                 seats={[{player_id: 'p2', name: 'Bia', stack: 10, state: 'active', contributed: 0}]}
                 onOpenChange={onOpenChange} onSend={onSend}/>);
    expect(screen.getByRole('log')).toHaveTextContent('BiaBoa mão');
    await user.type(screen.getByLabelText('Mensagem para a mesa'), '  olá  ');
    await user.click(screen.getByRole('button', {name: 'Enviar mensagem'}));
    expect(onSend).toHaveBeenCalledWith('olá');
    expect(screen.getByLabelText('Mensagem para a mesa')).toHaveValue('');
  });
  
  test('reports send failure and toggles the controlled panel', async () => {
    const onOpenChange = vi.fn();
    render(<Chat open connected items={[]} onOpenChange={onOpenChange} onSend={() => false}/>);
    await userEvent.type(screen.getByLabelText('Mensagem para a mesa'), 'teste');
    await userEvent.click(screen.getByRole('button', {name: 'Enviar mensagem'}));
    expect(screen.getByRole('alert')).toHaveTextContent('Mensagem não enviada');
    await userEvent.click(screen.getByRole('button', {name: 'Fechar chat'}));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe('hand outcome', () => {
  const renderOutcome = (outcome: HandOutcomeState) =>
    render(<HandOutcomeBanner outcome={outcome} holdOpen/>);
  
  test.each([
    [{
      key: 1,
      kind: 'win',
      handCategory: 'pair',
      winningCards: ['AH', 'AD', 'KC', 'QS', '2D'],
      stackBefore: 900,
      stackAfter: 1200
    }, 'Você venceu!'],
    [{key: 2, kind: 'tie', handCategory: 'straight', winningCards: ['AH', 'KD', 'QS', 'JC', 'TH']}, 'Pote dividido'],
    [{key: 3, kind: 'fold', couldHaveWon: false}, 'Você desistiu'],
    [{key: 4, kind: 'mixed', wonAmount: 100, refundAmount: 25}, 'Resultado misto'],
  ] satisfies Array<[HandOutcomeState, string]>)('renders outcome %#', (outcome, text) => {
    renderOutcome(outcome);
    expect(screen.getAllByText(new RegExp(text))[0]).toBeInTheDocument();
  });
  
  test('explains a showdown loss decided by kicker', () => {
    renderOutcome({
      key: 5,
      kind: 'lose',
      winnerName: 'Bia',
      viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'],
      winningCards: ['AS', 'AC', 'KH', 'QD', '3D'],
      viewerHoleCards: ['AH', 'KC'],
    });
    expect(screen.getByText('Mesma combinação, o kicker decidiu.')).toBeInTheDocument();
    expect(screen.getAllByText(/Bia/)[0]).toBeInTheDocument();
  });
});
