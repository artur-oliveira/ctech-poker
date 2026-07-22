'use client';
import {useCallback, useEffect, useRef, useState} from 'react';
import {getAccessToken, subscribeAccessToken} from '@/lib/api/client';
import {cardLabel} from '@/lib/cards';
import {useWebSocket, type WSStatus} from '@aoctech/ws-client';
import {type MockScenario, MockTableService, USE_MOCK} from '@/lib/mock';
import type {PokerAction, ServerMessage, TableSnapshot} from '@/lib/api/table';
import {playerName} from '@/lib/utils';

export type ConnectionStatus = WSStatus
export type ActionError = { code: string; message: string }

const ACTION_TIMEOUT_MS = 8000;

const ERROR_MESSAGES: Record<string, string> = {
  unauthorized: 'Sua sessão expirou. Entre novamente para continuar.',
  forbidden: 'Você não tem acesso a esta mesa.',
  unavailable: 'A mesa está indisponível no momento. Tente reconectar.',
  rate_limited: 'Muitas ações em sequência. Aguarde um instante e tente novamente.',
  invalid_action: 'Essa ação não é mais válida. Confira o estado atual da mesa.',
  invalid_post: 'Não foi possível confirmar o blind. Tente novamente.',
  message_too_long: 'A mensagem ultrapassa o limite de 500 caracteres.',
  not_connected: 'Sem conexão com a mesa. Reconecte antes de agir.',
  action_timeout: 'A mesa demorou para confirmar a ação. O estado será atualizado antes de uma nova tentativa.',
  connection_lost: 'A conexão caiu antes da confirmação. Aguarde a atualização da mesa.'
};

function actionError(code = 'unknown'): ActionError {
  return {code, message: ERROR_MESSAGES[code] || 'Não foi possível concluir a ação. Tente novamente.'};
}

const STAGE_LABELS: Record<string, string> = {
  waiting_for_players: 'aguardando jogadores', pre_flop: 'pré-flop', flop: 'flop', turn: 'turn', river: 'river',
  showdown: 'showdown', complete: 'mão encerrada'
};

function playerLabel(playerId: string, viewerId?: string) {
  return playerName(playerId, viewerId);
}

function describeSnapshot(previous: TableSnapshot | null, next: TableSnapshot, viewerId?: string) {
  if (!previous) return `Mesa atualizada. ${STAGE_LABELS[next.stage] || next.stage}.`;
  const messages: string[] = [];
  if (next.stage !== previous.stage) messages.push(`Etapa: ${STAGE_LABELS[next.stage] || next.stage}`);
  if (next.board.length > previous.board.length) {
    const dealt = next.board.slice(previous.board.length).map(cardLabel).join(', ');
    messages.push(`${next.board.length === 3 ? 'Flop' : next.board.length === 4 ? 'Turn' : 'River'}: ${dealt}`);
  }
  const previousSeats = new Map(previous.seats.map(seat => [seat.player_id, seat]));
  const bettor = next.seats.find(seat => seat.contributed > (previousSeats.get(seat.player_id)?.contributed || 0));
  if (bettor) {
    const added = bettor.contributed - (previousSeats.get(bettor.player_id)?.contributed || 0);
    messages.push(`${playerLabel(bettor.player_id, viewerId)} colocou ${added.toLocaleString('pt-BR')} fichas no pote`);
  }
  if (next.current_player_id && next.current_player_id !== previous.current_player_id) {
    messages.push(next.current_player_id === viewerId ? 'Sua vez de agir' : `Vez de ${playerLabel(next.current_player_id, viewerId)}`);
  }
  if (next.payouts && !previous.payouts) {
    messages.push(...Object.entries(next.payouts).filter(([, amount]) => amount > 0)
      .map(([playerId, amount]) => `${playerLabel(playerId, viewerId)} ganhou ${amount.toLocaleString('pt-BR')} fichas`));
  }
  return messages.join('. ');
}

