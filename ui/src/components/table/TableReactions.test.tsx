import {act, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import type {SeatView} from '@/lib/api/table';
import type {ReactionCatalogEntry, ReactionPurchase} from '@/lib/api/reactionPurchases';
import {TABLE_REACTIONS, type TableReactionEvent, type TableReactionID} from '@/lib/reactions';
import {HOVER_PANEL_CLOSE_DELAY_MS} from '@/lib/hooks/useHoverPanel';
import {registerSeatElement, seatCenter} from '@/lib/seatRects';
import {TableReactions} from './TableReactions';

vi.mock('@/lib/utils', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/utils')>(),
  isHoverCapable: () => true,
}));

const viewer = seat('viewer');
const opponent = seat('opponent');

function seat(playerId: string): SeatView {
  return {
    player_id: playerId,
    name: playerId,
    stack: 1_000,
    state: 'active',
    contributed: 0,
  };
}

/** Stands in for `Seat`, publishing its element the same way through the
 * shared registry — no class/data-attribute contract to keep in sync. */
function StubSeat({playerId}: {playerId: string}) {
  return <div ref={node => registerSeatElement(playerId, node)}/>;
}

/** A seat whose viewport rect is fixed, for asserting the reaction geometry
 * jsdom's own zero rects can't express. */
function seatNode(left: number) {
  const node = document.createElement('div');
  node.getBoundingClientRect = () => ({left, top: 0, width: 40, height: 40, right: left + 40,
    bottom: 40, x: left, y: 0, toJSON: () => undefined}) as DOMRect;
  return node;
}

function renderReactions(overrides: Partial<React.ComponentProps<typeof TableReactions>> = {}) {
  const callbacks = {
    onQuickSendAction: vi.fn(),
    onPendingReactionChangeAction: vi.fn(),
    onOpenChangeAction: vi.fn(),
    onLockedReactionAction: vi.fn(),
    onFavoriteReactionsChangeAction: vi.fn(async () => undefined),
  };
  const props: React.ComponentProps<typeof TableReactions> = {
    items: [],
    seats: [viewer, opponent],
    viewerId: viewer.player_id,
    connected: true,
    coolingDown: false,
    pendingReaction: null,
    open: true,
    favorites: [],
    ...callbacks,
    ...overrides,
  };
  const rendered = render(<>
    <StubSeat playerId={viewer.player_id}/>
    <StubSeat playerId={opponent.player_id}/>
    <TableReactions {...props}/>
  </>);
  return {...rendered, callbacks, props};
}

