import {fireEvent, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {type ActionAvailability, ActionBar} from './ActionBar';
import {Board} from './Board';
import {Chat} from './Chat';
import {HandOutcomeBanner, type HandOutcomeState} from './HandOutcome';
import {balancedSeatPosition, stableSeatOccupants, tableCapacity, TableStage} from './TableStage';
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
    pot: 200,
    shortcutsEnabled: true,
    favoriteBetPresets: [],
    favoriteBetPresetsSaving: false,
    onToggleFavoriteBetPresetAction: vi.fn(),
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


const NON_MATCHING_MEDIA = (query: string) => ({
  matches: false, media: query, onchange: null,
  addListener: vi.fn(), removeListener: vi.fn(),
  addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
} as unknown as MediaQueryList);

// The vertical stage is a media-query decision; restore the default so the
// hover-capable checks in the other suites keep seeing a desktop pointer.
function useVerticalStage() {
  vi.mocked(window.matchMedia).mockImplementation(query => ({
    ...NON_MATCHING_MEDIA(query), matches: true,
  } as unknown as MediaQueryList));
}

afterEach(() => vi.mocked(window.matchMedia).mockImplementation(NON_MATCHING_MEDIA));

describe('table presentation', () => {
  test.each([[2, 2], [3, 6], [6, 6], [7, 9], [9, 9], [undefined, 9]] as const)(
    'maps room capacity %s to the %s-seat physical layout', (maximum, capacity) => {
      expect(tableCapacity(maximum)).toBe(capacity);
    });

  test('keeps surviving players in stable slots and fills a vacancy in turn order', () => {
    expect(stableSeatOccupants(['a', 'b', 'c'], [null, null, null])).toEqual(['a', 'b', 'c']);
    expect(stableSeatOccupants(['a', 'c'], ['a', 'b', 'c'])).toEqual(['a', null, 'c']);
    expect(stableSeatOccupants(['a', 'd', 'c'], ['a', null, 'c'])).toEqual(['a', 'd', 'c']);
  });

  test.each([
    ['heads_up', 2], ['layout_3', 3], ['layout_4', 4], ['layout_5', 5],
    ['six_max', 6], ['layout_7', 7], ['layout_8', 8], ['nine_max', 9],
  ] as const)('balances the %s portrait ring by current occupancy', (scenario, playerCount) => {
    useVerticalStage();
    const snapshot = snapshotForScenario(scenario);
    const {container} = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID} maxSeats={9}
      seatLayoutKey="room-1" pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}/>);
    expect(container.querySelector('.stage-v')).toHaveAttribute('data-player-count', String(playerCount));
    expect(container.querySelectorAll('.stage-v-ring .game-seat')).toHaveLength(playerCount - 1);
    expect(container.querySelector('.stage-v > .game-seat.viewer')).toBeInTheDocument();
    expect(container.querySelectorAll('.stage-v-ring [data-balanced-seat]')).toHaveLength(playerCount - 1);
  });

  test('derives the canonical heads-up, triangle and diamond geometry', () => {
    expect(balancedSeatPosition(0, 2)).toEqual({s: 0.5, t: 1, zone: 'bottom', side: 'center'});
    expect(balancedSeatPosition(1, 2)).toEqual({s: 0.5, t: 0, zone: 'top', side: 'center'});
    expect(balancedSeatPosition(1, 3)).toEqual({s: 0.067, t: 0.25, zone: 'left', side: 'left'});
    expect(balancedSeatPosition(2, 3)).toEqual({s: 0.933, t: 0.25, zone: 'right', side: 'right'});
    expect([0, 1, 2, 3].map(index => balancedSeatPosition(index, 4))).toEqual([
      {s: 0.5, t: 1, zone: 'bottom', side: 'center'},
      {s: 0, t: 0.5, zone: 'left', side: 'left'},
      {s: 0.5, t: 0, zone: 'top', side: 'center'},
      {s: 1, t: 0.5, zone: 'right', side: 'right'},
    ]);
  });

  test('samples portrait seats from the capsule rail instead of an inset ellipse', () => {
    expect(balancedSeatPosition(1, 2, true)).toEqual({s: 0.5, t: 0, zone: 'top', side: 'center'});
    expect(balancedSeatPosition(1, 3, true)).toEqual({s: 0.078, t: 0.179, zone: 'left', side: 'left'});
    expect(balancedSeatPosition(2, 3, true)).toEqual({s: 0.922, t: 0.179, zone: 'right', side: 'right'});
    expect(balancedSeatPosition(1, 9, true)).toEqual({s: 0.078, t: 0.798, zone: 'bottom', side: 'left'});
    expect(balancedSeatPosition(8, 9, true)).toEqual({s: 0.922, t: 0.798, zone: 'bottom', side: 'right'});
  });

  test.each([
    ['heads_up', 2], ['layout_3', 3], ['layout_4', 4], ['layout_5', 5],
    ['six_max', 6], ['layout_7', 7], ['layout_8', 8], ['nine_max', 9],
  ] as const)('balances all %s desktop seats on the occupied perimeter', (scenario, playerCount) => {
    const snapshot = snapshotForScenario(scenario);
    const {container} = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID} maxSeats={9}
      seatLayoutKey="room-1" pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}/>);
    expect(container.querySelector('.game-table')).toHaveAttribute('data-player-count', String(playerCount));
    expect(container.querySelectorAll('.game-table > [data-balanced-seat]')).toHaveLength(playerCount);
    expect(container.querySelector('.game-table > .viewer')).toHaveAttribute('data-seat-zone', 'bottom');
  });

  test('flies the house mark on the felt on both stages', () => {
    const desktop = render(<TableStage snapshot={snapshotForScenario('pre_flop')} viewer={MOCK_PLAYER_ID}
      pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}/>);
    const desktopMark = desktop.container.querySelector('.game-felt .felt-wordmark');
    expect(desktopMark).toHaveTextContent('CTECH');
    expect(desktopMark).toHaveAttribute('aria-hidden', 'true');
    // The board and the street rail travel together so the rail can never
    // drift into the lane the bottom seats' bet chips move through.
    expect(desktop.container.querySelector('.felt-center > .board')).toBeInTheDocument();
    expect(desktop.container.querySelector('.felt-center > .street-progress')).toBeInTheDocument();
    desktop.unmount();

    useVerticalStage();
    const portrait = render(<TableStage snapshot={snapshotForScenario('six_max')} viewer={MOCK_PLAYER_ID}
      maxSeats={6} seatLayoutKey="room-1" pot={0} bigBlind={50} nowMs={Date.now()} outcome={null}
      holdOutcomeOpen={false}/>);
    expect(portrait.container.querySelector('.stage-v-ring .game-felt .felt-wordmark')).toHaveTextContent('CTECH');
  });

  test('shows one public playstyle badge and leaves unbadged seats unchanged', () => {
    const snapshot = snapshotForScenario('pre_flop');
    snapshot.seats.forEach(seat => {
      seat.playstyle_badge = undefined;
    });
    snapshot.seats[0].playstyle_badge = 'initiative';
    const {container} = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID}
                                           pot={0} bigBlind={50} nowMs={Date.now()} outcome={null}
                                           holdOutcomeOpen={false}
                                           announcement="Etapa: flop. Flop: ás de copas. Sua vez de agir"/>);
    expect(screen.getByText('Iniciativa')).toHaveAttribute('title', 'PFR representa pelo menos 70% do VPIP');
    expect(container.querySelectorAll('.seat-playstyle')).toHaveLength(1);
    expect(container.querySelector('.table-callout')).toHaveTextContent('Flop: ás de copas');
    expect(container.querySelector('.table-callout')).not.toHaveTextContent('Etapa: flop');
    expect(container.querySelector('.street-progress .is-current')).toHaveTextContent('Pré');
  });

  test('the felt names the current stage and, once armed, the next-hand countdown', () => {
    const waiting = render(<TableStage snapshot={snapshotForScenario('waiting')} viewer={MOCK_PLAYER_ID}
                                       pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}/>);
    expect(waiting.container.querySelector('.street-progress-label')).toHaveTextContent('Aguardando jogadores');
    expect(waiting.container.querySelector('.hand-outcome-ring-standalone')).not.toBeInTheDocument();
    waiting.unmount();

    // No personalized outcome yet (outcome={null}), so the countdown ring
    // falls back to the standalone corner dot instead of the felt center
    // (see HandOutcomeBanner: that center spot is where a winner's payout
    // chips float up from their seat, which is what pushed this ring out
    // of .street-progress in the first place).
    const snapshot = snapshotForScenario('complete');
    const armed = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID}
                                     pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}
                                     nextHandDeadlineMs={Date.now() + 5_000} nextHandDurationMs={5_000}/>);
    expect(armed.container.querySelector('.street-progress-label')).toHaveTextContent('Mão encerrada');
    expect(armed.container.querySelector('.hand-outcome-ring-standalone')).toBeInTheDocument();
  });

  test('queues the paid winner-card offer behind the primary hand outcome', async () => {
    useVerticalStage();
    const snapshot = snapshotForScenario('winner_cards');
    render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID} maxSeats={6}
      pot={0} bigBlind={50} nowMs={Date.now()} outcome={{key: 1, kind: 'fold'}} holdOutcomeOpen/>);
    expect(screen.queryByRole('button', {name: /Pedir a mão/})).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Minimizar resultado'}));
    expect(screen.getByRole('button', {name: /Pedir a mão/})).toBeInTheDocument();
  });

  test('the next-hand countdown rides on the outcome badge once a personalized result exists', () => {
    const outcome: HandOutcomeState = {key: 1, kind: 'win', handCategory: 'pair'};
    const {container, getByRole} = render(<HandOutcomeBanner outcome={outcome} holdOpen
      nextHandDeadlineMs={Date.now() + 5_000} nextHandDurationMs={5_000}/>);
    // Full card: ring lives on the dismiss button.
    expect(container.querySelector('.hand-outcome-dismiss .hand-outcome-ring')).toBeInTheDocument();
    fireEvent.click(getByRole('button', {name: 'Minimizar resultado'}));
    // Collapsed: ring moves with it onto the badge.
    const badgeRing = container.querySelector('.hand-outcome-badge .hand-outcome-ring');
    expect(badgeRing).toBeInTheDocument();
    expect(badgeRing?.querySelector('rect')).toHaveAttribute('rx', '20');
  });

  test('renders a speech bubble on the seat with the latest chat message', () => {
    const snapshot = snapshotForScenario('pre_flop');
    const speaker = snapshot.seats.find(seat => seat.player_id !== MOCK_PLAYER_ID);
    const {container} = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID}
                                           pot={0} bigBlind={50} nowMs={Date.now()} outcome={null}
                                           holdOutcomeOpen={false}
                                           chatBubbles={{[speaker!.player_id]: {id: 'chat-1', message: 'boa sorte'}}}/>);
    const bubble = container.querySelector(`[data-player-id="${speaker!.player_id}"] .seat-chat-bubble`);
    expect(bubble).toHaveTextContent('boa sorte');
    expect(container.querySelector(`[data-player-id="${MOCK_PLAYER_ID}"] .seat-chat-bubble`)).not.toBeInTheDocument();
  });


  test('lays the stage out vertically with the viewer as a separate hero seat', () => {
    useVerticalStage();
    const snapshot = snapshotForScenario('pre_flop');
    const {container} = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID}
      pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}/>);

    expect(container.querySelector('.game-table')).toHaveClass('stage-v');
    expect(container.querySelectorAll('.stage-v-ring .game-seat')).toHaveLength(snapshot.seats.length - 1);
    expect(container.querySelectorAll('.game-seat')).toHaveLength(snapshot.seats.length);
  });

  test('keeps every seat in the vertical ring when the viewer is only watching', () => {
    useVerticalStage();
    const snapshot = snapshotForScenario('pre_flop');
    const {container} = render(<TableStage snapshot={snapshot} viewer="not-seated"
      pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}/>);
    expect(container.querySelectorAll('.stage-v-ring .game-seat')).toHaveLength(snapshot.seats.length);
  });

  test.each([
    ['Mesa atualizada. Bia pagou 50', 'Bia pagou 50'],
    ['Etapa: pré-flop. Vez de Bia', 'Vez de Bia'],
    ['Bia colocou 100 fichas', 'Bia colocou 100 fichas'],
    ['Algo diferente aconteceu', 'Algo diferente aconteceu'],
  ])('reduces the announcement %s to its headline', (announcement, headline) => {
    const {container} = render(<TableStage snapshot={snapshotForScenario('pre_flop')} viewer={MOCK_PLAYER_ID}
      pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}
      announcement={announcement}/>);
    expect(container.querySelector('.table-callout')).toHaveTextContent(headline);
  });

  test('drops the dealer callout once the hand is over', () => {
    const {container} = render(<TableStage snapshot={snapshotForScenario('complete')} viewer={MOCK_PLAYER_ID}
      pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}
      announcement="Etapa: mão encerrada. Você ganhou 120 fichas"/>);
    expect(container.querySelector('.table-callout')).toBeNull();
  });

  test('names a stage the client does not know without crashing the pips', () => {
    const snapshot = {...snapshotForScenario('pre_flop'), stage: 'run_it_twice_runout'};
    const {container} = render(<TableStage snapshot={snapshot} viewer={MOCK_PLAYER_ID}
      pot={0} bigBlind={50} nowMs={Date.now()} outcome={null} holdOutcomeOpen={false}/>);
    expect(container.querySelector('.street-progress-label')).toHaveTextContent('run it twice runout');
    expect(container.querySelector('.street-progress .is-current')).toBeNull();
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
  test('offers an explicit close control inside the mobile panel', async () => {
    const onOpenChangeAction = vi.fn();
    render(<Chat open items={[]} onOpenChangeAction={onOpenChangeAction} onSendAction={vi.fn()}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Fechar painel de chat'}));
    expect(onOpenChangeAction).toHaveBeenCalledWith(false);
  });

  test('opens, resolves player names and sends a trimmed message', async () => {
    const user = userEvent.setup();
    const onOpenChangeAction = vi.fn();
    const onSend = vi.fn(() => true);
    render(<Chat open items={[{id: '1', player: 'p2', message: 'Boa mão'}]}
                 seats={[{player_id: 'p2', name: 'Bia'}]}
                 onOpenChangeAction={onOpenChangeAction} onSendAction={onSend}/>);
    expect(screen.getByRole('log')).toHaveTextContent('BiaBoa mão');
    await user.type(screen.getByLabelText('Mensagem para a mesa'), '  olá  ');
    expect(screen.getByText('7/50')).toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: 'Enviar mensagem'}));
    expect(onSend).toHaveBeenCalledWith('olá');
    expect(screen.getByLabelText('Mensagem para a mesa')).toHaveValue('');
    expect(screen.getByText('0/50')).toBeInTheDocument();
  });

  test('caps the message field at 50 characters', () => {
    render(<Chat open items={[]} onOpenChangeAction={vi.fn()} onSendAction={vi.fn()}/>);
    expect(screen.getByLabelText('Mensagem para a mesa')).toHaveAttribute('maxLength', '50');
  });
  
  test('reports send failure and toggles the controlled panel', async () => {
    const onOpenChangeAction = vi.fn();
    render(<Chat open connected items={[]} onOpenChangeAction={onOpenChangeAction} onSendAction={() => false}/>);
    await userEvent.type(screen.getByLabelText('Mensagem para a mesa'), 'teste');
    await userEvent.click(screen.getByRole('button', {name: 'Enviar mensagem'}));
    expect(screen.getByRole('alert')).toHaveTextContent('Mensagem não enviada');
    await userEvent.click(screen.getByRole('button', {name: 'Fechar chat'}));
    expect(onOpenChangeAction).toHaveBeenCalledWith(false);
  });

  test('shows an empty state and the unread dot only while the panel is closed', () => {
    const {container, rerender} = render(<Chat open items={[]} seats={[]} viewerId="viewer"
      onOpenChangeAction={vi.fn()} onSendAction={vi.fn()}/>);
    expect(screen.getByText('Nenhuma mensagem ainda. Diga um oi para a mesa.')).toBeInTheDocument();
    expect(container.querySelector('.chat-unread-dot')).toBeNull();

    rerender(<Chat open={false} items={[{id: '1', player: 'p2', message: 'oi'}]} seats={[]} viewerId="viewer"
      onOpenChangeAction={vi.fn()} onSendAction={vi.fn()}/>);
    expect(container.querySelector('.chat-unread-dot')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('disse: oi');

    rerender(<Chat open items={[{id: '1', player: 'p2', message: 'oi'}]} seats={[]} viewerId="viewer"
      onOpenChangeAction={vi.fn()} onSendAction={vi.fn()}/>);
    expect(container.querySelector('.chat-unread-dot')).toBeNull();
  });

  test('refuses to send while disconnected and clears the error on the next keystroke', async () => {
    const onSendAction = vi.fn(() => true);
    render(<Chat open items={[]} connected={false} seats={[]} onOpenChangeAction={vi.fn()}
      onSendAction={onSendAction}/>);
    const input = screen.getByLabelText('Mensagem para a mesa');
    expect(input).toBeDisabled();
    expect(input).toHaveAttribute('placeholder', 'Reconectando…');

    fireEvent.change(input, {target: {value: 'olá'}});
    fireEvent.submit(input.closest('form')!);
    expect(onSendAction).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toBeInTheDocument();

    fireEvent.change(input, {target: {value: 'olá!'}});
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  test('ignores a submit with nothing but whitespace', () => {
    const onSendAction = vi.fn(() => true);
    render(<Chat open connected items={[]} seats={[]} onOpenChangeAction={vi.fn()} onSendAction={onSendAction}/>);
    const input = screen.getByLabelText('Mensagem para a mesa');
    fireEvent.change(input, {target: {value: '   '}});
    fireEvent.submit(input.closest('form')!);
    expect(onSendAction).not.toHaveBeenCalled();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  test('dismisses on Escape and on an outside click, but not an inside one', async () => {
    const user = userEvent.setup();
    const onOpenChangeAction = vi.fn();
    render(<Chat open items={[]} onOpenChangeAction={onOpenChangeAction} onSendAction={vi.fn()}/>);

    await user.click(screen.getByLabelText('Mensagem para a mesa'));
    expect(onOpenChangeAction).not.toHaveBeenCalled();

    await user.keyboard('{Escape}');
    expect(onOpenChangeAction).toHaveBeenCalledWith(false);
    expect(screen.getByRole('button', {name: 'Fechar chat'})).toHaveFocus();

    onOpenChangeAction.mockClear();
    await user.click(document.body);
    expect(onOpenChangeAction).toHaveBeenCalledWith(false);
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

  test('explains a showdown win decided by kicker too, not just a loss', () => {
    renderOutcome({
      key: 6,
      kind: 'win',
      viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'],
      beatenCards: ['AS', 'AC', 'KH', 'QD', '3D'],
    });
    expect(screen.getByText('Mesma combinação, o kicker decidiu.')).toBeInTheDocument();
    // Pair alone is 2 cards; the deciding kicker(s) must render too, or the
    // note above has nothing on screen to point at.
    expect(document.querySelectorAll('.hand-outcome-card-slot').length).toBe(5);
    expect(document.querySelectorAll('.is-kicker').length).toBe(3);
  });

  test('does not claim a kicker decision on a win with no revealed opponent hand', () => {
    renderOutcome({
      key: 7,
      kind: 'win',
      viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'],
    });
    expect(screen.queryByText('Mesma combinação, o kicker decidiu.')).not.toBeInTheDocument();
  });

  test('names each side pot\'s actual winner in a mixed result, not the viewer twice', () => {
    renderOutcome({
      key: 8,
      kind: 'mixed',
      handCategory: 'two_pair',
      viewerCards: ['AH', 'AD', 'KC', 'KD', '2D'],
      viewerHoleCards: ['AH', 'AD'],
      pots: [
        {won: true},
        {won: false, winnerName: 'Bia', category: 'straight', winningCards: ['9H', 'TD', 'JC', 'QS', 'KH']},
      ],
    });
    expect(screen.getByText(/Pote 1/)).toBeInTheDocument();
    expect(screen.getByText(/Pote 2/)).toBeInTheDocument();
    expect(screen.getAllByText(/Bia/)[0]).toBeInTheDocument();
    expect(screen.getByText('Sequência')).toBeInTheDocument();
  });

  test('shows a 2-way chop as a comparison between the viewer and the other tied hand', () => {
    renderOutcome({
      key: 9,
      kind: 'tie',
      viewerCards: ['AH', 'KD', 'QS', 'JC', 'TH'],
      viewerHoleCards: ['AH', 'KD'],
      tiedWith: [{name: 'Bia', cards: ['AS', 'KH', 'QD', 'JS', 'TC']}],
    });
    expect(screen.getByText('Você')).toBeInTheDocument();
    expect(screen.getByText('Bia')).toBeInTheDocument();
    expect(document.querySelectorAll('.hand-outcome-comparison-row').length).toBe(2);
  });

  test('shows a 3+-way chop with one row per tied hand', () => {
    renderOutcome({
      key: 10,
      kind: 'tie',
      viewerCards: ['AH', 'KD', 'QS', 'JC', 'TH'],
      viewerHoleCards: ['AH', 'KD'],
      tiedWith: [
        {name: 'Bia', cards: ['AS', 'KH', 'QD', 'JS', 'TC']},
        {name: 'Caio', cards: ['AC', 'KS', 'QH', 'JD', 'TS']},
      ],
    });
    expect(screen.getByText('Bia')).toBeInTheDocument();
    expect(screen.getByText('Caio')).toBeInTheDocument();
    expect(document.querySelectorAll('.hand-outcome-comparison-row').length).toBe(3);
  });

  test('falls back to the single combination when the tied opponent never revealed', () => {
    renderOutcome({key: 11, kind: 'tie', handCategory: 'straight',
      winningCards: ['AH', 'KD', 'QS', 'JC', 'TH']});
    expect(document.querySelectorAll('.hand-outcome-comparison-row').length).toBe(0);
    expect(document.querySelectorAll('.hand-outcome-card-slot').length).toBe(5);
  });

  test('sr-only announcement names the hand category and a kicker decision on a win', () => {
    renderOutcome({
      key: 12,
      kind: 'win',
      viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'],
      beatenCards: ['AS', 'AC', 'KH', 'QD', '3D'],
    });
    expect(screen.getByRole('status')).toHaveTextContent('Você venceu com Par, decidido pelo kicker.');
  });

  test('sr-only announcement names the winner\'s category and kicker decision on a loss', () => {
    renderOutcome({
      key: 13,
      kind: 'lose',
      winnerName: 'Bia',
      viewerCards: ['AH', 'AD', 'KC', 'QS', '2D'],
      winningCards: ['AS', 'AC', 'KH', 'QD', '3D'],
      viewerHoleCards: ['AH', 'KC'],
    });
    expect(screen.getByRole('status')).toHaveTextContent('Vencedor: Bia com Par, decidido pelo kicker.');
  });

  test('sr-only announcement names the tied hand category', () => {
    renderOutcome({key: 14, kind: 'tie', handCategory: 'straight',
      winningCards: ['AH', 'KD', 'QS', 'JC', 'TH']});
    expect(screen.getByRole('status')).toHaveTextContent('Pote dividido com Sequência.');
  });

  test('shows the refund amount on a win, not just tie/mixed', () => {
    renderOutcome({
      key: 15, kind: 'win', handCategory: 'pair',
      winningCards: ['AH', 'AD', 'KC', 'QS', '2D'],
      wonAmount: 100, refundAmount: 25,
    });
    expect(screen.getByText('100 fichas ganhas e 25 fichas devolvidas')).toBeInTheDocument();
  });

  test('shows the refund amount on a loss, not just tie/mixed', () => {
    renderOutcome({
      key: 16, kind: 'lose', winnerName: 'Bia', refundAmount: 40,
    });
    expect(screen.getByText('40 fichas devolvidas')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('40 fichas devolvidas');
  });

  test('a win also tells the player the next hand is coming, like every other outcome', () => {
    renderOutcome({
      key: 17, kind: 'win', handCategory: 'pair',
      winningCards: ['AH', 'AD', 'KC', 'QS', '2D'],
    });
    expect(screen.getByText('A próxima mão já está a caminho.')).toBeInTheDocument();
  });

  test('dismissing the card collapses it to a reopenable badge', async () => {
    const user = userEvent.setup();
    renderOutcome({key: 18, kind: 'win', handCategory: 'pair', winningCards: ['AH', 'AD', 'KC', 'QS', '2D']});

    await user.click(screen.getByRole('button', {name: 'Minimizar resultado'}));
    expect(document.querySelector('.hand-outcome-card')).not.toBeInTheDocument();
    const badge = screen.getByRole('button', {name: 'Ver resultado: você venceu'});
    expect(badge).toBeInTheDocument();

    await user.click(badge);
    expect(document.querySelector('.hand-outcome-card')).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Ver resultado/})).not.toBeInTheDocument();
  });

  test('a dismissal does not carry over to the next hand\'s outcome', async () => {
    const user = userEvent.setup();
    const {rerender} = render(<HandOutcomeBanner
      outcome={{key: 19, kind: 'lose', winnerName: 'Bia'}} holdOpen/>);
    await user.click(screen.getByRole('button', {name: 'Minimizar resultado'}));
    expect(document.querySelector('.hand-outcome-card')).not.toBeInTheDocument();

    rerender(<HandOutcomeBanner outcome={{key: 20, kind: 'win', handCategory: 'pair'}} holdOpen/>);
    expect(document.querySelector('.hand-outcome-card')).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /Ver resultado/})).not.toBeInTheDocument();
  });
});