export function useTableRealtime(id: string, viewerId?: string, mockOptions?: {
  scenario?: MockScenario;
  delay?: number
}) {
  const pendingTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const pendingActionRef = useRef<PokerAction | null>(null);
  const previousSnapshot = useRef<TableSnapshot | null>(null);
  const sendRef = useRef<(value: object) => boolean>(() => false);
  const [snapshot, setSnapshot] = useState<TableSnapshot | null>(null);
  const [unlock, setUnlock] = useState<{ key: string; stars: number } | null>(null);
  const [chat, setChat] = useState<{ player: string; message: string }[]>([]);
  const [pendingAction, setPendingAction] = useState<PokerAction | null>(null);
  const [lastActionError, setLastActionError] = useState<ActionError | null>(null);
  const [announcement, setAnnouncement] = useState('');
  const [mockStatus, setMockStatus] = useState<WSStatus>('connecting');
  const [mockReconnectAttempt, setMockReconnectAttempt] = useState(0);
  const mockService = useRef<MockTableService | null>(null);

  const clearPending = useCallback(() => {
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
    pendingTimer.current = undefined;
    pendingActionRef.current = null;
    setPendingAction(null);
  }, []);

  const failPending = useCallback((code: string) => {
    clearPending();
    setLastActionError(actionError(code));
  }, [clearPending]);

  const receive = useCallback((message: ServerMessage) => {
    if (message.type === 'state' && message.snapshot) {
      const liveMessage = describeSnapshot(previousSnapshot.current, message.snapshot, viewerId);
      previousSnapshot.current = message.snapshot;
      if (liveMessage) setAnnouncement(liveMessage);
      setSnapshot(message.snapshot);
      clearPending();
    }
    if (message.type === 'error') failPending(message.code || 'unknown');
    if (message.type === 'achievement_unlocked' && message.key) setUnlock({
      key: message.key,
      stars: message.stars || 1
    });
    if (message.type === 'chat' && message.message) {
      const chatMessage = message.message;
      setChat(value => [...value.slice(-39), {player: message.player_id || '?', message: chatMessage}]);
    }
  }, [clearPending, failPending, viewerId]);

  const origin = (process.env.NEXT_PUBLIC_API_URL || (typeof window !== 'undefined' ? window.location.origin : '')).replace(/^http/, 'ws');
  const wsUrl = id ? `${origin}/v1.0/tables/${encodeURIComponent(id)}/ws` : null;
  const subscribeToken = useCallback((callback: (token: string) => void) => subscribeAccessToken(token => {
    if (token) callback(token);
  }), []);
  const handleOpen = useCallback(() => {
    sendRef.current({type: 'ping'});
  }, []);
  const {status: wsStatus, attempt: wsReconnectAttempt, send: wsSend, reconnect: wsRetryNow} = useWebSocket({
    url: wsUrl,
    onMessage: data => receive(data as ServerMessage),
    enabled: Boolean(wsUrl) && !USE_MOCK,
    authToken: getAccessToken() || undefined,
    subscribeToken,
    onOpen: handleOpen
  });
  const mockScenario = mockOptions?.scenario || 'flop';
  const mockDelay = Math.min(15000, Math.max(0, mockOptions?.delay ?? 650));
  useEffect(() => {
    if (!USE_MOCK || !id) return () => {

    };
    previousSnapshot.current = null;
    const service = new MockTableService(mockScenario, mockDelay, {
      onMessage: receive,
      onStatus: (next, attempt) => {
        setMockStatus(next);
        setMockReconnectAttempt(attempt);
      }
    });
    mockService.current = service;
    service.connect();
    return () => {
      service.close();
      if (mockService.current === service) mockService.current = null;
    };
  }, [id, mockDelay, mockScenario, receive]);

  const send = useCallback((value: object) => USE_MOCK ? Boolean(mockService.current?.send(value as Record<string, unknown>)) : wsSend(value), [wsSend]);
  const retryNow = useCallback(() => USE_MOCK ? mockService.current?.reconnect() : wsRetryNow(), [wsRetryNow]);
  const status = USE_MOCK ? mockStatus : wsStatus;
  const reconnectAttempt = USE_MOCK ? mockReconnectAttempt : wsReconnectAttempt;
  useEffect(() => {
    sendRef.current = send;
  }, [send]);

  useEffect(() => () => {
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
  }, []);

  const emit = useCallback((value: object) => {
    if (!send(value)) {
      setLastActionError(actionError('not_connected'));
      return false;
    }
    return true;
  }, [send]);

  const act = useCallback((action: PokerAction, amount = 0) => {
    if (pendingActionRef.current) return false;
    setLastActionError(null);
    if (!emit({type: 'act', action, amount, action_id: crypto.randomUUID()})) return false;
    pendingActionRef.current = action;
    setPendingAction(action);
    pendingTimer.current = setTimeout(() => {
      clearPending();
      if (!send({type: 'ping'})) {
        setLastActionError(actionError('connection_lost'));
        return;
      }
      setLastActionError(actionError('action_timeout'));
    }, ACTION_TIMEOUT_MS);
    return true;
  }, [clearPending, emit, send]);

  return {
    status, snapshot, unlock, chat, pendingAction, actionError: lastActionError, reconnectAttempt, announcement,
    clearActionError: () => setLastActionError(null), retryNow,
    ready: (ready = true) => emit({type: 'ready', ready}), act,
    sendChat: (message: string) => emit({type: 'chat', message})
  };
}
