import {act, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {Seat} from './Seat';
import type {SeatView} from '@/lib/api/table';

vi.mock('@/lib/hooks/useReducedMotionCountdown', () => ({useReducedMotionCountdown: () => 7}));
vi.mock('@/lib/hooks/useDeckVariant', () => ({useDeckVariant: () => 'four-color'}));

afterEach(() => {
  vi.useRealTimers();
});

const seat = (overrides: Partial<SeatView> = {}): SeatView => ({
  player_id: 'player-1', name: 'Bia', stack: 1000, state: 'active', dealt_in: true, contributed: 0, ...overrides,
});

const render_ = (props: Partial<Parameters<typeof Seat>[0]> = {}) =>
  render(<Seat seat={seat()} isViewer={false} isTurn={false} index={0} {...props}/>);

describe('Seat', () => {
  test('names the seat, its stack and its hand category', () => {
    render_({seat: seat({hand_category: 'two_pair'})});
    expect(screen.getByText('Bia')).toBeInTheDocument();
    expect(screen.getByText('1.000 fichas')).toBeInTheDocument();
    expect(screen.getByText('Dois pares')).toBeInTheDocument();
  });

  test('falls back to the raw category for one the client does not translate', () => {
    render_({seat: seat({hand_category: 'royal_flush_supreme'})});
    expect(screen.getByText('royal_flush_supreme')).toBeInTheDocument();
  });

  test.each([
    ['folded', 'Desistiu'], ['all_in', 'All-in'], ['sitting_out', 'Ausente'],
    ['disconnected', 'Desconectado'], ['pending_entry', 'Aguardando'],
  ])('labels the %s seat state', (state, label) => {
    render_({seat: seat({state})});
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  test('flags a dropped connection separately from the seat state', () => {
    const {container} = render_({seat: seat({state: 'active', connection_state: 'disconnected'})});
    expect(screen.getByText('Desconectado')).toBeInTheDocument();
    expect(container.querySelector('.game-seat')).toHaveClass('disconnected');
  });

  test('announces a pause that only takes effect after this hand', () => {
    render_({seat: seat({ready: false, state: 'active'})});
    expect(screen.getByText('Pausa na próxima mão')).toBeInTheDocument();
  });

  test('does not repeat the pause notice for a seat already sitting out', () => {
    render_({seat: seat({ready: false, state: 'sitting_out'})});
    expect(screen.queryByText('Pausa na próxima mão')).not.toBeInTheDocument();
    expect(screen.getByText('Ausente')).toBeInTheDocument();
  });

  test.each([
    [{isDealer: true, isSmallBlind: true}, 'D/SB', 'Dealer e small blind'],
    [{isDealer: true}, 'D', 'Dealer'],
    [{isSmallBlind: true}, 'SB', 'Small blind'],
    [{isBigBlind: true}, 'BB', 'Big blind'],
  ])('shows the %o position badge with its spelled-out label', (roles, badge, label) => {
    render_(roles);
    expect(screen.getByLabelText(label)).toHaveTextContent(badge);
  });

  test('shows no position badge for a seat holding none', () => {
    const {container} = render_();
    expect(container.querySelector('.seat-role')).toBeNull();
  });

  test.each([
    [0.1, 10, 'bg-[var(--danger)]'],
    [0.5, 50, 'bg-[var(--gold)]'],
    [0.9, 90, 'bg-[var(--success)]'],
  ])('tones the viewer equity bar for %s', (equity, percent, tone) => {
    const {container} = render_({seat: seat({equity}), isViewer: true});
    expect(screen.getByLabelText(`Chance estimada de vitória: ${percent}%`)).toBeInTheDocument();
    expect(container.querySelector(`.${CSS.escape(tone)}`)).toBeInTheDocument();
  });

  test('never shows an opponent equity readout', () => {
    render_({seat: seat({equity: 0.9}), isViewer: false});
    expect(screen.queryByLabelText(/Chance estimada/)).not.toBeInTheDocument();
  });

  describe('equity behind the peek gate', () => {
    const peekSeat = seat({hole_cards: ['AH', 'KD'], equity: 0.9});

    test('withholds equity until both of the viewer own cards have been peeked', async () => {
      render_({seat: peekSeat, isViewer: true, onPeekCards: vi.fn()});
      expect(screen.queryByLabelText(/Chance estimada/)).not.toBeInTheDocument();

      const cards = screen.getAllByRole('button', {name: /^Ver sua/});
      await userEvent.click(cards[0]);
      expect(screen.queryByLabelText(/Chance estimada/)).not.toBeInTheDocument();

      await userEvent.click(screen.getByRole('button', {name: /^Ver sua 2ª/}));
      expect(screen.getByLabelText('Chance estimada de vitória: 90%')).toBeInTheDocument();
    });

    test('hides equity again when a peeked card is put back face-down', async () => {
      render_({seat: peekSeat, isViewer: true, onPeekCards: vi.fn()});
      for (const card of screen.getAllByRole('button', {name: /^Ver sua/})) await userEvent.click(card);
      expect(screen.getByLabelText('Chance estimada de vitória: 90%')).toBeInTheDocument();

      await userEvent.click(screen.getByRole('button', {name: /^Ocultar sua 1ª/}));
      expect(screen.queryByLabelText(/Chance estimada/)).not.toBeInTheDocument();
    });

    test('shows equity with no peek gate once the hand completes', () => {
      render_({seat: peekSeat, isViewer: true, onPeekCards: vi.fn(), handComplete: true});
      expect(screen.getByLabelText('Chance estimada de vitória: 90%')).toBeInTheDocument();
    });
  });

  test('runs the decision clock while the base deadline is still ahead', () => {
    const now = Date.now();
    const {container} = render_({
      isTurn: true, baseDeadlineMs: now + 1000, actionDeadlineMs: now + 4000, nowMs: now, turnTimeoutMs: 3000,
    });
    expect(container.querySelector('.seat-turn-ring')).toBeInTheDocument();
    expect(container.querySelector('.seat-timebank-ring')).toBeNull();
    expect(screen.getByText('7')).toBeInTheDocument();
  });

  test('hands over to the labelled time bank once the decision clock expires', () => {
    const now = Date.now();
    const {container} = render_({
      isTurn: true, baseDeadlineMs: now - 1000, actionDeadlineMs: now + 2000, nowMs: now, turnTimeoutMs: 3000,
    });
    expect(container.querySelector('.seat-turn-ring')).toBeNull();
    expect(container.querySelector('.seat-timebank-ring')).toBeInTheDocument();
    expect(screen.getByLabelText('Jogador usando o time bank')).toBeInTheDocument();
  });

  test('shows no ring at all once both deadlines are behind', () => {
    const now = Date.now();
    const {container} = render_({
      isTurn: true, baseDeadlineMs: now - 7000, actionDeadlineMs: now - 4000, nowMs: now, turnTimeoutMs: 3000,
    });
    expect(container.querySelector('.seat-turn-ring')).toBeNull();
    expect(container.querySelector('.seat-timebank-ring')).toBeNull();
  });

  test('starts the time bank readout off a live clock, with no fresh snapshot to mark the handover', () => {
    vi.useFakeTimers();
    const now = Date.now();
    const {container} = render_({
      isTurn: true, baseDeadlineMs: now + 1000, actionDeadlineMs: now + 4000, nowMs: now, turnTimeoutMs: 3000,
    });
    expect(container.querySelector('.seat-timebank-ring')).toBeNull();

    act(() => void vi.advanceTimersByTime(1500));
    expect(container.querySelector('.seat-turn-ring')).toBeNull();
    expect(screen.getByLabelText('Jogador usando o time bank')).toBeInTheDocument();

    act(() => void vi.advanceTimersByTime(3000));
    expect(container.querySelector('.seat-timebank-ring')).toBeNull();
  });

  test('shows the bet in front of the seat only when there is one', () => {
    const {container, rerender} = render(
      <Seat seat={seat({contributed: 250})} isViewer={false} isTurn={false} index={0} bigBlind={20}/>);
    expect(screen.getByLabelText('Aposta de 250 fichas')).toBeInTheDocument();

    rerender(<Seat seat={seat({contributed: 0})} isViewer={false} isTurn={false} index={0} bigBlind={20}/>);
    expect(container.querySelector('.seat-bet')).toBeNull();
  });

  test.each([
    [{tied: true, playerId: 'player-1', amount: 100}, 'Empate'],
    [{tied: false, place: 2, playerId: 'player-1', amount: 100}, '2º lugar'],
    [undefined, 'Venceu'],
  ])('labels the win pill for %o', (winStanding, label) => {
    render_({isWinner: true, winAmount: 100, winStanding, credit: 100});
    expect(screen.getByRole('status')).toHaveTextContent(`${label}+100`);
  });

  test('shows a refund separately from a win', () => {
    const {container} = render_({refundAmount: 40});
    expect(screen.getByText(/↩ 40/)).toBeInTheDocument();
    expect(container.querySelector('.seat-win')).toBeNull();
  });

  test('offers reveal only on the still-hidden card and reports its index', async () => {
    const onRevealCardAction = vi.fn();
    render_({
      seat: seat({hole_cards: ['AH', 'KD'], hole_cards_revealed: [true, false]}),
      isViewer: true, canRevealCards: true, handComplete: true, onRevealCardAction,
    });
    const reveals = screen.getAllByRole('button', {name: /mostrar/i});
    expect(reveals).toHaveLength(1);
    await userEvent.click(reveals[0]);
    expect(onRevealCardAction).toHaveBeenCalledWith(1);
  });

  test('offers no reveal when the seat is not allowed to show cards', () => {
    render_({seat: seat({hole_cards: ['AH', 'KD']}), isViewer: true, canRevealCards: false});
    expect(screen.queryByRole('button', {name: /mostrar/i})).not.toBeInTheDocument();
  });

  describe('private card peek', () => {
    test('gates the viewer own cards behind a peek toggle only on the live table (onPeekCards wired)', () => {
      render_({seat: seat({hole_cards: ['AH', 'KD']}), isViewer: true, onPeekCards: vi.fn()});
      expect(screen.getAllByRole('button', {name: /^Ver sua/})).toHaveLength(2);
      expect(screen.queryByText('AS')).not.toBeInTheDocument();
    });

    test('never gates cards when there is nowhere to report a peek (e.g. hand-history replay)', () => {
      render_({seat: seat({hole_cards: ['AH', 'KD']}), isViewer: true});
      expect(screen.queryByRole('button', {name: /^Ver sua/})).not.toBeInTheDocument();
    });

    test('clicking Ver reveals the card and reports the peek once, clicking again only hides it locally', async () => {
      const onPeekCards = vi.fn();
      render_({seat: seat({hole_cards: ['AH', 'KD']}), isViewer: true, onPeekCards});
      const [firstCard] = screen.getAllByRole('button', {name: /^Ver sua/});
      await userEvent.click(firstCard);
      expect(screen.getByRole('button', {name: /Ocultar sua 1ª carta/})).toBeInTheDocument();
      expect(onPeekCards).toHaveBeenCalledOnce();

      await userEvent.click(screen.getByRole('button', {name: /Ocultar sua 1ª carta/}));
      expect(screen.getByRole('button', {name: 'Ver sua 1ª carta'})).toBeInTheDocument();
      expect(onPeekCards).toHaveBeenCalledOnce();
    });

    test('keeps one flippable node across both peek states so the turn animates each way', async () => {
      render_({seat: seat({hole_cards: ['AH', 'KD']}), isViewer: true, onPeekCards: vi.fn()});
      const card = screen.getAllByRole('button', {name: /^Ver sua/})[0];
      expect(card).toHaveClass('card-reveal');
      expect(card).not.toHaveClass('is-peeked');
      expect(card.querySelector('.card-back')).toBeInTheDocument();
      expect(card.querySelector('.card-front')).toBeInTheDocument();

      await userEvent.click(card);
      expect(card).toHaveClass('is-peeked');
      expect(card.querySelector('.card-back')).toBeInTheDocument();
      expect(card.querySelector('.card-front')).toBeInTheDocument();

      await userEvent.click(card);
      expect(card).not.toHaveClass('is-peeked');
    });

    test('never reveals on hover alone', async () => {
      render_({seat: seat({hole_cards: ['AH', 'KD']}), isViewer: true, onPeekCards: vi.fn()});
      const [firstCard] = screen.getAllByRole('button', {name: /^Ver sua/});
      await userEvent.hover(firstCard);
      expect(firstCard).not.toHaveClass('is-peeked');
      expect(screen.queryByRole('button', {name: /^Ocultar sua/})).not.toBeInTheDocument();
    });

    test('auto-reveals the viewer own cards once the hand completes, with no peek gate at all', () => {
      render_({seat: seat({hole_cards: ['AH', 'KD']}), isViewer: true, onPeekCards: vi.fn(), handComplete: true});
      expect(screen.queryByRole('button', {name: /^Ver sua/})).not.toBeInTheDocument();
      expect(screen.queryByRole('button', {name: /^Ocultar sua/})).not.toBeInTheDocument();
    });
  });

  describe('hot/cold streak badge', () => {
    test('shows no badge without a streak', () => {
      const {container} = render_({seat: seat({current_streak: 0})});
      expect(container.querySelector('.seat-streak')).not.toBeInTheDocument();
    });

    test('shows a hot win-streak badge', () => {
      const {container} = render_({seat: seat({current_streak: 3})});
      const badge = container.querySelector('.seat-streak');
      expect(badge).toHaveClass('is-hot');
      expect(badge).toHaveTextContent('V3');
    });

    test('shows a cold loss-streak badge', () => {
      const {container} = render_({seat: seat({current_streak: -2})});
      const badge = container.querySelector('.seat-streak');
      expect(badge).toHaveClass('is-cold');
      expect(badge).toHaveTextContent('D2');
    });
  });

  test('opens the private note editor for an opponent and reflects an existing note', async () => {
    const onEditNote = vi.fn();
    const {rerender} = render(<Seat seat={seat()} isViewer={false} isTurn={false} index={0}
      onEditNote={onEditNote}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Adicionar nota privada sobre Bia'}));
    expect(onEditNote).toHaveBeenCalledOnce();

    rerender(<Seat seat={seat()} isViewer={false} isTurn={false} index={0} onEditNote={onEditNote}
      playerNote={{opponent_id: 'player-1', note: 'agressivo', tag: 'red', updated_at: '2026-08-01T00:00:00Z'}}/>);
    const trigger = screen.getByRole('button', {name: 'Editar nota privada sobre Bia'});
    expect(trigger).toHaveClass('has-note');
    expect(trigger.querySelector('.tag-red')).toBeInTheDocument();
  });

  test('never offers a private note on the viewer own seat', () => {
    render_({isViewer: true, onEditNote: vi.fn()});
    expect(screen.queryByRole('button', {name: /nota privada/})).not.toBeInTheDocument();
  });

  test('names an unnamed opponent in the note label', () => {
    render_({seat: seat({name: undefined}), onEditNote: vi.fn()});
    expect(screen.getByRole('button', {name: 'Adicionar nota privada sobre jogador'})).toBeInTheDocument();
  });

  test('offers the seat as a reaction target only with both a label and a handler', async () => {
    const onReactionTarget = vi.fn();
    const {container, rerender} = render(<Seat seat={seat()} isViewer={false} isTurn={false} index={0}
      reactionTargetLabel="Jogar tomate" onReactionTarget={onReactionTarget}/>);
    expect(container.querySelector('.game-seat')).toHaveClass('is-reaction-target');
    await userEvent.click(screen.getByRole('button', {name: 'Jogar tomate em Bia'}));
    expect(onReactionTarget).toHaveBeenCalledOnce();

    rerender(<Seat seat={seat()} isViewer={false} isTurn={false} index={0} reactionTargetLabel="Jogar tomate"/>);
    expect(screen.queryByRole('button', {name: /Jogar tomate/})).not.toBeInTheDocument();
  });

  test('renders a chat bubble word by word for the staggered animation', () => {
    const {container} = render_({chatBubble: {id: 'm1', message: 'boa mão'}});
    const bubble = container.querySelector('.seat-chat-bubble')!;
    expect(bubble.querySelectorAll('span')).toHaveLength(2);
    expect(bubble).toHaveTextContent('boa mão');
  });

  test('shows the playstyle badge with its explanation', () => {
    render_({seat: seat({playstyle_badge: 'counter'})});
    expect(screen.getByText('Contra-ataque')).toHaveAttribute('title', '3-bet de pelo menos 10%');
  });

  test('marks top-rail seats so the winner pill drops below them', () => {
    const {container} = render_({index: 4});
    expect(container.querySelector('.game-seat')).toHaveClass('top-seat');
  });

  test('marks a seat still waiting for its player name', () => {
    const {container} = render_({seat: seat({name: undefined})});
    expect(container.querySelector('.game-seat')).toHaveClass('is-pending-name');
  });
});
