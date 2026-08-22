import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
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

describe('WinnerCards', () => {
  test('shows the winner name and big-blind price, then requests the reveal', async () => {
    const onRequest = vi.fn();
    render(<WinnerCards snapshot={snapshot()} viewer="viewer" bigBlind={50} onRequestWinnerCardsAction={onRequest}/>);
    await userEvent.click(screen.getByRole('button', {name: /Ver a mão de Bia por 50 fichas/}));
    expect(onRequest).toHaveBeenCalledOnce();
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
