// Client-side shaping of the loaded hand-history pages (#115): day grouping
// and the "nesta lista" roll-up. All pure, all over the pages already in the
// react-query cache. The outcome/table filters that used to live here were
// removed: they could only ever filter the pages fetched so far, so their
// counts and results misled anyone with more history than one page.
import type {HandItem} from '@/lib/api/player';

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

/** A table id is a 26-char ULID that names nothing a player memorizes, yet the
 * head and tail together are enough to tell two tables apart. Ellipsis-in-the-
 * middle keeps both ends readable where a CSS trailing clip would eat the tail. */
export function shortTableId(tableId: string, edge = 4): string {
  if (tableId.length <= edge * 2 + 1) return tableId;
  return `${tableId.slice(0, edge)}…${tableId.slice(-edge)}`;
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