describe('TableReactions Poker Theater', () => {
  test('separates self tells from targeted gestures and sends each through the correct flow', async () => {
    const user = userEvent.setup();
    const {callbacks} = renderReactions();

    expect(screen.getByRole('tab', {name: /Na minha cadeira.*13 tells/})).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('button', {name: /Coração all-in/})).toBeEnabled();
    expect(screen.queryByRole('button', {name: /Boa leitura/})).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', {name: /Coração all-in/}));
    expect(callbacks.onQuickSendAction).toHaveBeenCalledWith('heartbeat');
    expect(callbacks.onOpenChangeAction).toHaveBeenCalledWith(false);

    await user.click(screen.getByRole('tab', {name: /Mandar para alguém.*17 gestos/}));
    expect(screen.getByRole('tabpanel', {name: 'Reações para outro jogador'})).toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: /Boa leitura/}));
    expect(callbacks.onPendingReactionChangeAction).toHaveBeenCalledWith('spotlight');
    expect(callbacks.onQuickSendAction).not.toHaveBeenCalledWith('spotlight');
  });

  test('explains and disables targeted reactions when nobody can receive one', async () => {
    const user = userEvent.setup();
    renderReactions({seats: [viewer]});

    await user.click(screen.getByRole('tab', {name: /Mandar para alguém/}));
    expect(screen.getByText('Outro jogador precisa estar sentado para receber um gesto.')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: /Passar a coroa/})).toBeDisabled();
  });

  test('disables the catalog while disconnected or cooling down', () => {
    const {rerender, props} = renderReactions({connected: false});
    expect(screen.getByRole('button', {name: /Aplausos/})).toBeDisabled();

    rerender(<TableReactions {...props} connected coolingDown/>);
    expect(screen.getByRole('button', {name: /Aplausos/})).toBeDisabled();
  });

  test('shows loading, unavailable, locked and owned premium states', async () => {
    const user = userEvent.setup();
    const catalog: ReactionCatalogEntry[] = [
      {id: 'cold', premium: true, owned: false},
      {id: 'fire', premium: true, owned: true},
    ];
    const {callbacks, rerender, props} = renderReactions({premiumEnabled: true, catalog});

    const cold = screen.getByRole('button', {name: /Frio na mesa/});
    expect(within(cold).getByLabelText('Premium bloqueada')).toBeInTheDocument();
    await user.click(cold);
    expect(callbacks.onLockedReactionAction).toHaveBeenCalledWith(catalog[0]);

    const fire = screen.getByRole('button', {name: /Pegando fogo/});
    expect(within(fire).getByLabelText('Premium liberada')).toBeInTheDocument();
    await user.click(fire);
    expect(callbacks.onQuickSendAction).toHaveBeenCalledWith('fire');

    rerender(<TableReactions {...props} premiumEnabled premiumLoading catalog={catalog}/>);
    expect(screen.getByRole('button', {name: /Frio na mesa/})).toBeDisabled();
    expect(screen.getAllByLabelText('Carregando').length).toBeGreaterThan(0);

    rerender(<TableReactions {...props} premiumEnabled catalog={[]}/>);
    expect(screen.getByRole('button', {name: /Frio na mesa/})).toBeDisabled();
  });

  test('blocks a refunding targeted premium reaction', async () => {
    const user = userEvent.setup();
    const purchases: ReactionPurchase[] = [{
      purchase_id: 'purchase-1',
      reaction_id: 'poop',
      method: 'fichas',
      status: 'refunding',
    }];
    renderReactions({
      premiumEnabled: true,
      catalog: [{id: 'poop', premium: true, owned: false}],
      purchases,
    });

    await user.click(screen.getByRole('tab', {name: /Mandar para alguém/}));
    expect(screen.getByRole('button', {name: /Jogar cocô/})).toBeDisabled();
  });

  test('renders favorites, opens their editor and persists a new selection', async () => {
    const user = userEvent.setup();
    const save = vi.fn(async () => undefined);
    renderReactions({
      favorites: ['heartbeat', 'crown'],
      onFavoriteReactionsChangeAction: save,
    });

    const favorites = screen.getByRole('group', {name: 'Reações favoritas'});
    await user.click(within(favorites).getByTitle('Coração all-in'));
    expect(screen.getByText('Teatro da mesa')).toBeInTheDocument();

    await user.click(screen.getByRole('button', {name: 'Editar reações favoritas'}));
    expect(screen.getByRole('dialog', {name: 'Atalhos de reação'})).toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: 'Modo tubarão'}));
    await user.click(screen.getByRole('button', {name: 'Salvar atalhos'}));

    expect(save).toHaveBeenCalledWith(['heartbeat', 'crown', 'shark']);
  });

  test('hides and restores live effects while preserving the preference', async () => {
    const user = userEvent.setup();
    const item: TableReactionEvent = {
      id: 'reaction-1',
      playerId: viewer.player_id,
      reactionId: 'pokerface',
    };
    renderReactions({items: [item]});

    await waitFor(() => expect(screen.getByRole('img', {name: 'Pokerface'})).toBeVisible());
    await user.click(screen.getByRole('button', {name: 'Ocultar efeitos de reações'}));
    expect(screen.queryByRole('img', {name: 'Pokerface'})).not.toBeInTheDocument();
    expect(window.localStorage.getItem('poker:table-reactions-muted')).toBe('true');

    await user.click(screen.getByRole('button', {name: 'Mostrar efeitos de reações'}));
    expect(await screen.findByRole('img', {name: 'Pokerface'})).toBeInTheDocument();
    expect(window.localStorage.getItem('poker:table-reactions-muted')).toBe('false');
  });

  test('starts muted from the stored preference', () => {
    window.localStorage.setItem('poker:table-reactions-muted', 'true');
    renderReactions({
      items: [{id: 'reaction-1', playerId: viewer.player_id, reactionId: 'clap'}],
    });
    expect(screen.queryByRole('img', {name: 'Aplausos'})).not.toBeInTheDocument();
  });

  test('re-aims a thrown reaction at the recipient when a layout change moves the seats mid-flight', () => {
    const {container} = renderReactions({
      items: [{
        id: 'reaction-1', playerId: viewer.player_id,
        reactionId: 'tomato', targetPlayerId: opponent.player_id,
      }],
    });
    const effect = container.querySelector<HTMLElement>('[data-reaction-id="tomato"]');
    // jsdom gives the stub seats a zero rect, so the flight starts flat.
    expect(effect?.style.getPropertyValue('--reaction-dx')).toBe('0px');

    // The stage flipping between its oval and portrait-ring layouts moves both
    // seats while the ~3 s projectile is still in the air.
    const stops = [
      registerSeatElement(viewer.player_id, seatNode(0)),
      registerSeatElement(opponent.player_id, seatNode(400)),
    ];
    act(() => void fireEvent(window, new Event('orientationchange')));
    expect(effect?.style.getPropertyValue('--reaction-dx')).toBe('400px');
    expect(effect?.style.getPropertyValue('--reaction-x')).toBe('20px');

    // A plain viewport resize re-aims it the same way.
    stops.push(registerSeatElement(opponent.player_id, seatNode(120)));
    act(() => void fireEvent(window, new Event('resize')));
    expect(effect?.style.getPropertyValue('--reaction-dx')).toBe('120px');

    stops.forEach(stop => stop?.());
  });

  test('leaves a thrown reaction unpositioned while its recipient has no seat on screen', () => {
    const {container} = renderReactions({
      items: [{
        id: 'reaction-1', playerId: viewer.player_id,
        reactionId: 'tomato', targetPlayerId: 'left-the-table',
      }],
    });

    expect(seatCenter('left-the-table')).toBeUndefined();
    // The stylesheet keeps the effect hidden until it is positioned; nothing
    // here reveals it, so no inline geometry is written at all.
    const effect = container.querySelector<HTMLElement>('[data-reaction-id="tomato"]');
    expect(effect?.style.visibility).toBe('');
    expect(effect?.style.getPropertyValue('--reaction-dx')).toBe('');
  });

  test('gives every catalog action its own effect identity and positions targeted effects', async () => {
    const ids = Object.keys(TABLE_REACTIONS) as TableReactionID[];
    const items: TableReactionEvent[] = ids.map((reactionId, index) => ({
      id: `reaction-${index}`,
      playerId: viewer.player_id,
      reactionId,
      targetPlayerId: TABLE_REACTIONS[reactionId].targeted ? opponent.player_id : undefined,
    }));
    const {container} = renderReactions({items});

    await waitFor(() => expect(container.querySelectorAll('[data-reaction-impact]')).toHaveLength(ids.length));
    for (const id of ids) {
      const effect = container.querySelector<HTMLElement>(`[data-reaction-id="${id}"]`);
      expect(effect).toHaveClass(`reaction-${id}`);
      expect(effect).toHaveStyle({visibility: 'visible'});
      expect(effect?.querySelector(`[data-reaction-impact="${id}"]`)).toBeInTheDocument();
      expect(effect).toHaveClass(TABLE_REACTIONS[id].targeted ? 'thrown' : 'emote');
    }
    expect(container.querySelector('[data-reaction-id="spotlight"]')).toHaveStyle({
      '--reaction-dx': '0px',
      '--reaction-dy': '0px',
    });
    expect(container.querySelectorAll(
      '[data-reaction-impact="knife"] .reaction-impact-particles i'
    )).toHaveLength(8);
  });

  test('keeps an effect hidden until its required seat nodes exist', () => {
    const {container} = render(<TableReactions
      items={[{id: 'missing-source', playerId: 'missing', reactionId: 'clap'},
        {id: 'missing-target', playerId: viewer.player_id, targetPlayerId: 'missing', reactionId: 'crown'}]}
      seats={[viewer]} viewerId={viewer.player_id} connected coolingDown={false}
      pendingReaction={null} open={false} onQuickSendAction={vi.fn()}
      onPendingReactionChangeAction={vi.fn()} onOpenChangeAction={vi.fn()}/>);

    expect((container.querySelector<HTMLElement>('[data-reaction-id="clap"]'))?.style.visibility).toBe('');
    expect((container.querySelector<HTMLElement>('[data-reaction-id="crown"]'))?.style.visibility).toBe('');
  });

  test('names pending targeting, cancels it, toggles the panel and closes after the hover grace period', () => {
    vi.useFakeTimers();
    try {
      const {callbacks, rerender, props} = renderReactions({open: false, pendingReaction: 'bandage'});
      expect(screen.getByRole('status')).toHaveTextContent('Curar bad beat');
      fireEvent.click(screen.getByRole('button', {name: 'Cancelar reação direcionada'}));
      expect(callbacks.onPendingReactionChangeAction).toHaveBeenCalledWith(null);

      rerender(<TableReactions {...props} open={false} pendingReaction={null}/>);
      const aside = screen.getByLabelText('Reações da mesa');
      fireEvent.click(screen.getByRole('button', {name: 'Abrir reações'}));
      fireEvent.mouseEnter(aside);
      expect(callbacks.onOpenChangeAction).toHaveBeenCalledWith(true);

      // Leaving does not close immediately: the pointer gets a grace period to
      // reach the panel, and coming back inside cancels the pending close.
      fireEvent.mouseLeave(aside);
      act(() => void vi.advanceTimersByTime(HOVER_PANEL_CLOSE_DELAY_MS - 20));
      expect(callbacks.onOpenChangeAction).not.toHaveBeenCalledWith(false);
      fireEvent.mouseEnter(aside);
      act(() => void vi.advanceTimersByTime(HOVER_PANEL_CLOSE_DELAY_MS));
      expect(callbacks.onOpenChangeAction).not.toHaveBeenCalledWith(false);

      fireEvent.mouseLeave(aside);
      act(() => void vi.advanceTimersByTime(HOVER_PANEL_CLOSE_DELAY_MS));
      expect(callbacks.onOpenChangeAction).toHaveBeenCalledWith(false);
    } finally {
      vi.useRealTimers();
    }
  });

  test('leaves the aside alone while a targeted reaction is waiting for its player', () => {
    vi.useFakeTimers();
    try {
      const {callbacks} = renderReactions({open: false, pendingReaction: 'bandage'});
      const aside = screen.getByLabelText('Reações da mesa');
      fireEvent.mouseEnter(aside);
      fireEvent.mouseLeave(aside);
      act(() => void vi.advanceTimersByTime(HOVER_PANEL_CLOSE_DELAY_MS * 2));
      expect(callbacks.onOpenChangeAction).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});
