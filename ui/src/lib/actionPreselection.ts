import type {ActionPreselection, PokerAction} from '@/lib/api/table';

export type {ActionPreselection};

export function resolvePreselection(
  selection: ActionPreselection,
  legalActions: Iterable<PokerAction>
): PokerAction | null {
  const legal = new Set(legalActions);
  if (selection === 'check_fold' && legal.has('check')) return 'check';
  return legal.has('fold') ? 'fold' : null;
}
