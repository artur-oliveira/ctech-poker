import {
  ArrowRight,
  ArrowUp,
  Check,
  ChevronRight,
  CircleDollarSign,
  Clock3,
  Eye,
  HelpCircle,
  LogIn,
  LogOut,
  LucideIcon,
  MessageSquare,
  Pause,
  Play,
  Repeat2,
  Smile,
  TrendingUp,
  Trophy,
  UserPen,
  WifiOff,
  X,
} from 'lucide-react';

import {Action, HandHistoryAction} from '@/lib/api/table';
import {isTableReaction, TABLE_REACTIONS} from '@/lib/reactions';
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
    label: 'Fora da rodada',
    Icon: Pause,
  },
  disconnect_sit_out: {
    label: 'Desconectou (Sit Out)',
    Icon: WifiOff,
  },
  keep_seat: {
    label: 'Confirmou presença',
    Icon: Clock3,
  },
  set_run_it_twice: {
    label: 'Ajustou Rodar Duas Vezes',
    Icon: Repeat2,
  },
  
  // Fluxo da partida
  next_hand: {
    label: 'Nova mão',
    Icon: ChevronRight,
  },
  runout_step: {
    label: 'Abriu o board',
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

  // Mesa social
  chat: {
    label: 'Falou no chat',
    Icon: MessageSquare,
  },
  reaction: {
    label: 'Reagiu',
    Icon: Smile,
  },
  set_identity: {
    label: 'Atualizou o perfil',
    Icon: UserPen,
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
      const meta = ACTION_META[a.action] || {label: a.action.replaceAll('_', ' '), Icon: HelpCircle};
      const Icon = meta.Icon;
      const reaction = a.reaction_id && isTableReaction(a.reaction_id) ? TABLE_REACTIONS[a.reaction_id] : null;
      return <li key={a.seq} className={`action-row action-${a.action}`}
                 style={{'--delay': `${Math.min(i, 12) * 30}ms`} as React.CSSProperties}>
        <span className="action-row-icon" aria-hidden="true"><Icon/></span>
        <span className="action-row-who">{resolveName(a.player_id)}</span>
        <span className="action-row-what">
          {meta.label}
          {reaction && <>
            {' '}<span className="action-row-emoji" role="img" aria-label={reaction.label}>{reaction.glyph}</span>
            {a.target_player_id && <> para {resolveName(a.target_player_id)}</>}
          </>}
          {a.amount > 0 && <b>{a.amount.toLocaleString('pt-BR')}</b>}
        </span>
        <span className="action-row-when">{a.timestamp ? formatTime(a.timestamp) : '—'}</span>
      </li>;
    })}
  </ol>;
}
