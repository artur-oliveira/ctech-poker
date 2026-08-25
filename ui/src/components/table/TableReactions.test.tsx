import {fireEvent, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';

import {TableReactions} from './TableReactions';
import type {SeatView} from '@/lib/api/table';
import type {TableReactionEvent} from '@/lib/reactions';

const seats: SeatView[] = [
  {player_id: 'viewer', name: 'Você', stack: 1_000, state: 'active', contributed: 0},
  {player_id: 'opponent-1', name: 'Ana', stack: 900, state: 'active', contributed: 50},
  {player_id: 'opponent-2', name: 'Beto', stack: 800, state: 'folded', contributed: 25},
];

function renderReactions(overrides: Partial<Parameters<typeof TableReactions>[0]> = {}) {
  const props = {
    items: [] as TableReactionEvent[],
    seats,
    viewerId: 'viewer',
    connected: true,
    coolingDown: false,
    pendingReaction: null,
    onQuickSendAction: vi.fn(),
    onPendingReactionChangeAction: vi.fn(),
    open: true,
    onOpenChangeAction: vi.fn(),
    ...overrides,
  };
  return {props, ...render(<TableReactions {...props}/>)};
}

describe('TableReactions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
    vi.mocked(window.matchMedia).mockImplementation(query => ({
      matches: false, media: query, onchange: null, addListener: vi.fn(), removeListener: vi.fn(),
      addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
    } as unknown as MediaQueryList));
  });
  
  test('opens and closes through the controlled toggle', async () => {
    const closed = renderReactions({open: false});
    expect(screen.queryByText('Reagir')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Abrir reações'}));
    expect(closed.props.onOpenChangeAction).toHaveBeenCalledWith(true);
    closed.unmount();
    
    const opened = renderReactions();
    await userEvent.click(screen.getByRole('button', {name: 'Fechar reações'}));
    expect(opened.props.onOpenChangeAction).toHaveBeenCalledWith(false);
    await userEvent.click(screen.getByRole('button', {name: 'Fechar painel de reações'}));
    expect(opened.props.onOpenChangeAction).toHaveBeenCalledTimes(2);
  });

  test('dismisses on Escape and on an outside click', async () => {
    const user = userEvent.setup();
    const escaped = renderReactions();
    await user.keyboard('{Escape}');
    expect(escaped.props.onOpenChangeAction).toHaveBeenCalledWith(false);
    expect(screen.getByRole('button', {name: 'Fechar reações'})).toHaveFocus();
    escaped.unmount();

    const clicked = renderReactions();
    await user.click(document.body);
    expect(clicked.props.onOpenChangeAction).toHaveBeenCalledWith(false);
  });
  
  test('sends quick reactions immediately and closes the panel, same as seat targeting', async () => {
    const {props} = renderReactions();
    fireEvent.click(screen.getByTitle('Aplausos'));
    expect(props.onQuickSendAction).toHaveBeenCalledWith('clap');
    expect(props.onOpenChangeAction).toHaveBeenNthCalledWith(1, false);
    fireEvent.click(screen.getByTitle('Jogar tomate'));
    expect(props.onPendingReactionChangeAction).toHaveBeenCalledWith('tomato');
    expect(props.onOpenChangeAction).toHaveBeenNthCalledWith(2, false);
  });
  
  test('blocks sends while disconnected, during cooldown, and when no target exists', async () => {
    const disconnected = renderReactions({connected: false});
    expect(screen.getByTitle('Risada')).toBeDisabled();
    expect(screen.getByTitle('Jogar ficha')).toBeDisabled();
    disconnected.unmount();
    
    const cooldown = renderReactions({coolingDown: true});
    expect(screen.getByTitle('Uau')).toBeDisabled();
    expect(screen.getByTitle('Jogar tomate')).toBeDisabled();
    cooldown.unmount();
    
    renderReactions({seats: [seats[0]]});
    expect(screen.getByTitle('Mandar café')).toBeDisabled();
  });
  
  test('turns the toggle into a cancel action while choosing a seat', async () => {
    const {props} = renderReactions({open: false, pendingReaction: 'coffee'});
    expect(screen.getByText('Escolha um jogador')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar arremesso'}));
    expect(props.onPendingReactionChangeAction).toHaveBeenCalledWith(null);
  });
  
  test('persists mute and hides incoming effects without hiding controls', async () => {
    const item: TableReactionEvent = {id: 'reaction-1', playerId: 'opponent-1', reactionId: 'angry'};
    const first = renderReactions({items: [item]});
    expect(screen.getByRole('img', {name: 'Raiva'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Silenciar animações de reações'}));
    expect(localStorage.getItem('poker:table-reactions-muted')).toBe('true');
    expect(screen.queryByRole('img', {name: 'Raiva'})).not.toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Ativar animações de reações'})).toBeInTheDocument();
    first.unmount();
    
    renderReactions({items: [item]});
    expect(screen.queryByRole('img', {name: 'Raiva'})).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Ativar animações de reações'}));
    expect(localStorage.getItem('poker:table-reactions-muted')).toBe('false');
    expect(screen.getByRole('img', {name: 'Raiva'})).toBeInTheDocument();
  });
  
  test('positions emotes from the source seat and thrown objects toward their target', () => {
    const source = document.createElement('div');
    source.className = 'game-seat';
    source.dataset.playerId = 'opponent-1';
    source.getBoundingClientRect = vi.fn(() => ({
      x: 10, y: 20, left: 10, top: 20, right: 110, bottom: 70, width: 100, height: 50,
      toJSON: () => ({}),
    }));
    const target = document.createElement('div');
    target.className = 'game-seat';
    target.dataset.playerId = 'viewer';
    target.getBoundingClientRect = vi.fn(() => ({
      x: 210, y: 120, left: 210, top: 120, right: 290, bottom: 160, width: 80, height: 40,
      toJSON: () => ({}),
    }));
    document.body.append(source, target);
    
    renderReactions({
      items: [
        {id: 'emote', playerId: 'opponent-1', reactionId: 'laugh'},
        {id: 'object', playerId: 'opponent-1', targetPlayerId: 'viewer', reactionId: 'chip'},
      ]
    });
    
    const emote = screen.getByRole('img', {name: 'Risada'});
    expect(emote.style.getPropertyValue('--reaction-x')).toBe('60px');
    expect(emote.style.getPropertyValue('--reaction-dy')).toBe('-72px');
    expect(emote.style.visibility).toBe('visible');
    
    const object = screen.getByRole('img', {name: 'Jogar ficha'});
    expect(object.style.getPropertyValue('--reaction-dx')).toBe('190px');
    expect(object.style.getPropertyValue('--reaction-dy')).toBe('95px');
    expect(object).toHaveClass('thrown');
    expect(object).toHaveClass('reaction-chip');
  });

  test('renders an impact effect for self reactions like cold and fire', () => {
    renderReactions({items: [{id: 'self-cold', playerId: 'viewer', reactionId: 'cold'}]});
    const cold = screen.getByRole('img', {name: 'Frio na mesa'});
    expect(cold).toHaveClass('emote');
    expect(cold).toHaveClass('reaction-cold');
    expect(cold.querySelector('.reaction-impact-cold')).not.toBeNull();
  });

  test('renders an impact effect for self reactions like respect and sleepy', () => {
    renderReactions({
      items: [
        {id: 'self-respect', playerId: 'viewer', reactionId: 'respect'},
        {id: 'self-sleepy', playerId: 'viewer', reactionId: 'sleepy'},
      ],
    });
    const respect = screen.getByRole('img', {name: 'Respeito'});
    expect(respect).toHaveClass('emote');
    expect(respect.querySelector('.reaction-impact-respect')).not.toBeNull();
    const sleepy = screen.getByRole('img', {name: 'Sono'});
    expect(sleepy).toHaveClass('emote');
    expect(sleepy.querySelector('.reaction-impact-sleepy')).not.toBeNull();
  });

  test('renders the new targeted reactions with their own impact markup', () => {
    const items: TableReactionEvent[] = [
      {id: 'r-poop', playerId: 'opponent-1', targetPlayerId: 'viewer', reactionId: 'poop'},
      {id: 'r-rofl', playerId: 'opponent-1', targetPlayerId: 'viewer', reactionId: 'rofl'},
      {id: 'r-duck', playerId: 'opponent-1', targetPlayerId: 'viewer', reactionId: 'duck'},
      {id: 'r-turtle', playerId: 'opponent-1', targetPlayerId: 'viewer', reactionId: 'turtle'},
      {id: 'r-knife', playerId: 'opponent-1', targetPlayerId: 'viewer', reactionId: 'knife'},
      {id: 'r-flowers', playerId: 'opponent-1', targetPlayerId: 'viewer', reactionId: 'flowers'},
    ];
    renderReactions({items});
    expect(screen.getByRole('img', {name: 'Jogar cocô'}).querySelector('.reaction-impact-poop')).not.toBeNull();
    expect(screen.getByRole('img', {name: 'Rir da cara'}).querySelector('.reaction-impact-rofl')).not.toBeNull();
    expect(screen.getByRole('img', {name: 'Jogar pato'}).querySelector('.reaction-impact-duck')).not.toBeNull();
    expect(screen.getByRole('img', {name: 'Chamar de lento'}).querySelector('.reaction-impact-turtle')).not.toBeNull();
    expect(screen.getByRole('img', {name: 'Jogar faca'}).querySelector('.reaction-impact-knife')).not.toBeNull();
    expect(screen.getByRole('img', {name: 'Mandar flores'}).querySelector('.reaction-impact-flowers')).not.toBeNull();
  });

  test('lists cold/fire among quick emotes and the new objects among thrown reactions', () => {
    renderReactions();
    expect(screen.getByTitle('Frio na mesa')).toBeInTheDocument();
    expect(screen.getByTitle('Sequência quente')).toBeInTheDocument();
    expect(screen.getByTitle('Respeito')).toBeInTheDocument();
    expect(screen.getByTitle('Sono')).toBeInTheDocument();
    expect(screen.getByTitle('Jogar cocô')).toBeInTheDocument();
    expect(screen.getByTitle('Rir da cara')).toBeInTheDocument();
    expect(screen.getByTitle('Jogar pato')).toBeInTheDocument();
    expect(screen.getByTitle('Chamar de lento')).toBeInTheDocument();
    expect(screen.getByTitle('Jogar faca')).toBeInTheDocument();
    expect(screen.getByTitle('Mandar flores')).toBeInTheDocument();
  });

  test('opens the buy flow for locked premium reactions and sends owned ones normally', async () => {
    const catalog = [{id: 'cold', premium: true, owned: false, price_cents: 100, price_fichas: 100_000}];
    const locked = renderReactions({premiumEnabled: true, catalog, purchases: [], onLockedReactionAction: vi.fn()});
    await userEvent.click(screen.getByTitle('Frio na mesa'));
    expect(locked.props.onLockedReactionAction).toHaveBeenCalledWith(catalog[0]);
    expect(locked.props.onQuickSendAction).not.toHaveBeenCalled();
    locked.unmount();

    const owned = renderReactions({
      premiumEnabled: true,
      catalog: [{...catalog[0], owned: true}],
      purchases: [{purchase_id: 'rp-1', reaction_id: 'cold', method: 'fichas', status: 'confirmed'}],
      onLockedReactionAction: vi.fn(),
    });
    await userEvent.click(screen.getByTitle('Frio na mesa'));
    expect(owned.props.onQuickSendAction).toHaveBeenCalledWith('cold');
    expect(owned.props.onLockedReactionAction).not.toHaveBeenCalled();
  });


  test('renders an impact effect for every thrown and premium reaction', () => {
    const thrown = ['chip', 'tomato', 'coffee', 'clover', 'horseshoe', 'tear', 'poop', 'rofl',
      'duck', 'turtle', 'knife', 'flowers'] as const;
    for (const reactionId of thrown) {
      const {container, unmount} = render(<TableReactions items={[
        {id: `r-${reactionId}`, playerId: 'opponent-1', reactionId, targetPlayerId: 'viewer'},
      ]} seats={seats} viewerId="viewer" connected coolingDown={false} pendingReaction={null}
        onQuickSendAction={vi.fn()} onPendingReactionChangeAction={vi.fn()} open={false}
        onOpenChangeAction={vi.fn()}/>);
      expect(container.querySelector(`.reaction-impact-${reactionId}`)).toBeInTheDocument();
      unmount();
    }
  });

  test('shows the shortcut row and sends a favorite without opening the full picker', async () => {
    const onQuickSendAction = vi.fn();
    const onOpenChangeAction = vi.fn();
    renderReactions({
      favorites: ['clap', 'fire'], premiumEnabled: true,
      catalog: [{id: 'fire', premium: true, owned: true, price_cents: 990, price_fichas: 5000}],
      purchases: [{purchase_id: 'p1', reaction_id: 'fire', method: 'pix', status: 'confirmed'}],
      onQuickSendAction, onOpenChangeAction,
    });
    const shortcuts = screen.getByRole('group', {name: 'Reações favoritas'});
    expect(shortcuts.querySelectorAll('button')).toHaveLength(2);
    expect(screen.getAllByLabelText('Premium liberada').length).toBeGreaterThan(0);

    await userEvent.click(shortcuts.querySelector<HTMLButtonElement>('button[title="Aplausos"]')!);
    expect(onQuickSendAction).toHaveBeenCalledWith('clap');
    expect(onOpenChangeAction).toHaveBeenCalledWith(false);
  });

  test('starts targeting from a favorite thrown object instead of sending it', async () => {
    const onPendingReactionChangeAction = vi.fn();
    renderReactions({favorites: ['tomato'], onPendingReactionChangeAction});
    await userEvent.click(screen.getByRole('group', {name: 'Reações favoritas'})
      .querySelector<HTMLButtonElement>('button[title="Jogar tomate"]')!);
    expect(onPendingReactionChangeAction).toHaveBeenCalledWith('tomato');
  });

  test('ignores an unknown favorite instead of crashing the shortcut row', () => {
    renderReactions({favorites: ['clap', 'not-a-reaction'] as never});
    expect(screen.getByRole('group', {name: 'Reações favoritas'}).querySelectorAll('button')).toHaveLength(1);
  });

  test('disables premium reactions while the catalog is still loading', () => {
    renderReactions({premiumEnabled: true, premiumLoading: true, catalog: [], purchases: []});
    expect(screen.getByRole('button', {name: 'Sequência quente'})).toBeDisabled();
  });

  test('disables a premium reaction that is mid-refund or missing from the catalog', async () => {
    const onLockedReactionAction = vi.fn();
    renderReactions({
      premiumEnabled: true,
      catalog: [{id: 'fire', premium: true, price_cents: 990, price_fichas: 5000}],
      purchases: [{purchase_id: 'p1', reaction_id: 'fire', method: 'pix', status: 'refunding'}],
      onLockedReactionAction,
    });
    expect(screen.getByRole('button', {name: 'Sequência quente'})).toBeDisabled();
    // Not in the catalog at all: unavailable rather than buyable.
    expect(screen.getByRole('button', {name: 'Frio na mesa'})).toBeDisabled();
    expect(onLockedReactionAction).not.toHaveBeenCalled();
  });

  test('keeps premium locking off entirely when the store is disabled', async () => {
    const onQuickSendAction = vi.fn();
    renderReactions({premiumEnabled: false, onQuickSendAction});
    await userEvent.click(screen.getByRole('button', {name: 'Sequência quente'}));
    expect(onQuickSendAction).toHaveBeenCalledWith('fire');
  });


  test('marks a locked thrown object and opens the store instead of throwing it', async () => {
    const onLockedReactionAction = vi.fn();
    const onPendingReactionChangeAction = vi.fn();
    const onOpenChangeAction = vi.fn();
    renderReactions({
      premiumEnabled: true, favorites: ['knife'],
      catalog: [{id: 'knife', premium: true, price_cents: 990, price_fichas: 5000}],
      purchases: [],
      onLockedReactionAction, onPendingReactionChangeAction, onOpenChangeAction,
    });
    expect(screen.getAllByLabelText('Premium bloqueada').length).toBeGreaterThan(1);

    await userEvent.click(screen.getByRole('button', {name: /Jogar faca/}));
    expect(onLockedReactionAction).toHaveBeenCalledWith(expect.objectContaining({id: 'knife'}));
    expect(onPendingReactionChangeAction).not.toHaveBeenCalled();
    expect(onOpenChangeAction).toHaveBeenCalledWith(false);
  });

  test('opens on hover only while no throw is being aimed', async () => {
    const onOpenChangeAction = vi.fn();
    const {container} = renderReactions({open: false, onOpenChangeAction});
    vi.mocked(window.matchMedia).mockImplementation(query => ({
      matches: true, media: query, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    } as unknown as MediaQueryList));

    const aside = container.querySelector('.table-reactions')!;
    fireEvent.mouseEnter(aside);
    expect(onOpenChangeAction).toHaveBeenCalledWith(true);
    fireEvent.mouseLeave(aside);
    expect(onOpenChangeAction).toHaveBeenCalledWith(false);

    onOpenChangeAction.mockClear();
    const aiming = renderReactions({open: false, pendingReaction: 'tomato', onOpenChangeAction});
    fireEvent.mouseEnter(aiming.container.querySelector('.table-reactions')!);
    expect(onOpenChangeAction).not.toHaveBeenCalled();
  });

  test('edits up to three favorites from the table picker', async () => {
    const save = vi.fn().mockResolvedValue(undefined);
    renderReactions({favorites: ['clap'], onFavoriteReactionsChangeAction: save});
    await userEvent.click(screen.getByRole('button', {name: 'Editar reações favoritas'}));
    expect(screen.getByRole('heading', {name: 'Atalhos de reação'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Risada/}));
    await userEvent.click(screen.getByRole('button', {name: 'Salvar atalhos'}));
    expect(save).toHaveBeenCalledWith(['clap', 'laugh']);
  });
});
