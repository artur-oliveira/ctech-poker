import {fireEvent, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import {ActionTimeline} from './ActionTimeline';
import {HandReplayer} from './HandReplayer';
import {OutcomeBadge} from './OutcomeBadge';
import type {HandHistoryAction} from '@/lib/api/table';
import type {HandItem} from '@/lib/api/player';

vi.mock('@/lib/hooks/useDeckVariant', () => ({
  useDeckVariant: () => 'four-color',
}));

const hand: HandItem = {
  pk: 'viewer',
  sk: 'h1',
  table_id: 'table-1',
  hand_id: 'h1',
  outcome: 'won',
  net_change: 200,
  ended_at: 1_700_000_000_000,
  board: ['AH', 'KD', 'QS', 'JC', 'TH'],
  hole_cards: ['2C', '3D'],
  opponents: [{player_id: 'p2', name: 'Bia', hole_cards: ['4C', '5D'], won: false}],
};

const actions: HandHistoryAction[] = [
  {
    seq: 1, player_id: 'viewer', action: 'call', amount: 50, timestamp: 1000, frame: {
      stage: 'pre_flop', board: [], pot: 100, current_player_id: 'p2',
      seats: [
        {player_id: 'viewer', name: 'Ana', stack: 950, state: 'active', contributed: 50, dealt_in: true},
        {player_id: 'p2', name: 'Bia', stack: 950, state: 'active', contributed: 50, dealt_in: true},
      ],
    }
  },
  {
    seq: 2, player_id: 'p2', action: 'check', amount: 0, timestamp: 2000, frame: {
      stage: 'flop', board: ['AH', 'KD', 'QS'], pot: 100, current_player_id: 'viewer',
      seats: [
        {player_id: 'viewer', name: 'Ana', stack: 950, state: 'active', contributed: 50, dealt_in: true},
        {player_id: 'p2', name: 'Bia', stack: 950, state: 'active', contributed: 50, dealt_in: true},
      ],
    }
  },
  {
    seq: 3, player_id: 'viewer', action: 'won', amount: 200, timestamp: 3000, frame: {
      stage: 'complete', board: hand.board, pot: 200, payouts: {viewer: 200}, winners: ['viewer'],
      seats: [
        {player_id: 'viewer', name: 'Ana', stack: 1150, state: 'active', contributed: 50, dealt_in: true},
        {player_id: 'p2', name: 'Bia', stack: 850, state: 'active', contributed: 50, dealt_in: true},
      ],
    }
  },
];

describe('hand history components', () => {
  test('timeline handles empty, known and future backend actions', () => {
    const {rerender} = render(<ActionTimeline actions={[]} resolveName={id => id}/>);
    expect(screen.getByText('Nenhuma ação registrada para esta mão.')).toBeInTheDocument();
    rerender(<ActionTimeline actions={[
      actions[0],
      {...actions[0], seq: 9, action: 'future_action' as HandHistoryAction['action'], amount: 0, timestamp: 0},
    ]} resolveName={id => id === 'viewer' ? 'Você' : id}/>);
    expect(screen.getByText('Call')).toBeInTheDocument();
    // Unknown keys degrade to a humanized form rather than raw snake_case.
    expect(screen.getByText('future action')).toBeInTheDocument();
    expect(screen.getByText('50')).toBeInTheDocument();
  });
  
  test('timeline translates the social actions and shows the reaction emoji', () => {
    render(<ActionTimeline actions={[
      {seq: 1, player_id: 'viewer', action: 'reaction', amount: 0, timestamp: 0, reaction_id: 'clap'},
      {
        seq: 2, player_id: 'viewer', action: 'reaction', amount: 0, timestamp: 0,
        reaction_id: 'tomato', target_player_id: 'rival'
      },
      {seq: 3, player_id: 'viewer', action: 'chat', amount: 0, timestamp: 0},
      {seq: 4, player_id: 'viewer', action: 'set_identity', amount: 0, timestamp: 0},
    ]} resolveName={id => id === 'viewer' ? 'Você' : 'Rival'}/>);
    expect(screen.getByRole('img', {name: 'Aplausos'})).toHaveTextContent('👏');
    expect(screen.getByRole('img', {name: 'Jogar tomate'})).toBeInTheDocument();
    expect(screen.getByText(/para Rival/)).toBeInTheDocument();
    expect(screen.getByText('Falou no chat')).toBeInTheDocument();
    expect(screen.getByText('Atualizou o perfil')).toBeInTheDocument();
  });
  
  test.each(['won', 'lost', 'tied'] as const)('renders %s outcome badge', outcome => {
    render(<OutcomeBadge outcome={outcome}/>);
    expect(screen.getByText(outcome === 'won' ? 'Vitória' : outcome === 'lost' ? 'Derrota' : 'Empate')).toBeInTheDocument();
  });
  
  test('replay translates frameless system actions and shows reactions on their beat', async () => {
    const user = userEvent.setup();
    render(<HandReplayer hand={hand} viewerId="viewer" actions={[
      // A reaction has no frame of its own, so it must attach to the action it
      // followed rather than disappearing from the replay entirely.
      {...actions[0], action: 'join', amount: 0},
      {seq: 4, player_id: 'p2', action: 'reaction', amount: 0, timestamp: 1500, reaction_id: 'clap'},
      actions[1],
      {seq: 5, player_id: 'p2', action: 'reaction', amount: 0, timestamp: 2500, reaction_id: 'tomato'},
    ]}/>);
    expect(screen.getByText('entrou na mesa')).toBeInTheDocument();
    expect(screen.getByText('👏')).toBeInTheDocument();
    expect(screen.getByText(/Bia · Aplausos/)).toBeInTheDocument();
    
    await user.click(screen.getByRole('button', {name: 'Próxima ação'}));
    expect(screen.queryByText('👏')).not.toBeInTheDocument();
    expect(screen.getByText('🍅')).toBeInTheDocument();
  });
  
  test('shows an explicit fallback when old hands have no replay frames', () => {
    render(<HandReplayer hand={hand} actions={[]} viewerId="viewer"/>);
    expect(screen.getByText(/antes dos frames de replay/)).toBeInTheDocument();
  });
  
  test('navigates replay manually, by range and with playback controls', async () => {
    const user = userEvent.setup();
    render(<HandReplayer hand={hand} actions={actions} viewerId="viewer"/>);
    expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();
    expect(screen.getAllByText(/Você/).length).toBeGreaterThan(0);
    
    await user.click(screen.getByRole('button', {name: 'Próxima ação'}));
    expect(screen.getByText(/Ação 2 de 3/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: 'Voltar ao início'}));
    expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();
    
    fireEvent.change(screen.getByRole('slider'), {target: {value: '2'}});
    expect(screen.getByText(/Ação 3 de 3/)).toBeInTheDocument();
    expect(screen.getByText('Vitória')).toBeInTheDocument();
    
    await user.click(screen.getByRole('button', {name: 'Reproduzir replay'}));
    expect(screen.getByRole('button', {name: 'Pausar replay'})).toBeInTheDocument();
    await user.click(screen.getByRole('button', {name: 'Pausar replay'}));
    await user.click(screen.getByRole('button', {name: 'Velocidade 1 vezes'}));
    expect(screen.getByRole('button', {name: 'Velocidade 2 vezes'})).toBeInTheDocument();
  });
});
