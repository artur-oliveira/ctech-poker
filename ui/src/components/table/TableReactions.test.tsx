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
    onQuickSend: vi.fn(),
    onPendingReactionChange: vi.fn(),
    open: true,
    onOpenChange: vi.fn(),
    ...overrides,
  };
  return {props, ...render(<TableReactions {...props}/>)};
}

describe('TableReactions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
  });
  
  test('opens and closes through the controlled toggle', async () => {
    const closed = renderReactions({open: false});
    expect(screen.queryByText('Reagir')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Abrir reações'}));
    expect(closed.props.onOpenChange).toHaveBeenCalledWith(true);
    closed.unmount();
    
    const opened = renderReactions();
    await userEvent.click(screen.getByRole('button', {name: 'Fechar reações'}));
    expect(opened.props.onOpenChange).toHaveBeenCalledWith(false);
  });
  
  test('sends quick reactions immediately and closes the panel before seat targeting', async () => {
    const {props} = renderReactions();
    fireEvent.click(screen.getByTitle('Aplausos'));
    expect(props.onQuickSend).toHaveBeenCalledWith('clap');
    fireEvent.click(screen.getByTitle('Jogar tomate'));
    expect(props.onPendingReactionChange).toHaveBeenCalledWith('tomato');
    expect(props.onOpenChange).toHaveBeenCalledWith(false);
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
    expect(props.onPendingReactionChange).toHaveBeenCalledWith(null);
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
  });
});
