import {act, fireEvent, render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {type ActionAvailability, ActionBar} from './ActionBar';

const allActions: ActionAvailability = {fold: true, check: true, call: true, raise: true};

function renderActionBar(overrides: Partial<React.ComponentProps<typeof ActionBar>> = {}) {
  const onActAction = vi.fn(() => true);
  const onPreselectAction = vi.fn(() => true);
  const props: React.ComponentProps<typeof ActionBar> = {
    onActAction,
    available: allActions,
    callAmount: 75,
    minRaise: 150,
    maxRaise: 1000,
    raiseStep: 25,
    effectiveStack: 1000,
    raisePresets: [{label: 'Mín', value: 150}, {label: '½ pote', value: 250}, {label: 'Máx', value: 1000}],
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
    onPreselectAction,
    timeBankMs: 30_000,
    voiceCommands: false,
    ...overrides,
  };
  const view = render(<ActionBar {...props}/>);
  return {onActAction, onPreselectAction, view, props};
}

const slider = () => screen.getByRole('slider');

afterEach(() => {
  vi.useRealTimers();
  vi.mocked(window.matchMedia).mockImplementation(query => ({
    matches: false, media: query, onchange: null, addListener: vi.fn(), removeListener: vi.fn(),
    addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
  } as unknown as MediaQueryList));
});

describe('ActionBar raise controls', () => {
  test('jumps the amount to a preset and keeps the raise button in sync', async () => {
    renderActionBar();
    await userEvent.click(screen.getByRole('button', {name: '½ pote'}));
    expect(slider()).toHaveValue('250');
    expect(slider()).toHaveAttribute('aria-valuetext', 'Total 250 fichas');
    expect(document.querySelector('.bet-commitment-meter i')).toHaveStyle('--bet-progress: 0.25');
  });

  test('reads the maximum raise as an all-in instead of a number', async () => {
    renderActionBar();
    await userEvent.click(screen.getByRole('button', {name: 'Máx'}));
    expect(screen.getByRole('button', {name: /All In/})).toBeInTheDocument();
    expect(slider()).toHaveAttribute('aria-valuetext', 'Total 1.000 fichas, All In');
    expect(document.querySelector('.bet-output.is-all-in .bet-commitment-meter i')).toHaveStyle('--bet-progress: 1');
  });

  test('keeps the explicit minimum and maximum names when clamped presets duplicate them', () => {
    renderActionBar({
      minRaise: 100,
      maxRaise: 1000,
      raisePresets: [
        {label: 'Mín', value: 100}, {label: '½ pote', value: 75},
        {label: 'Pote', value: 1200}, {label: 'Máx', value: 1000},
      ],
    });
    expect(screen.getByRole('button', {name: 'Mín'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Máx'})).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: '½ pote'})).not.toBeInTheDocument();
  });

  test('clamps an amount the server no longer allows and says which bound it hit', () => {
    const {view} = renderActionBar();
    fireEvent.change(slider(), {target: {value: '400'}});
    expect(document.querySelector('.bet-clamped-note')).toBeNull();

    // Another player's raise moved the legal window past the chosen amount.
    view.rerender(<ActionBar {...renderActionBarProps({minRaise: 600, maxRaise: 1000})}/>);
    expect(document.querySelector('.bet-clamped-note')).toHaveTextContent('ajustado ao mínimo');

    view.rerender(<ActionBar {...renderActionBarProps({minRaise: 150, maxRaise: 300})}/>);
    expect(document.querySelector('.bet-clamped-note')).toHaveTextContent('ajustado ao máximo');
  });

  test.each([
    ['ArrowUp', 1000],
    ['ArrowDown', 150],
    ['a', 1000],
  ])('keyboard %s jumps the raise amount to a bound', (key, expected) => {
    renderActionBar();
    act(() => void fireEvent.keyDown(window, {key}));
    expect(slider()).toHaveValue(String(expected));
  });

  test('the half-pot shortcut uses the server preset, and R submits the raise', () => {
    const {onActAction} = renderActionBar();
    act(() => void fireEvent.keyDown(window, {key: 'h'}));
    expect(slider()).toHaveValue('250');

    act(() => void fireEvent.keyDown(window, {key: 'r'}));
    expect(onActAction).toHaveBeenCalledWith('raise', 250);
  });

  test('ctrl+arrow steps the amount by a bigger stride than a bare arrow', () => {
    renderActionBar();
    act(() => void fireEvent.keyDown(window, {key: 'ArrowRight'}));
    const single = Number((slider() as HTMLInputElement).value);

    act(() => void fireEvent.keyDown(window, {key: 'ArrowRight', ctrlKey: true}));
    expect(Number((slider() as HTMLInputElement).value)).toBeGreaterThan(single);

    act(() => void fireEvent.keyDown(window, {key: 'ArrowLeft'}));
    expect(Number((slider() as HTMLInputElement).value)).toBeLessThan(
      Number((slider() as HTMLInputElement).value) + 1);
  });

  test('ignores raise shortcuts while the controls are inactive', () => {
    const {onActAction} = renderActionBar({isTurn: false});
    act(() => void fireEvent.keyDown(window, {key: 'r'}));
    expect(onActAction).not.toHaveBeenCalled();
  });

  test('on a compact screen the raise button first expands the slider, then submits', async () => {
    vi.mocked(window.matchMedia).mockReturnValue({matches: true} as MediaQueryList);
    const {onActAction} = renderActionBar();

    await userEvent.click(screen.getByRole('button', {name: /Aumentar/}));
    expect(onActAction).not.toHaveBeenCalled();
    expect(screen.getByRole('button', {name: 'Cancelar'})).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', {name: /Aumentar para/}));
    expect(onActAction).toHaveBeenCalledWith('raise', 150);

    await userEvent.click(screen.getByRole('button', {name: 'Cancelar'}));
    expect(screen.queryByRole('button', {name: 'Cancelar'})).not.toBeInTheDocument();
  });

  test('adjusts a mobile bet with explicit plus and minus buttons and clamps to the limits', async () => {
    renderActionBar({minRaise: 150, maxRaise: 200, raiseStep: 25});
    const plus = screen.getByRole('button', {name: 'Mais fichas'});
    const minus = screen.getByRole('button', {name: 'Menos fichas'});
    await userEvent.click(plus);
    await userEvent.click(plus);
    await userEvent.click(plus);
    expect(slider()).toHaveValue('200');
    await userEvent.click(minus);
    expect(slider()).toHaveValue('175');
  });

  test('accelerates a held mobile bet adjustment and stops on pointer release', () => {
    vi.useFakeTimers();
    renderActionBar({maxRaise: 5000, raiseStep: 25});
    const plus = screen.getByRole('button', {name: 'Mais fichas'});
    fireEvent.pointerDown(plus, {button: 0});
    act(() => vi.advanceTimersByTime(420 + 12 * 130));
    fireEvent.pointerUp(plus);
    const released = Number((slider() as HTMLInputElement).value);
    expect(released).toBeGreaterThan(150 + 12 * 25);
    act(() => vi.advanceTimersByTime(520));
    expect(slider()).toHaveValue(String(released));
  });

  test('ignores secondary-pointer holds and suppresses the click emitted after a completed hold', () => {
    vi.useFakeTimers();
    renderActionBar({maxRaise: 5000, raiseStep: 25});
    const plus = screen.getByRole('button', {name: 'Mais fichas'});

    fireEvent.pointerDown(plus, {button: 2});
    act(() => vi.advanceTimersByTime(600));
    expect(slider()).toHaveValue('150');

    fireEvent.pointerDown(plus, {button: 0});
    act(() => vi.advanceTimersByTime(420));
    fireEvent.pointerUp(plus);
    expect(slider()).toHaveValue('175');
    fireEvent.click(plus);
    expect(slider()).toHaveValue('175');
  });

  test('holding an arrow key accelerates the raise amount and stops on key release', () => {
    vi.useFakeTimers();
    renderActionBar({maxRaise: 5000, raiseStep: 25});
    act(() => void fireEvent.keyDown(window, {key: 'ArrowRight'}));
    expect(slider()).toHaveValue('175');

    act(() => vi.advanceTimersByTime(420 + 12 * 130));
    const held = Number((slider() as HTMLInputElement).value);
    expect(held).toBeGreaterThan(175 + 12 * 25);

    act(() => void fireEvent.keyUp(window, {key: 'ArrowRight'}));
    act(() => vi.advanceTimersByTime(520));
    expect(slider()).toHaveValue(String(held));
  });

  test('a held ctrl+arrow uses the faster stride, and a window blur ends the hold', () => {
    vi.useFakeTimers();
    renderActionBar({maxRaise: 50000, raiseStep: 25});
    const heldFrom = (options: {ctrlKey?: boolean}) => {
      act(() => void fireEvent.keyDown(window, {key: 'ArrowUp'}));
      act(() => void fireEvent.keyDown(window, {key: 'ArrowLeft', ...options}));
      act(() => vi.advanceTimersByTime(420 + 4 * 130));
      act(() => void fireEvent.keyUp(window, {key: 'ArrowLeft'}));
      return 50000 - Number((slider() as HTMLInputElement).value);
    };
    const fast = heldFrom({ctrlKey: true});
    const plain = heldFrom({});
    expect(plain).toBeGreaterThan(0);
    expect(fast).toBe(plain * 3);

    act(() => void fireEvent.keyDown(window, {key: 'ArrowLeft'}));
    act(() => vi.advanceTimersByTime(420 + 130));
    act(() => void fireEvent.blur(window));
    const stopped = Number((slider() as HTMLInputElement).value);
    act(() => vi.advanceTimersByTime(520));
    expect(slider()).toHaveValue(String(stopped));
  });

  test('falls back to first and last duplicate presets when explicit bound names are absent', () => {
    renderActionBar({
      minRaise: 100,
      maxRaise: 1000,
      raisePresets: [
        {label: 'Abrir', value: 50}, {label: 'Dobrar', value: 100},
        {label: 'Pote', value: 1200}, {label: 'Tudo', value: 1400},
      ],
    });
    expect(screen.getByRole('button', {name: 'Abrir'})).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Dobrar'})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'Pote'})).not.toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Tudo'})).toBeInTheDocument();
  });

  test('closes mobile sizing when the action scope changes', async () => {
    vi.mocked(window.matchMedia).mockReturnValue({matches: true} as MediaQueryList);
    const {view} = renderActionBar();
    await userEvent.click(screen.getByRole('button', {name: /Aumentar/}));
    expect(screen.getByRole('button', {name: 'Cancelar'})).toBeInTheDocument();

    view.rerender(<ActionBar {...renderActionBarProps({actionKey: 'hand-1:turn'})}/>);
    expect(screen.queryByRole('button', {name: 'Cancelar'})).not.toBeInTheDocument();
  });

  test('shows the all-in spinner label while the raise is in flight', () => {
    renderActionBar({pending: 'raise', minRaise: 1000, maxRaise: 1000});
    expect(screen.getByRole('button', {name: /Indo All In…/})).toBeInTheDocument();
  });
});

describe('ActionBar time bank', () => {
  test('counts the reserve down while it is being spent', () => {
    vi.useFakeTimers();
    const now = Date.now();
    renderActionBar({isTurn: true, actionBaseDeadlineMs: now - 1000, actionDeadlineMs: now + 8000});
    const timer = screen.getByRole('timer');
    expect(timer).toHaveAttribute('aria-label', expect.stringContaining('Time bank em uso'));
    expect(timer).toHaveTextContent('Time bank em uso');

    act(() => void vi.advanceTimersByTime(1000));
    expect(timer).toHaveTextContent(/[0-7]s/);
  });

  test('shows the untouched reserve while the normal clock still runs', () => {
    const now = Date.now();
    renderActionBar({isTurn: true, actionBaseDeadlineMs: now + 5000, actionDeadlineMs: now + 15000});
    expect(screen.getByRole('timer')).toHaveTextContent('Reserva pronta 30s');
  });

  test('labels the reserve as idle when it is not the viewer turn', () => {
    renderActionBar({isTurn: false, timeBankMs: 12_000});
    expect(screen.getByRole('timer')).toHaveTextContent('Sua reserva 12s');
    expect(screen.getByRole('timer')).toHaveClass('inactive');
  });
});

describe('ActionBar preselection', () => {
  test('executes a prepared fold the moment the turn arrives and hides the manual controls', () => {
    const {onActAction} = renderActionBar({
      canPreselect: false, selectionScope: 'hand-1:flop', preselection: 'fold', isTurn: true,
    });
    expect(onActAction).toHaveBeenCalledWith('fold');
    expect(screen.getByText('Executando sua ação preparada…')).toBeInTheDocument();
    expect(document.querySelector('.action-choices')).toBeNull();
  });

  test('executes a prepared all-in as a raise to the maximum', () => {
    const {onActAction} = renderActionBar({
      canPreselect: false, selectionScope: 'hand-1:flop', preselection: 'all_in', isTurn: true,
    });
    expect(onActAction).toHaveBeenCalledWith('raise', 1000);
  });

  test('waits to execute a prepared action while disconnected or another action is pending', () => {
    const disconnected = renderActionBar({
      selectionScope: 'hand-1:flop', preselection: 'fold', isTurn: true, connected: false,
    });
    expect(disconnected.onActAction).not.toHaveBeenCalled();
    disconnected.view.unmount();

    const pending = renderActionBar({
      selectionScope: 'hand-1:flop', preselection: 'fold', isTurn: true, pending: 'call',
    });
    expect(pending.onActAction).not.toHaveBeenCalled();
  });

  test('keeps manual controls when a prepared action is no longer legal', () => {
    const {onActAction} = renderActionBar({
      selectionScope: 'hand-1:flop', preselection: 'call', preselectionAmount: 50,
      callAmount: 75, isTurn: true,
    });
    expect(onActAction).not.toHaveBeenCalled();
    expect(document.querySelector('.action-choices')).toBeInTheDocument();
  });

  test('releases the manual controls again once the prepared action is rejected', () => {
    renderActionBar({
      selectionScope: 'hand-1:flop', preselection: 'fold', isTurn: true,
      error: {code: 'stale_state', message: 'Estado desatualizado.'},
    });
    expect(document.querySelector('.action-choices')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toBeInTheDocument();
  });

  test('prepares an action for the next turn and clears it when chosen again', async () => {
    const {onPreselectAction, view} = renderActionBar({
      canPreselect: true, selectionScope: 'hand-1:flop', isTurn: false,
    });
    const option = () => within(document.querySelector<HTMLElement>('.action-preselectors')!)
      .getByRole('button', {name: 'FoldDesistir quando chegar sua vez'});
    await userEvent.click(option());
    expect(onPreselectAction).toHaveBeenCalledWith('fold', 0);
    expect(option()).toHaveAttribute('aria-pressed', 'false');

    view.rerender(<ActionBar {...renderActionBarProps({
      canPreselect: true, selectionScope: 'hand-1:flop', isTurn: false,
      preselection: 'fold', onPreselectAction,
    })}/>);
    expect(option()).toHaveAttribute('aria-pressed', 'true');

    await userEvent.click(option());
    expect(onPreselectAction).toHaveBeenLastCalledWith(null, 0);
  });

  test('drops the stack detail from the turn prompt when the viewer is all-in', () => {
    renderActionBar({effectiveStack: 0});
    expect(screen.getByText('Sua vez de agir.')).toBeInTheDocument();
  });
});

describe('ActionBar preselection shortcuts', () => {
  test.each([
    ['x', 'check_fold'],
    ['f', 'fold'],
    ['a', 'all_in'],
  ])('%s selects the matching preselection', (key, selection) => {
    const {onPreselectAction} = renderActionBar({canPreselect: true, isTurn: false});
    act(() => void fireEvent.keyDown(window, {key}));
    expect(onPreselectAction).toHaveBeenCalledWith(selection, 0);
  });

  test('c selects call_any directly when there is nothing fixed to call', () => {
    const {onPreselectAction} = renderActionBar({
      canPreselect: true, isTurn: false, supportsCallPreselection: true, prospectiveCallAmount: 0,
    });
    act(() => void fireEvent.keyDown(window, {key: 'c'}));
    expect(onPreselectAction).toHaveBeenCalledWith('call_any', 0);
  });

  test('c cycles call, then call_any, then clears when a fixed call exists', () => {
    const {onPreselectAction, view} = renderActionBar({
      canPreselect: true, isTurn: false, supportsCallPreselection: true, prospectiveCallAmount: 75,
    });
    act(() => void fireEvent.keyDown(window, {key: 'c'}));
    expect(onPreselectAction).toHaveBeenLastCalledWith('call', 75);

    view.rerender(<ActionBar {...renderActionBarProps({
      canPreselect: true, isTurn: false, supportsCallPreselection: true, prospectiveCallAmount: 75,
      preselection: 'call', preselectionAmount: 75, onPreselectAction,
    })}/>);
    act(() => void fireEvent.keyDown(window, {key: 'c'}));
    expect(onPreselectAction).toHaveBeenLastCalledWith('call_any', 0);

    view.rerender(<ActionBar {...renderActionBarProps({
      canPreselect: true, isTurn: false, supportsCallPreselection: true, prospectiveCallAmount: 75,
      preselection: 'call_any', onPreselectAction,
    })}/>);
    act(() => void fireEvent.keyDown(window, {key: 'c'}));
    expect(onPreselectAction).toHaveBeenLastCalledWith(null, 0);
  });

  test('pressing the same key again clears the selection', () => {
    const {onPreselectAction, view} = renderActionBar({canPreselect: true, isTurn: false, preselection: 'fold'});
    view.rerender(<ActionBar {...renderActionBarProps({
      canPreselect: true, isTurn: false, preselection: 'fold', onPreselectAction,
    })}/>);
    act(() => void fireEvent.keyDown(window, {key: 'f'}));
    expect(onPreselectAction).toHaveBeenCalledWith(null, 0);
  });

  test('has no effect when preselection is unavailable', () => {
    const {onPreselectAction} = renderActionBar({canPreselect: false, isTurn: false});
    act(() => void fireEvent.keyDown(window, {key: 'x'}));
    expect(onPreselectAction).not.toHaveBeenCalled();
  });

  test('has no effect while typing in an input', () => {
    const {onPreselectAction} = renderActionBar({canPreselect: true, isTurn: false});
    const input = document.createElement('input');
    document.body.appendChild(input);
    act(() => void fireEvent.keyDown(input, {key: 'x'}));
    expect(onPreselectAction).not.toHaveBeenCalled();
    document.body.removeChild(input);
  });
});

function renderActionBarProps(overrides: Partial<React.ComponentProps<typeof ActionBar>>) {
  return {
    onActAction: vi.fn(() => true), available: allActions, callAmount: 75, minRaise: 150, maxRaise: 1000,
    raiseStep: 25, effectiveStack: 1000, raisePresets: [{label: 'Mín', value: 150}],
    actionKey: 'hand-1:pre_flop', isTurn: true, connected: true, pending: null, error: null,
    onDismissErrorAction: vi.fn(), canPreselect: false, supportsCallPreselection: false, selectionScope: '',
    preselection: null, preselectionAmount: 0, prospectiveCallAmount: 0, onPreselectAction: vi.fn(() => true),
    timeBankMs: 30_000, voiceCommands: false, ...overrides,
  } as React.ComponentProps<typeof ActionBar>;
}
