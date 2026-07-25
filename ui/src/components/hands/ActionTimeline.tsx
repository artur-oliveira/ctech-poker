import {ArrowUp, Check, Equal, X} from 'lucide-react';
import type {HandHistoryAction} from '@/lib/api/table';

const ACTION_META: Record<string, { label: string; Icon: typeof Check }> = {
  fold: {label: 'Fold', Icon: X},
  check: {label: 'Check', Icon: Check},
  call: {label: 'Call', Icon: Equal},
  raise: {label: 'Raise', Icon: ArrowUp},
  bet: {label: 'Bet', Icon: ArrowUp},
  all_in: {label: 'All-in', Icon: ArrowUp}
};

function formatTime(unixMillis: number) {
  return new Date(unixMillis).toLocaleTimeString('pt-BR', {hour: '2-digit', minute: '2-digit', second: '2-digit'});
}

export function ActionTimeline({actions, resolveName}: {
  actions: HandHistoryAction[];
  resolveName: (playerId: string) => string;
}) {
  if (!actions.length) return <p className="action-timeline-empty">Nenhuma ação registrada para esta mão.</p>;

  return <ol className="action-timeline">
    {actions.map((a, i) => {
      const meta = ACTION_META[a.action] || {label: a.action, Icon: Equal};
      const Icon = meta.Icon;
      return <li key={a.seq} className={`action-row action-${a.action}`}
                 style={{'--delay': `${Math.min(i, 12) * 30}ms`} as React.CSSProperties}>
        <span className="action-row-icon" aria-hidden="true"><Icon/></span>
        <span className="action-row-who">{resolveName(a.player_id)}</span>
        <span className="action-row-what">{meta.label}{a.amount > 0 &&
            <b>{a.amount.toLocaleString('pt-BR')}</b>}</span>
        <span className="action-row-when">{a.timestamp ? formatTime(a.timestamp) : '—'}</span>
      </li>;
    })}
  </ol>;
}
