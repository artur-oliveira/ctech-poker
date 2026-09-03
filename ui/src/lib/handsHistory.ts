// Client-side shaping of the loaded hand-history pages (#115): outcome/table
// filters, day grouping and the "carregadas" roll-up. All pure, all over the
// pages already in the react-query cache — filtering must not cost a refetch,
// which is the whole point of the acceptance criterion "filter to só vitórias
// without a full reload".
import type {HandItem, HandOutcome} from '@/lib/api/player';

export type OutcomeFilter = 'all' | HandOutcome;

export const ALL_TABLES = 'all';

export interface HandsFilter {
  outcome: OutcomeFilter;
  tableId: string;
}

export const NO_FILTER: HandsFilter = {outcome: 'all', tableId: ALL_TABLES};

export function filterHands(hands: HandItem[], filter: HandsFilter): HandItem[] {
  return hands.filter(hand =>
    (filter.outcome === 'all' || hand.outcome === filter.outcome) &&
    (filter.tableId === ALL_TABLES || hand.table_id === filter.tableId));
}

export interface LoadedTotals {
  total: number;
  netSum: number;
  wins: number;
  ties: number;
  losses: number;
  winRate: number;
}

/** Roll-up of exactly the hands passed in — the loaded subset, never lifetime. */
export function loadedTotals(hands: HandItem[]): LoadedTotals | null {
  if (!hands.length) return null;
  let netSum = 0;
  let wins = 0;
  let ties = 0;
  let losses = 0;
  for (const hand of hands) {
    netSum += hand.net_change;
    if (hand.outcome === 'won') wins++;
    else if (hand.outcome === 'tied') ties++;
    else losses++;
  }
  return {total: hands.length, netSum, wins, ties, losses, winRate: Math.round((wins / hands.length) * 100)};
}

export interface TableOption {
  tableId: string;
  count: number;
}

/** The tables present in the loaded pages, busiest first, so the filter chips
 * only ever offer a table the player can actually see rows for. */
export function handTables(hands: HandItem[]): TableOption[] {
  const counts = new Map<string, number>();
  for (const hand of hands) counts.set(hand.table_id, (counts.get(hand.table_id) ?? 0) + 1);
  return [...counts.entries()]
    .map(([tableId, count]) => ({tableId, count}))
    .sort((a, b) => b.count - a.count || a.tableId.localeCompare(b.tableId));
}

export type HandsRow =
  | {kind: 'day'; key: string; label: string; count: number}
  | {kind: 'hand'; key: string; day: string; hand: HandItem};

function dayKey(endedAt: number): string {
  const date = new Date(endedAt);
  return `${date.getFullYear()}-${date.getMonth() + 1}-${date.getDate()}`;
}

/** "Hoje" / "Ontem" for the two days a player recognizes at a glance, the full
 * pt-BR date for everything older. */
export function dayLabel(endedAt: number, nowMs: number = Date.now()): string {
  const key = dayKey(endedAt);
  if (key === dayKey(nowMs)) return 'Hoje';
  if (key === dayKey(nowMs - 24 * 3600_000)) return 'Ontem';
  return new Date(endedAt).toLocaleDateString('pt-BR', {day: '2-digit', month: 'long', year: 'numeric'});
}

/**
 * Flattens the list into `[day header, …hands, day header, …hands]`. Flat
 * because the virtualizer measures one row at a time; the day a row belongs to
 * rides along on the row so the sticky bar above the list can name whichever
 * day is currently in view without re-deriving it.
 */
export function groupHandsByDay(hands: HandItem[], nowMs: number = Date.now()): HandsRow[] {
  const rows: HandsRow[] = [];
  let currentKey = '';
  let header: Extract<HandsRow, {kind: 'day'}> | null = null;
  for (const hand of hands) {
    const key = dayKey(hand.ended_at);
    if (key !== currentKey) {
      currentKey = key;
      // The row index is in the key, not just the day: the list is server-
      // ordered, so the same day can legitimately reappear further down and
      // two headers keyed `day-<date>` would collide in the virtualizer.
      header = {kind: 'day', key: `day-${key}-${rows.length}`, label: dayLabel(hand.ended_at, nowMs), count: 0};
      rows.push(header);
    }
    if (header) header.count++;
    rows.push({kind: 'hand', key: hand.hand_id, day: header?.label ?? '', hand});
  }
  return rows;
}
