import {
  ArrowRight,
  ArrowUp,
  Check,
  ChevronRight,
  CircleDollarSign,
  Eye,
  HelpCircle,
  LogIn,
  LogOut,
  LucideIcon,
  Pause,
  Play,
  TrendingUp,
  Trophy,
  WifiOff,
  X,
} from 'lucide-react';

import {Action, HandHistoryAction} from '@/lib/api/table';
import React from "react";

const ACTION_META: Record<Action, { label: string; Icon: LucideIcon }> = {
  // Sistema
  join: {
    label: 'Entrou na mesa',
    Icon: LogIn,
  },
  leave: {
    label: 'Saiu da mesa',
    Icon: LogOut,
  },
  ready: {
    label: 'Pronto para jogar',
    Icon: Play,
  },
  not_ready: {
    label: 'Não está pronto',
    Icon: Pause,
  },
  sit_out: {
    label: 'Entrou em Sit Out',
    Icon: Pause,
  },
  disconnect_sit_out: {
    label: 'Desconectou (Sit Out)',
    Icon: WifiOff,
  },

  // Fluxo da partida
  next_hand: {
    label: 'Nova mão',
    Icon: ChevronRight,
  },
  runout_step: {
    label: 'Runout',
    Icon: ArrowRight,
  },
  escalate_blinds: {
    label: 'Blinds aumentaram',
    Icon: TrendingUp,
  },
  post_big_blind: {
    label: 'Postou o Big Blind',
    Icon: CircleDollarSign,
  },

  // Ações do jogador
  check: {
    label: 'Check',
    Icon: Check,
  },
  fold: {
    label: 'Fold',
    Icon: X,
  },
  call: {
    label: 'Call',
    Icon: ArrowRight,
  },
  bet: {
    label: 'Bet',
    Icon: CircleDollarSign,
  },
  raise: {
    label: 'Raise',
    Icon: ArrowUp,
  },
  all_in: {
    label: 'All-in',
    Icon: ArrowUp,
  },
  show_cards: {
    label: 'Mostrou as cartas',
    Icon: Eye,
  },

  // Resultado da mão
  won: {
    label: 'Venceu',
    Icon: Trophy,
  },
  tie: {
    label: 'Empatou',
    Icon: Trophy,
  },
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
      const meta = ACTION_META[a.action] || {label: a.action, Icon: HelpCircle};
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
