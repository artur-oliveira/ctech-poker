import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import type {TableSnapshot} from '@/lib/api/table';
import {WinnerCards} from './WinnerCards';

function snapshot(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
  return {
    stage: 'complete', won_without_showdown: true, winners: ['winner'], board: [],
    seats: [
      {player_id: 'viewer', stack: 1000, state: 'folded', contributed: 0, dealt_in: true},
      {player_id: 'winner', name: 'Bia', stack: 1000, state: 'active', contributed: 0, dealt_in: true,
        hole_cards: ['back', 'back']},
    ],
    ...overrides,
  };
}

function request(overrides: Partial<NonNullable<TableSnapshot['pending_winner_cards']>> = {}) {
  return {
    requester_id: 'viewer', requester_name: 'Ana', winner_id: 'winner', fee: 50,
    expires_at_unix_ms: 1_000_000 + 8_000, ...overrides,
  };
}

describe('WinnerCards', () => {
  beforeEach(() => vi.useFakeTimers({shouldAdvanceTime: true}));
  afterEach(() => vi.useRealTimers());

  test('shows the winner name and big-blind price, then requests the reveal', async () => {
    const onRequest = vi.fn();
    render(<WinnerCards snapshot={snapshot()} viewer="viewer" bigBlind={50} onRequestWinnerCardsAction={onRequest}/>);
    await userEvent.click(screen.getByRole('button', {name: /Pedir a mão de Bia por 50 fichas/}));
    expect(onRequest).toHaveBeenCalledOnce();
  });

  test('asks the winner to consent, naming the requester, the fee and the deadline', async () => {
    const onAnswer = vi.fn();
    vi.setSystemTime(new Date(1_000_000));
    render(<WinnerCards viewer="winner" bigBlind={50} onAnswerWinnerCardsAction={onAnswer}
      snapshot={snapshot({pending_winner_cards: request()})}/>);

    expect(screen.getByText(/Ana quer pagar 50 fichas para ver sua mão/)).toBeInTheDocument();
    expect(screen.getByText(/8s para responder/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Mostrar/}));
    expect(onAnswer).toHaveBeenCalledWith(true);
    await userEvent.click(screen.getByRole('button', {name: /Recusar/}));
    expect(onAnswer).toHaveBeenCalledWith(false);
  });

  test('shows the requester a wait state with the refund promise instead of the buy button', () => {
    vi.setSystemTime(new Date(1_000_000));
    render(<WinnerCards snapshot={snapshot({pending_winner_cards: request()})} viewer="viewer" bigBlind={50}/>);
    expect(screen.getByText('Aguardando resposta…')).toBeInTheDocument();
    expect(screen.getByText(/suas 50 fichas voltam/)).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('disables both answers while one is in flight', () => {
    render(<WinnerCards snapshot={snapshot({pending_winner_cards: request()})} viewer="winner" bigBlind={50} pending/>);
    for (const button of screen.getAllByRole('button')) expect(button).toBeDisabled();
  });

  test('falls back to a neutral requester label when the name is missing', () => {
    render(<WinnerCards viewer="winner" bigBlind={50}
      snapshot={snapshot({pending_winner_cards: request({requester_name: ''})})}/>);
    expect(screen.getByText(/Um jogador quer pagar/)).toBeInTheDocument();
  });

  test.each([
    {viewer: 'winner'},
    {snapshot: snapshot({seats: [
      {player_id: 'viewer', stack: 1000, state: 'pending_entry', contributed: 0, dealt_in: false},
      {player_id: 'winner', name: 'Bia', stack: 1000, state: 'active', contributed: 0, dealt_in: true,
        hole_cards: ['back', 'back']},
    ]})},
    {snapshot: snapshot({seats: [
      {player_id: 'viewer', stack: 1000, state: 'folded', contributed: 0, dealt_in: true},
      {player_id: 'winner', name: 'Bia', stack: 1000, state: 'active', contributed: 0, dealt_in: true, hole_cards: ['Ah', 'Kd']},
    ]})},
  ])('stays hidden when a purchase is not meaningful', ({viewer = 'viewer', snapshot: value = snapshot()}) => {
    const {container} = render(<WinnerCards snapshot={value} viewer={viewer} bigBlind={50}/>);
    expect(container).toBeEmptyDOMElement();
  });

  test('stays locked after a rejected request until a revealed snapshot arrives', () => {
    const {rerender} = render(<WinnerCards snapshot={snapshot()} viewer="viewer" bigBlind={50}/>);
    expect(screen.getByRole('button')).toBeInTheDocument();
    rerender(<WinnerCards snapshot={snapshot({seats: [
      {player_id: 'viewer', stack: 950, state: 'folded', contributed: 0, dealt_in: true},
      {player_id: 'winner', name: 'Bia', stack: 1010, state: 'active', contributed: 0, dealt_in: true, hole_cards: ['Ah', 'Kd']},
    ]})} viewer="viewer" bigBlind={50}/>);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
