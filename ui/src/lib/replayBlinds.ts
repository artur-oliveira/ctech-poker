import type {Action, HandHistoryAction} from '@/lib/api/table';

/** Replay and public-share hands render chip stacks and pot-relative sizing
 * against the table's big blind. Until the blind level is stored on the hand
 * itself (backend issue #75), recover it from the largest `post_big_blind`
 * amount logged in the timeline. Hands old enough to predate that action fall
 * back to FALLBACK_BIG_BLIND so `chipTier` stays finite. */
export const FALLBACK_BIG_BLIND = 25;

const POST_BIG_BLIND: Action = 'post_big_blind';

export function deriveBigBlind(
  actions: readonly Pick<HandHistoryAction, 'action' | 'amount'>[],
  stored?: number
): number {
  if (typeof stored === 'number' && stored > 0) return stored;
  let derived = 0;
  for (const action of actions) {
    if (action.action === POST_BIG_BLIND && action.amount > derived) derived = action.amount;
  }
  return derived > 0 ? derived : FALLBACK_BIG_BLIND;
}
