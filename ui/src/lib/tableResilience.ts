import {CHAT_MESSAGE_MAX_LENGTH} from '@/lib/chat';

export type ActionError = { code: string; message: string };

/** The table's resilience vocabulary: how long a command may go unanswered,
 * how often it may be resubmitted, which rejection codes mean "your view of
 * the table is not the server's view", and what the player is told.
 *
 * Split out of `useTableRealtime` because none of it touches the socket or
 * any of that hook's refs — it is the part of the resilience state machine
 * that can be reasoned about, and tested, on its own. The semantics are
 * unchanged; only the location is. */
export const ACTION_TIMEOUT_MS = 8000;
// A stale_state rejection means the actor's own snapshot/hand precondition
// didn't match the server's — the same action resubmitted against the fresh
// version the resync just fetched is legal again (see actor.go's
// validateActionPrecondition). Cap the auto-resubmits so a genuinely illegal
// action (or a table stuck racing another player) fails visibly instead of
// looping forever.
export const MAX_ACTION_RETRIES = 3;
// Rejections that mean "your view of the table is not the server's view".
// invalid_action belongs here even though it is also the code for a genuinely
// illegal move: a resync costs one snapshot, while not resyncing leaves a
// player who hit a server-side desync stuck until they reload the page.
export const RESYNC_ERROR_CODES = new Set(['stale_state', 'rate_limited', 'invalid_action', 'unavailable']);
export const TERMINAL_ERROR_CODES = new Set(['forbidden', 'not_found']);
export const RESYNC_TIMEOUT_MS = 2500;
// First resubmit delay for an auxiliary command rejected against stale state,
// doubled per retry. Deliberately longer than the resync backoff scheduled for
// the same action_id (<=450ms on a first rejection) so the resubmit is judged
// against the state that resync pulled, not the one that just rejected it.
export const AUX_RETRY_BASE_MS = 700;

export const ERROR_MESSAGES: Record<string, string> = {
  unauthorized: 'Sua sessão expirou. Entre novamente para continuar.',
  forbidden: 'Você não tem acesso a esta mesa.',
  not_found: 'Essa sala não está mais disponível.',
  unavailable: 'A mesa está indisponível no momento. Tente reconectar.',
  rate_limited: 'Muitas ações em sequência. Aguarde um instante e tente novamente.',
  invalid_action: 'Essa ação não é mais válida. Confira o estado atual da mesa.',
  missing_action_id: 'A ação não pôde ser identificada. Atualize a página e tente novamente.',
  missing_precondition: 'O estado da mesa ainda não está pronto para receber essa ação.',
  stale_state: 'A mesa mudou antes da sua ação. Sincronizando o estado mais recente.',
  invalid_post: 'Não foi possível confirmar o blind. Tente novamente.',
  message_too_long: `A mensagem ultrapassa o limite de ${CHAT_MESSAGE_MAX_LENGTH} caracteres.`,
  not_connected: 'Sem conexão com a mesa. Reconecte antes de agir.',
  action_timeout: 'A mesa demorou para confirmar a ação. O estado será atualizado antes de uma nova tentativa.',
  bot_challenge_required: 'Conclua a verificação para continuar jogando.',
  bot_challenge_failed: 'A verificação não foi concluída. Tente novamente.',
  connection_lost: 'A conexão caiu antes da confirmação. Aguarde a atualização da mesa.'
};

/** Shown in the reconnect notice while a table is being moved between server
 * instances (a deploy or a spot-termination drain), if the server's own
 * `table_migrating` frame carries no text. The reconnect is transparent —
 * this only sets expectations. See api issue #354. */
export const MIGRATION_NOTICE_FALLBACK =
  'Esta mesa está migrando de servidor. Sua conexão será restabelecida automaticamente em instantes.';

export function actionError(code = 'unknown'): ActionError {
  return {code, message: ERROR_MESSAGES[code] || 'Não foi possível concluir a ação. Tente novamente.'};
}

/** How long to wait before resubmitting an auxiliary command the server
 * rejected against stale state: `AUX_RETRY_BASE_MS` doubled per retry, plus
 * jitter so two clients racing the same table do not resubmit in lockstep.
 * `retries` is 1 on the first resubmit. */
export function auxRetryDelayMs(retries: number, jitter = Math.random()) {
  return AUX_RETRY_BASE_MS * 2 ** (retries - 1) + Math.floor(jitter * 200);
}
