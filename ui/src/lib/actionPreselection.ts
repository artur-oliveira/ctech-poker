import type {ActionPreselection, PokerAction} from '@/lib/api/table';

export type {ActionPreselection};

export function resolvePreselection(
  selection: ActionPreselection,
  legalActions: Iterable<PokerAction>,
  callAmount = 0,
  selectedCallAmount = 0
): PokerAction | null {
  const legal = new Set(legalActions);
  if (selection === 'check_fold' && legal.has('check')) return 'check';
  if (selection === 'check_fold' || selection === 'fold') return legal.has('fold') ? 'fold' : null;
  if (selection === 'call') {
    return legal.has('call') && callAmount === selectedCallAmount ? 'call' : null;
  }
  if (selection === 'all_in') {
    if (legal.has('raise')) return 'raise';
    return legal.has('call') ? 'call' : legal.has('check') ? 'check' : null;
  }
  if (legal.has('call')) return 'call';
  return legal.has('check') ? 'check' : null;
}
