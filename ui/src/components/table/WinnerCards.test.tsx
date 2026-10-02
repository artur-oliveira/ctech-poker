import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import type {TableSnapshot, WinnerCardsRequest} from '@/lib/api/table';
import {winnerCardsSeat} from '@/lib/winnerCards';
import {WinnerCardsPrompt, WinnerCardsRequestControl} from './WinnerCards';

function snapshot(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
  return {
    stage: 'complete', won_without_showdown: true, winners: ['winner'], board: [], protocol_version: 12,
    seats: [
      {player_id: 'viewer', stack: 1000, state: 'folded', contributed: 0, dealt_in: true},
      {player_id: 'winner', name: 'Bia', stack: 1000, state: 'active', contributed: 0, dealt_in: true,
        hole_cards: ['back', 'back']},
    ],
    ...overrides,
  };
}

function request(overrides: Partial<WinnerCardsRequest> = {}): WinnerCardsRequest {
  return {
    requester_id: 'viewer', requester_name: 'Ana', winner_id: 'winner', fee: 50,
    expires_at_unix_ms: 1_000_000 + 8_000, ...overrides,
  };
}

describe('winnerCardsSeat', () => {
  test('offers the winner\'s cards to a dealt-in opponent', () => {
    expect(winnerCardsSeat(snapshot(), 'viewer')).toEqual({winnerId: 'winner', mode: 'offer'});
  });
  test.each([
    {protocol_version: 11},
    {won_without_showdown: false},
    {winner_cards_closed: true},
    {winners: ['winner', 'viewer']},
  ] satisfies Partial<TableSnapshot>[])('offers nothing when %o', overrides => {
    expect(winnerCardsSeat(snapshot(overrides), 'viewer')).toBeNull();
  });
  test('offers nothing once the winner\'s cards are face up', () => {
    const shown = snapshot();
    shown.seats[1] = {...shown.seats[1], hole_cards: ['AH', 'KD']};
    expect(winnerCardsSeat(shown, 'viewer')).toBeNull();
  });
  test('shows the requester their own wait, and the winner the whole batch', () => {
    const own = request();
    expect(winnerCardsSeat(snapshot({winner_cards_requests: [own]}), 'viewer'))
      .toEqual({winnerId: 'winner', mode: 'waiting', request: own});
    const batch = [own, request({requester_id: 'c', requester_name: 'Caio'})];
    expect(winnerCardsSeat(snapshot({winner_cards_requests: batch}), 'winner'))
      .toEqual({winnerId: 'winner', mode: 'prompt', requests: batch});
    expect(winnerCardsSeat(snapshot(), 'winner')).toBeNull();
  });
});

describe('WinnerCards controls', () => {
  beforeEach(() => {
    vi.useFakeTimers({shouldAdvanceTime: true});
    vi.setSystemTime(new Date(1_000_000));
  });
  afterEach(() => vi.useRealTimers());

  test('the winner\'s cards are the request button, priced in its accessible name', async () => {
    const onRequest = vi.fn();
    render(<WinnerCardsRequestControl state={{winnerId: 'winner', mode: 'offer'}} winnerName="Bia" fee={50}
                                      onRequest={onRequest}/>);
    await userEvent.click(screen.getByRole('button', {name: /Pedir para ver as cartas de Bia por 50/}));
    expect(onRequest).toHaveBeenCalledOnce();
  });

  test('a requester waits on the cards with the countdown and the refund promise', () => {
    render(<WinnerCardsRequestControl state={{winnerId: 'winner', mode: 'waiting', request: request()}}
                                      winnerName="Bia" fee={50}/>);
    expect(screen.getByRole('status', {name: /Aguardando Bia responder, 8s.*50/})).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  test('the winner answers the whole batch once: names, +N, half of every fee', async () => {
    const onAnswer = vi.fn();
    const requests = ['Ana', 'Bruno', 'Caio', 'Duda'].map((name, index) =>
      request({requester_id: `p${index}`, requester_name: name}));
    render(<WinnerCardsPrompt requests={requests} onAnswer={onAnswer}/>);
    expect(screen.getByRole('alert')).toHaveTextContent('Ana, Bruno e mais 2 querem ver sua mão');
    expect(screen.getByText('+1')).toBeInTheDocument();
    expect(screen.getByLabelText(/Você recebe 100/)).toBeInTheDocument();
    expect(screen.getByText('8s')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: /Recusar/}));
    await userEvent.click(screen.getByRole('button', {name: /Mostrar/}));
    expect(onAnswer.mock.calls).toEqual([[false], [true]]);
  });

  test('a single requester is named on their own', () => {
    render(<WinnerCardsPrompt requests={[request()]}/>);
    expect(screen.getByRole('alert')).toHaveTextContent('Ana quer ver sua mão');
  });
});
