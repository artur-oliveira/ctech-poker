// Global, dependency-free API error notifier. The axios response interceptor
// pipes every API failure through notifyApiError so the user always sees a
// clear message; callers that render their own inline error (forms, gates)
// opt out per-request with `silentError`.

export type NotificationVariant = 'error' | 'info'

export interface NotificationAction {
  label: string
  run: () => void | Promise<void>
}

export interface AppNotification {
  id: string
  message: string
  variant: NotificationVariant
  // Optional inline buttons. An ignored toast still auto-dismisses on the
  // normal timer: every action offered here also exists on a durable surface.
  actions?: NotificationAction[]
}

const listeners = new Set<(items: AppNotification[]) => void>();
let items: AppNotification[] = [];
const recent = new Map<string, number>();

const DEDUPE_MS = 600;
// Plain info toasts repeat a durable surface, so they clear themselves quickly.
const AUTO_DISMISS_MS = 6000;
// Errors and any toast carrying an action (retry, invite accept/decline) must
// survive a read: they stay until dismissed, with this hard ceiling so a stack
// of ignored toasts still drains on its own.
const PERSISTENT_DISMISS_MS = 20000;
const MAX_VISIBLE = 3;
let nextID = 0;

export function pushNotification(message: string, variant: NotificationVariant = 'error',
                                 actions?: NotificationAction[]): void {
  const now = Date.now();
  if (now - (recent.get(message) || 0) < DEDUPE_MS) return;
  recent.set(message, now);
  const id = `${now}-${nextID++}`;
  items = [...items, {id, message, variant, actions}].slice(-MAX_VISIBLE);
  listeners.forEach(f => f(items));
  const persistent = variant === 'error' || (actions?.length ?? 0) > 0;
  setTimeout(() => dismissNotification(id), persistent ? PERSISTENT_DISMISS_MS : AUTO_DISMISS_MS);
}

export function dismissNotification(id: string): void {
  items = items.filter(n => n.id !== id);
  listeners.forEach(f => f(items));
}

export function subscribeNotifications(f: (items: AppNotification[]) => void): () => void {
  listeners.add(f);
  return () => {
    listeners.delete(f);
  };
}

const STATUS_MESSAGES: Record<number, string> = {
  400: 'Não foi possível concluir a ação. Verifique os dados e tente novamente.',
  401: 'Sua sessão expirou. Entre novamente para continuar.',
  403: 'Acesso negado. Você não tem permissão para essa ação.',
  404: 'O recurso solicitado não foi encontrado.',
  409: 'Não foi possível concluir: o recurso já foi alterado ou está indisponível.',
  429: 'Muitas solicitações. Aguarde um instante e tente novamente.'
};

function messageForStatus(status?: number): string {
  if (status && STATUS_MESSAGES[status]) return STATUS_MESSAGES[status];
  if (status && status >= 500) return 'O servidor falhou ao processar a solicitação. Tente novamente em alguns instantes.';
  return 'Algo deu errado. Tente novamente.';
}

// #319: next_action/retry_after_seconds (RFC 9457 extension members) tell the
// client HOW to recover — retry, wait a known window, sign in again, or give
// up and contact support — instead of it pattern-matching `detail` text. Only
// ever overrides the generic status message when the server actually said
// something the plain status code doesn't already convey (a concrete wait
// time, or that retrying is pointless). Every automatic-retry decision
// (RETRYABLE_HTTP_STATUSES) already ran before this is ever shown — this is
// only reached once those are exhausted, so it can't duplicate a retry.
function messageForNextAction(nextAction?: string, retryAfterSeconds?: number): string | undefined {
  if ((nextAction === 'retry' || nextAction === 'wait') && retryAfterSeconds) {
    return `Muitas solicitações. Tente novamente em ${retryAfterSeconds}s.`;
  }
  if (nextAction === 'contact_support') {
    return 'Não foi possível concluir. Se o problema persistir, contate o suporte.';
  }
  return undefined;
}

export function notifyApiError(error: unknown): void {
  const normalized = error as {
    name?: string; status?: number;
    problem?: { detail?: string; title?: string; next_action?: string; retry_after_seconds?: number };
  };
  if (normalized?.name === 'ApiError') {
    const original = (error as {
      original?: {
        code?: string;
        message?: string;
        response?: unknown;
        request?: unknown;
      }
    }).original;
    if (!normalized.status && original && !original.response) {
      const timedOut = original.code === 'ECONNABORTED' || /timeout/i.test(original.message || '');
      pushNotification(timedOut
        ? 'O servidor demorou para responder. Verifique sua conexão e tente novamente.'
        : 'Sem conexão com o servidor. Verifique sua internet e tente novamente.');
      return;
    }
    const safeDetail = normalized.problem?.detail?.trim();
    const actionMessage = messageForNextAction(normalized.problem?.next_action, normalized.problem?.retry_after_seconds);
    pushNotification(actionMessage || safeDetail || messageForStatus(normalized.status));
    return;
  }
  const axiosErr = error as { isAxiosError?: boolean; response?: { status?: number }; request?: unknown };
  if (!axiosErr?.isAxiosError) {
    pushNotification('Algo deu errado. Tente novamente.');
    return;
  }
  if (!axiosErr.response) {
    pushNotification('Sem conexão com o servidor. Verifique sua internet e tente novamente.');
    return;
  }
  pushNotification(messageForStatus(axiosErr.response.status));
}
