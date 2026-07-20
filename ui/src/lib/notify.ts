// Global, dependency-free API error notifier. The axios response interceptor
// pipes every API failure through notifyApiError so the user always sees a
// clear message; callers that render their own inline error (forms, gates)
// opt out per-request with `silentError`.

export type NotificationVariant = 'error' | 'info'

export interface AppNotification {
  id: string
  message: string
  variant: NotificationVariant
}

const listeners = new Set<(items: AppNotification[]) => void>()
let items: AppNotification[] = []
const recent = new Map<string, number>()

const DEDUPE_MS = 600
const AUTO_DISMISS_MS = 6000

export function pushNotification(message: string, variant: NotificationVariant = 'error'): void {
  const now = Date.now()
  if (now - (recent.get(message) || 0) < DEDUPE_MS) return
  recent.set(message, now)
  const id = `${now}-${items.length}`
  items = [...items, {id, message, variant}]
  listeners.forEach(f => f(items))
  setTimeout(() => dismissNotification(id), AUTO_DISMISS_MS)
}

export function dismissNotification(id: string): void {
  items = items.filter(n => n.id !== id)
  listeners.forEach(f => f(items))
}

export function subscribeNotifications(f: (items: AppNotification[]) => void): () => void {
  listeners.add(f)
  return () => {
    listeners.delete(f)
  }
}

const STATUS_MESSAGES: Record<number, string> = {
  400: 'Não foi possível concluir a ação. Verifique os dados e tente novamente.',
  401: 'Sua sessão expirou. Entre novamente para continuar.',
  403: 'Acesso negado. Você não tem permissão para essa ação.',
  404: 'O recurso solicitado não foi encontrado.',
  409: 'Não foi possível concluir: o recurso já foi alterado ou está indisponível.',
  429: 'Muitas solicitações. Aguarde um instante e tente novamente.'
}

function messageForStatus(status?: number): string {
  if (status && STATUS_MESSAGES[status]) return STATUS_MESSAGES[status]
  if (status && status >= 500) return 'O servidor falhou ao processar a solicitação. Tente novamente em alguns instantes.'
  return 'Algo deu errado. Tente novamente.'
}

type ApiErrorBody = { detail?: string; title?: string }

export function notifyApiError(error: unknown): void {
  const axiosErr = error as { isAxiosError?: boolean; response?: { status?: number }; request?: unknown }
  if (!axiosErr?.isAxiosError) {
    pushNotification('Algo deu errado. Tente novamente.')
    return
  }
  if (!axiosErr.response) {
    pushNotification('Sem conexão com o servidor. Verifique sua internet e tente novamente.')
    return
  }
  pushNotification(messageForStatus(axiosErr.response.status))
}
