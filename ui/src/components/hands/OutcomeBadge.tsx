import {Equal, TrendingDown, TrendingUp} from 'lucide-react';
import type {HandOutcome} from '@/lib/api/player';

const OUTCOME_META: Record<HandOutcome, { label: string; Icon: typeof TrendingUp }> = {
  won: {label: 'Vitória', Icon: TrendingUp},
  lost: {label: 'Derrota', Icon: TrendingDown},
  tied: {label: 'Empate', Icon: Equal}
};

export function OutcomeBadge({outcome}: { outcome: HandOutcome }) {
  const {label, Icon} = OUTCOME_META[outcome];
  return <span className={`outcome-badge ${outcome}`}><Icon aria-hidden="true"/>{label}</span>;
}
