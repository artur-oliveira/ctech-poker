import type {HandItem} from '@/lib/api/player';
import type {HandHistoryAction} from '@/lib/api/table';

const OUTCOMES = {won: 'Vitória', lost: 'Derrota', tied: 'Empate'} as const;
const ACTIONS: Record<string, string> = {
  fold: 'Fold', check: 'Check', call: 'Call', bet: 'Bet', raise: 'Raise',
  all_in: 'All-in', show_cards: 'Mostrou cartas', won: 'Venceu', tie: 'Empatou'
};

export function serializeHand(hand: HandItem, actions: HandHistoryAction[], viewerId?: string, actionsAvailable = true) {
  const names = new Map((hand.opponents || []).map(opponent => [opponent.player_id, opponent.name || 'Adversário']));
  const nameOf = (id: string) => id === viewerId ? 'Você' : names.get(id) || (id ? `Jogador ${id.slice(0, 8)}` : 'Mesa');
  const lines = [
    `CTech Poker: Mão ${hand.hand_id}`,
    `Mesa: ${hand.table_id}`,
    `Encerrada em: ${new Date(hand.ended_at).toLocaleString('pt-BR')}`,
    `Resultado: ${OUTCOMES[hand.outcome]} (${hand.net_change >= 0 ? '+' : ''}${hand.net_change} fichas)`,
    '',
    `Suas cartas: ${hand.hole_cards?.join(' ') || 'não disponíveis'}`,
    `Board: ${hand.board?.join(' ') || 'sem board'}`
  ];
  for (const opponent of hand.opponents || []) {
    lines.push(`${opponent.name || 'Adversário'}: ${opponent.hole_cards?.join(' ') || 'cartas não reveladas'}${opponent.won ? ' (vencedor)' : ''}`);
  }
  lines.push('', 'Ações:');
  if (!actionsAvailable) {
    lines.push('Indisponíveis no momento da exportação. O resumo da mão acima continua válido.');
  } else if (!actions.length) {
    lines.push('Nenhuma ação registrada.');
  } else {
    for (const action of actions) {
      const time = action.timestamp ? new Date(action.timestamp).toLocaleTimeString('pt-BR') : '--:--:--';
      const amount = action.amount > 0 ? ` ${action.amount.toLocaleString('pt-BR')}` : '';
      lines.push(`${time}  ${nameOf(action.player_id)}: ${ACTIONS[action.action] || action.action}${amount}`);
    }
  }
  if (hand.commit_hash) lines.push('', `Commit hash: ${hand.commit_hash}`);
  return `${lines.join('\n')}\n`;
}
