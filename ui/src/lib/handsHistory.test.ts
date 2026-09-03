import {describe, expect, test} from 'vitest';
import {
  ALL_TABLES, dayLabel, filterHands, groupHandsByDay, handTables, loadedTotals, NO_FILTER
} from './handsHistory';
import type {HandItem, HandOutcome} from '@/lib/api/player';

// 2026-09-02T15:00:00Z, used as "now" everywhere so Hoje/Ontem are deterministic.
const NOW = Date.UTC(2026, 8, 2, 15);
const DAY = 24 * 3600_000;

function hand(overrides: Partial<HandItem> = {}): HandItem {
  return {
    hand_id: 'h1', table_id: 'tbl-aaaaaa', outcome: 'won' as HandOutcome, net_change: 100,
    ended_at: NOW, ...overrides
  } as HandItem;
}

describe('hand-history filtering (#115)', () => {
  test('the default filter is a no-op', () => {
    const hands = [hand({hand_id: 'a'}), hand({hand_id: 'b', outcome: 'lost'})];
    expect(filterHands(hands, NO_FILTER)).toEqual(hands);
  });

  test('narrows by outcome, by table, and by both at once', () => {
    const hands = [
      hand({hand_id: 'a', outcome: 'won', table_id: 't1'}),
      hand({hand_id: 'b', outcome: 'lost', table_id: 't1'}),
      hand({hand_id: 'c', outcome: 'won', table_id: 't2'})
    ];
    expect(filterHands(hands, {outcome: 'won', tableId: ALL_TABLES}).map(h => h.hand_id)).toEqual(['a', 'c']);
    expect(filterHands(hands, {outcome: 'all', tableId: 't1'}).map(h => h.hand_id)).toEqual(['a', 'b']);
    expect(filterHands(hands, {outcome: 'won', tableId: 't2'}).map(h => h.hand_id)).toEqual(['c']);
    expect(filterHands(hands, {outcome: 'tied', tableId: 't1'})).toEqual([]);
  });

  test('offers only tables present in the loaded pages, busiest first', () => {
    const hands = [
      hand({hand_id: 'a', table_id: 'quiet'}),
      hand({hand_id: 'b', table_id: 'busy'}),
      hand({hand_id: 'c', table_id: 'busy'}),
      hand({hand_id: 'd', table_id: 'also'})
    ];
    expect(handTables(hands)).toEqual([
      {tableId: 'busy', count: 2}, {tableId: 'also', count: 1}, {tableId: 'quiet', count: 1}
    ]);
    expect(handTables([])).toEqual([]);
  });
});

describe('loaded totals (#115)', () => {
  test('is null for an empty subset instead of a 0% win rate', () => {
    expect(loadedTotals([])).toBeNull();
  });

  test('counts wins, ties and losses and sums the net change', () => {
    expect(loadedTotals([
      hand({hand_id: 'a', outcome: 'won', net_change: 250}),
      hand({hand_id: 'b', outcome: 'lost', net_change: -100}),
      hand({hand_id: 'c', outcome: 'tied', net_change: 0}),
      hand({hand_id: 'd', outcome: 'won', net_change: 50})
    ])).toEqual({total: 4, netSum: 200, wins: 2, ties: 1, losses: 1, winRate: 50});
  });

  test('a purely losing subset nets negative and reports 0%', () => {
    expect(loadedTotals([hand({outcome: 'lost', net_change: -75})]))
      .toEqual({total: 1, netSum: -75, wins: 0, ties: 0, losses: 1, winRate: 0});
  });
});

describe('day grouping (#115)', () => {
  test('names today and yesterday, and dates anything older', () => {
    expect(dayLabel(NOW, NOW)).toBe('Hoje');
    expect(dayLabel(NOW - DAY, NOW)).toBe('Ontem');
    expect(dayLabel(NOW - 5 * DAY, NOW)).toMatch(/agosto de 2026/);
  });

  test('emits one header per day, carrying the day count and the row day', () => {
    const rows = groupHandsByDay([
      hand({hand_id: 'a', ended_at: NOW}),
      hand({hand_id: 'b', ended_at: NOW - 3600_000}),
      hand({hand_id: 'c', ended_at: NOW - DAY})
    ], NOW);

    expect(rows.map(row => row.kind)).toEqual(['day', 'hand', 'hand', 'day', 'hand']);
    expect(rows[0]).toMatchObject({kind: 'day', label: 'Hoje', count: 2});
    expect(rows[3]).toMatchObject({kind: 'day', label: 'Ontem', count: 1});
    // The day rides along on the row so the pinned bar never re-derives it.
    expect(rows.filter(row => row.kind === 'hand').map(row => row.kind === 'hand' && row.day))
      .toEqual(['Hoje', 'Hoje', 'Ontem']);
    // Keys must be unique across headers and rows for the virtualizer.
    expect(new Set(rows.map(row => row.key)).size).toBe(rows.length);
  });

  test('an empty list produces no headers at all', () => {
    expect(groupHandsByDay([], NOW)).toEqual([]);
  });

  test('re-entering a day later in the list opens a second header', () => {
    // The list is server-ordered; grouping must not silently merge a
    // non-contiguous repeat of the same day into the earlier header.
    const rows = groupHandsByDay([
      hand({hand_id: 'a', ended_at: NOW}),
      hand({hand_id: 'b', ended_at: NOW - DAY}),
      hand({hand_id: 'c', ended_at: NOW})
    ], NOW);
    expect(rows.filter(row => row.kind === 'day').map(row => row.kind === 'day' && row.label))
      .toEqual(['Hoje', 'Ontem', 'Hoje']);
  });
});
