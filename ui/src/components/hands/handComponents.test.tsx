import {act, fireEvent, render, screen} from '@testing-library/react';
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
    expect(screen.getByText('Pagou (call)')).toBeInTheDocument();
    expect(screen.getByRole('heading', {name: 'Pré-flop'})).toBeInTheDocument();
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
  
  test('translates peek_cards, renders the board runout without an actor, and ends on the outcome beat', async () => {
    const user = userEvent.setup();
    render(<HandReplayer hand={hand} viewerId="viewer" actions={[
      {...actions[0], action: 'peek_cards', amount: 0},
      actions[1],
      // First frame to reach `complete` is the server-dealt runout (empty
      // player_id); the `won` beat that follows shares the same frame.
      {seq: 6, player_id: '', action: 'runout_step', amount: 0, timestamp: 3000, frame: actions[2].frame},
      actions[2],
      {seq: 7, player_id: 'p2', action: 'lost', amount: 0, timestamp: 3100, frame: actions[2].frame},
      {seq: 8, player_id: 'p2', action: 'join', amount: 0, timestamp: 3200, frame: actions[2].frame},
    ]}/>);
    expect(screen.getByText('espiou as cartas')).toBeInTheDocument();

    // …runout beat: label stands alone, no bold actor name.
    await user.click(screen.getByRole('button', {name: 'Próxima ação'}));
    await user.click(screen.getByRole('button', {name: 'Próxima ação'}));
    expect(screen.getByText('Cartas restantes do board reveladas')).toBeInTheDocument();

    // …the `won` beat survives past the first complete frame; `join` does not.
    await user.click(screen.getByRole('button', {name: 'Próxima ação'}));
    expect(screen.getByText('venceu')).toBeInTheDocument();
    // …then the losing beat, and then the end — the next-hand `join` never
    // entered the replay.
    await user.click(screen.getByRole('button', {name: 'Próxima ação'}));
    expect(screen.getByText('perdeu a mão')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Próxima ação'})).toBeDisabled();
  });

  test('timeline translates peek_cards, the board runout and the lost result', () => {
    render(<ActionTimeline actions={[
      {seq: 1, player_id: 'viewer', action: 'peek_cards', amount: 0, timestamp: 0},
      {seq: 2, player_id: '', action: 'runout_step', amount: 0, timestamp: 0},
      {seq: 3, player_id: 'p2', action: 'lost', amount: 0, timestamp: 0},
      {seq: 4, player_id: 'viewer', action: 'request_exit', amount: 0, timestamp: 0},
    ]} resolveName={id => id === 'viewer' ? 'Você' : 'Rival'}/>);
    expect(screen.getByText('Espiou as cartas')).toBeInTheDocument();
    expect(screen.getByText('Cartas restantes do board reveladas')).toBeInTheDocument();
    expect(screen.getByText('Perdeu a mão')).toBeInTheDocument();
    expect(screen.getByText('Pediu para sair da mesa')).toBeInTheDocument();
  });

  // Issue #114: the transport was mouse-first — the buttons and the scrubber
  // were focusable, but nothing was bound on the replayer region itself.
  describe('keyboard transport', () => {
    function replayer() {
      render(<HandReplayer hand={hand} actions={actions} viewerId="viewer"/>);
      const section = screen.getByRole('region', {name: 'Replay interativo da mão'});
      section.focus();
      return section;
    }

    test('advertises the shortcuts on the region', () => {
      const section = replayer();
      expect(section).toHaveAccessibleDescription(/barra de espaço reproduz ou pausa/);
    });

    test('steps forward and back with the arrow keys, clamped at both ends', () => {
      const section = replayer();
      expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();

      fireEvent.keyDown(section, {key: 'ArrowLeft'});
      expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();

      fireEvent.keyDown(section, {key: 'ArrowRight'});
      expect(screen.getByText(/Ação 2 de 3/)).toBeInTheDocument();
      fireEvent.keyDown(section, {key: 'ArrowRight'});
      fireEvent.keyDown(section, {key: 'ArrowRight'});
      expect(screen.getByText(/Ação 3 de 3/)).toBeInTheDocument();

      fireEvent.keyDown(section, {key: 'ArrowLeft'});
      expect(screen.getByText(/Ação 2 de 3/)).toBeInTheDocument();
    });

    test('Home returns to the first action', () => {
      const section = replayer();
      fireEvent.keyDown(section, {key: 'ArrowRight'});
      expect(screen.getByText(/Ação 2 de 3/)).toBeInTheDocument();
      fireEvent.keyDown(section, {key: 'Home'});
      expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();
    });

    test('space toggles play/pause and pauses what the arrows resume', () => {
      const section = replayer();
      fireEvent.keyDown(section, {key: ' '});
      expect(screen.getByRole('button', {name: 'Pausar replay'})).toBeInTheDocument();
      fireEvent.keyDown(section, {key: ' '});
      expect(screen.getByRole('button', {name: 'Reproduzir replay'})).toBeInTheDocument();

      fireEvent.keyDown(section, {key: ' '});
      expect(screen.getByRole('button', {name: 'Pausar replay'})).toBeInTheDocument();
      fireEvent.keyDown(section, {key: 'ArrowRight'});
      expect(screen.getByRole('button', {name: 'Reproduzir replay'})).toBeInTheDocument();
    });

    test('space restarts a finished replay instead of resuming at the end', () => {
      const section = replayer();
      fireEvent.keyDown(section, {key: 'ArrowRight'});
      fireEvent.keyDown(section, {key: 'ArrowRight'});
      expect(screen.getByText(/Ação 3 de 3/)).toBeInTheDocument();
      fireEvent.keyDown(section, {key: ' '});
      expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();
    });

    test('ignores OS auto-repeat, unbound keys and the scrubber\'s own keys', () => {
      const section = replayer();
      fireEvent.keyDown(section, {key: 'ArrowRight', repeat: true});
      fireEvent.keyDown(section, {key: 'End'});
      expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();

      // The range input owns its arrows; the region must not step twice.
      fireEvent.keyDown(section.querySelector('input[type="range"]')!, {key: 'ArrowRight'});
      expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();
    });

    test('leaves space to a focused button so activation is not stolen', () => {
      replayer();
      fireEvent.keyDown(screen.getByRole('button', {name: 'Voltar ao início'}), {key: ' '});
      expect(screen.getByRole('button', {name: 'Reproduzir replay'})).toBeInTheDocument();
      // …but the arrows still work from there.
      fireEvent.keyDown(screen.getByRole('button', {name: 'Voltar ao início'}), {key: 'ArrowRight'});
      expect(screen.getByText(/Ação 2 de 3/)).toBeInTheDocument();
    });
  });

  test('cycles the playback speed through 1x, 2x and 0,5x', async () => {
    const user = userEvent.setup();
    render(<HandReplayer hand={hand} actions={actions} viewerId="viewer"/>);
    const speed = screen.getByRole('button', {name: /Velocidade 1 vezes/});
    expect(speed).toHaveTextContent('1×');
    await user.click(speed);
    expect(screen.getByRole('button', {name: /Velocidade 2 vezes/})).toHaveTextContent('2×');
    await user.click(screen.getByRole('button', {name: /Velocidade 2 vezes/}));
    expect(screen.getByRole('button', {name: /Velocidade 0,5 vezes/})).toHaveTextContent('0,5×');
  });

  // Issue #114: reduced motion suppresses the card-reveal animation that made
  // the cadence readable, so the step must slow down to compensate.
  test('slows every step to a flat, unhurried beat under reduced motion', () => {
    vi.useFakeTimers();
    const original = window.matchMedia;
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: query.includes('reduce'), media: query, onchange: null,
      addListener: vi.fn(), removeListener: vi.fn(),
      addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
    })) as typeof window.matchMedia;
    try {
      render(<HandReplayer hand={hand} actions={actions} viewerId="viewer"/>);
      fireEvent.click(screen.getByRole('button', {name: 'Reproduzir replay'}));
      // The animated build would have advanced at 900ms / 1700ms.
      act(() => void vi.advanceTimersByTime(1_800));
      expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();
      act(() => void vi.advanceTimersByTime(700));
      expect(screen.getByText(/Ação 2 de 3/)).toBeInTheDocument();
    } finally {
      vi.useRealTimers();
      window.matchMedia = original;
    }
  });

  test('shows an explicit fallback when old hands have no replay frames', () => {
    render(<HandReplayer hand={hand} actions={[]} viewerId="viewer"/>);
    expect(screen.getByText(/antes dos frames de replay/)).toBeInTheDocument();
  });

  test('shows the hand big blind, derived from post_big_blind for non-25 tables', () => {
    render(<HandReplayer hand={hand} viewerId="viewer" actions={[
      {seq: 0, player_id: 'p2', action: 'post_big_blind', amount: 100, timestamp: 0, frame: actions[0].frame},
      actions[0],
    ]}/>);
    const blind = document.querySelector('.replay-blind') as HTMLElement;
    expect(blind).toHaveTextContent('BB');
    expect(blind).toHaveTextContent('100');
  });

  test('prefers a stored big_blind over the timeline and falls back to 25 for legacy hands', () => {
    const {rerender} = render(<HandReplayer hand={{...hand, big_blind: 500}} viewerId="viewer" actions={[
      {seq: 0, player_id: 'p2', action: 'post_big_blind', amount: 100, timestamp: 0, frame: actions[0].frame},
      actions[0],
    ]}/>);
    expect(document.querySelector('.replay-blind')).toHaveTextContent('500');

    rerender(<HandReplayer hand={hand} viewerId="viewer" actions={[actions[0]]}/>);
    expect(document.querySelector('.replay-blind')).toHaveTextContent('25');
  });
  

  test('plays through the remaining steps and stops at the last action', () => {
    vi.useFakeTimers();
    try {
      render(<HandReplayer hand={hand} actions={actions} viewerId="viewer"/>);
      fireEvent.click(screen.getByRole('button', {name: 'Reproduzir replay'}));

      // The flop deal is deliberately slower than a plain action step.
      act(() => void vi.advanceTimersByTime(900));
      expect(screen.getByText(/Ação 1 de 3/)).toBeInTheDocument();
      act(() => void vi.advanceTimersByTime(800));
      expect(screen.getByText(/Ação 2 de 3/)).toBeInTheDocument();
      act(() => void vi.advanceTimersByTime(1400));
      expect(screen.getByText(/Ação 3 de 3/)).toBeInTheDocument();

      act(() => void vi.advanceTimersByTime(5000));
      expect(screen.getByText(/Ação 3 de 3/)).toBeInTheDocument();
      expect(screen.getByRole('button', {name: 'Reproduzir replay'})).toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  test('reveals opponent cards only after a show_cards action or at showdown', () => {
    const {rerender} = render(<HandReplayer hand={hand} viewerId="viewer" actions={[actions[0]]}/>);
    const backs = screen.getAllByAltText('Carta fechada');
    expect(backs.length).toBeGreaterThan(0);
    // The shared card back is above the fold and measured as LCP on every
    // table surface, so it must not be lazy-loaded.
    backs.forEach(back => expect(back).toHaveAttribute('loading', 'eager'));

    rerender(<HandReplayer hand={hand} viewerId="viewer" actions={[
      actions[0], {...actions[1], player_id: 'p2', action: 'show_cards'},
    ]}/>);
    fireEvent.click(screen.getByRole('button', {name: 'Próxima ação'}));
    expect(screen.queryAllByAltText('Carta fechada')).toHaveLength(0);
  });

  test('names an actor from the frame when the hand carries no opponent record', () => {
    render(<HandReplayer hand={{...hand, opponents: []}} viewerId="viewer" actions={[
      actions[0], {...actions[1], action: 'bet', amount: 0},
    ]}/>);
    fireEvent.click(screen.getByRole('button', {name: 'Próxima ação'}));
    expect(screen.getAllByText('Bia').length).toBeGreaterThan(0);
    expect(screen.getByText('apostou')).toBeInTheDocument();
  });

  test('ignores a reaction the client cannot draw and a replay with no framed action at all', () => {
    render(<HandReplayer hand={hand} viewerId="viewer" actions={[
      actions[0], {seq: 9, player_id: 'p2', action: 'reaction', amount: 0, timestamp: 1500, reaction_id: 'unknown'},
    ]}/>);
    expect(document.querySelector('.replay-reactions')).toBeNull();
  });

  test('skips an empty pot instead of drawing a zero-chip pot', () => {
    render(<HandReplayer hand={hand} viewerId="viewer" actions={[
      {...actions[0], frame: {...actions[0].frame!, pot: 0}},
    ]}/>);
    expect(screen.getByText(/Ação 1 de 1/)).toBeInTheDocument();
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
    await user.click(screen.getByRole('button', {name: /Velocidade 1 vezes/}));
    expect(screen.getByRole('button', {name: /Velocidade 2 vezes/})).toBeInTheDocument();
  });

  // Issue #351: coaching pauses/questions are opt-in and must not appear
  // (or change default behavior) unless `allowCoaching` is passed — this is
  // what keeps the public `/share` link, which never sets it, unaffected.
  test('hides the coaching toggle unless allowCoaching is set', () => {
    render(<HandReplayer hand={hand} actions={actions} viewerId="viewer"/>);
    expect(screen.queryByRole('button', {name: /Modo Coaching/})).not.toBeInTheDocument();
  });

  describe('coaching mode', () => {
    test('stays off by default even when allowed, so the reveal is not paused', () => {
      render(<HandReplayer hand={hand} actions={actions} viewerId="viewer" allowCoaching/>);
      expect(screen.getByRole('button', {name: 'Modo Coaching desativado'})).toBeInTheDocument();
      expect(screen.getByText('pagou')).toBeInTheDocument();
    });

    test('pauses at the hero\'s next decision, asks a question, and reveals it on demand', async () => {
      const user = userEvent.setup();
      render(<HandReplayer hand={hand} actions={actions} viewerId="viewer" allowCoaching/>);
      await user.click(screen.getByRole('button', {name: 'Modo Coaching desativado'}));
      expect(screen.getByRole('button', {name: 'Modo Coaching ativado'})).toBeInTheDocument();

      // The real action is hidden behind the coaching question…
      expect(screen.queryByText('pagou')).not.toBeInTheDocument();
      expect(screen.getByText(/posição na mesa/)).toBeInTheDocument();
      expect(screen.getByText(/pausado num ponto de decisão/)).toBeInTheDocument();
      expect(screen.getByRole('button', {name: 'Reproduzir replay'})).toBeDisabled();

      // …until the player asks to see it.
      await user.click(screen.getByRole('button', {name: 'Já pensei, revelar ação'}));
      expect(screen.getByText('pagou')).toBeInTheDocument();
      expect(screen.getByRole('button', {name: 'Reproduzir replay'})).toBeEnabled();
    });

    test('a skipped question does not reappear when the player steps back to it', async () => {
      const user = userEvent.setup();
      render(<HandReplayer hand={hand} actions={actions} viewerId="viewer" allowCoaching/>);
      await user.click(screen.getByRole('button', {name: 'Modo Coaching desativado'}));
      await user.click(screen.getByRole('button', {name: 'Pular pergunta'}));
      expect(screen.getByText('pagou')).toBeInTheDocument();

      await user.click(screen.getByRole('button', {name: 'Próxima ação'}));
      await user.click(screen.getByRole('button', {name: 'Ação anterior'}));
      expect(screen.getByText('pagou')).toBeInTheDocument();
    });
  });
});
