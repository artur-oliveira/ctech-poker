'use client';
import {useCallback, useEffect, useRef, useState} from 'react';
import {getAccessToken, setAccessToken, setUsername, subscribeAccessToken} from '@/lib/api/client';
import {doRefresh} from '@/lib/auth/oauth';
import {cardLabel} from '@/lib/cards';
import {useWebSocket, type WSStatus} from '@aoctech/ws-client';
import {type MockScenario, MockTableService, USE_MOCK} from '@/lib/mock';
import type {PokerAction, ServerMessage, TableSnapshot} from '@/lib/api/table';
import {playerName} from '@/lib/utils';
import {playSound} from '@/lib/sound';

export type ConnectionStatus = WSStatus
export type ActionError = { code: string; message: string }

const ACTION_TIMEOUT_MS = 8000;
// A player parked at the table for hours (away, in a long hand, distracted)
// never issues another REST call, so nothing would otherwise notice the JWT
// is about to expire — the socket would reconnect-loop with the same stale
// token until @aoctech/ws-client's retry budget runs out and gives up for
// good. Refreshing well inside any realistic access-token lifetime keeps
// subscribeAccessToken's listener (already wired into useWebSocket below)
// firing with a live token before that ever happens; ws-client force-
// reconnects on a genuinely new token regardless of how many attempts it has
// already burned.
const TOKEN_REFRESH_INTERVAL_MS = 4 * 60 * 1000;

const ERROR_MESSAGES: Record<string, string> = {
  unauthorized: 'Sua sessão expirou. Entre novamente para continuar.',
  forbidden: 'Você não tem acesso a esta mesa.',
  not_found: 'Essa sala não está mais disponível.',
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

function describeSnapshot(previous: TableSnapshot | null, next: TableSnapshot, viewerId?: string) {
  const nameOf = (id: string) => next.seats.find(seat => seat.player_id === id)?.name;
  const playerLabel = (id: string) => playerName(id, viewerId, nameOf(id));
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
    messages.push(`${playerLabel(bettor.player_id)} colocou ${added.toLocaleString('pt-BR')} fichas no pote`);
  }
  if (next.current_player_id && next.current_player_id !== previous.current_player_id) {
    messages.push(next.current_player_id === viewerId ? 'Sua vez de agir' : `Vez de ${playerLabel(next.current_player_id)}`);
  }
  if (next.payouts && !previous.payouts) {
    messages.push(...Object.entries(next.payouts).filter(([, amount]) => amount > 0)
      .map(([playerId, amount]) => `${playerLabel(playerId)} ganhou ${amount.toLocaleString('pt-BR')} fichas`));
  }
  return messages.join('. ');
}

// Plays at most one sound per snapshot transition (never on every broadcast —
// each condition compares against the previous snapshot exactly like
// describeSnapshot does). Priority: a new board card beats an all-in beats a
// bet beats a fold-to-one reveal, since at most one usually fires per frame
// anyway.
function playSoundForTransition(previous: TableSnapshot | null, next: TableSnapshot, viewerId?: string) {
  if (!previous) return;
  // Table is busy with a lot going on at once — the turn ring alone is easy
  // to miss, so this fires independently of (and can co-occur with) whatever
  // else this transition triggers below (a bet, a fold-to-one reveal, etc).
  if (viewerId && next.current_player_id === viewerId && previous.current_player_id !== viewerId) {
    playSound('your_turn');
  }
  if (next.board.length > previous.board.length) {
    const added = next.board.length - previous.board.length;
    // Flop deals 3 cards one at a time (Board/PlayingCard stagger reveal
    // animation-delay by 220ms per index) — one reveal sound per card, timed
    // to match. Turn/river add a single card with no stagger.
    for (let i = 0; i < added; i++) {
      if (i === 0) playSound('reveal');
      else setTimeout(() => playSound('reveal'), i * 220);
    }
    return;
  }
  const previousSeats = new Map(previous.seats.map(seat => [seat.player_id, seat]));
  const wentAllIn = next.seats.some(seat => seat.state === 'all_in' && previousSeats.get(seat.player_id)?.state !== 'all_in');
  if (wentAllIn) {
    playSound('all_in');
    return;
  }
  const pot = previous.seats.reduce((n, seat) => n + seat.contributed, 0);
  const bettor = next.seats.find(seat => seat.contributed > (previousSeats.get(seat.player_id)?.contributed || 0));
  if (bettor) {
    const added = bettor.contributed - (previousSeats.get(bettor.player_id)?.contributed || 0);
    playSound(pot > 0 && added >= pot / 2 ? 'half_pot' : 'bet');
    return;
  }
  if (next.stage === 'complete' && previous.stage !== 'complete' && !next.won_without_showdown) playSound('reveal');
}

export function useTableRealtime(id: string, viewerId?: string, shareCode?: string, mockOptions?: {
  scenario?: MockScenario;
  delay?: number
}) {
  const pendingTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const pendingActionRef = useRef<PokerAction | null>(null);
  const previousSnapshot = useRef<TableSnapshot | null>(null);
  const sendRef = useRef<(value: object) => boolean>(() => false);
  // A mid-hand joiner is seated as pending_entry and stays that way forever
  // unless the client opts them in (PostBigBlindCmd) — the product intent is
  // an automatic buy-in for the next hand's big blind, no manual click, so
  // fire it once as soon as the viewer's own seat shows pending_entry. Reset
  // when they leave that state so a *later* pending_entry spell (re-joining
  // after leaving) posts again instead of being silently skipped.
  const postedBigBlindRef = useRef(false);
  // ready() and showCards() go straight through emit() with no server
  // round-trip tracking (unlike act(), which pendingActionRef already
  // guards) — a double-click/double-tap sends the frame twice. A short
  // synchronous ref lock (not state, so two clicks in the same tick can't
  // both read a stale "not pending" value) blocks the repeat; the mirrored
  // state only drives the button's disabled/pending visual.
  const readyLockRef = useRef(false);
  const showCardsLockRef = useRef(false);
  const [readyPending, setReadyPending] = useState(false);
  const [showCardsPending, setShowCardsPending] = useState(false);
  const [snapshot, setSnapshot] = useState<TableSnapshot | null>(null);
  // Captured once per snapshot (in this event handler, never during render) so
  // Seat can compute its countdown ring's remaining time as a pure function of
  // props (deadlineMs - snapshotAt) instead of calling Date.now() itself.
  const [snapshotAt, setSnapshotAt] = useState(0);
  const [unlock, setUnlock] = useState<{ key: string; stars: number } | null>(null);
  const [chat, setChat] = useState<{ player: string; message: string }[]>([]);
  const [pendingAction, setPendingAction] = useState<PokerAction | null>(null);
  const [lastActionError, setLastActionError] = useState<ActionError | null>(null);
  const [announcement, setAnnouncement] = useState('');
  const [removed, setRemoved] = useState<{ code?: string } | null>(null);
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
      playSoundForTransition(previousSnapshot.current, message.snapshot, viewerId);
      previousSnapshot.current = message.snapshot;
      if (liveMessage) setAnnouncement(liveMessage);
      setSnapshot(message.snapshot);
      setSnapshotAt(Date.now());
      clearPending();
      const ownSeat = message.snapshot.seats.find(seat => seat.player_id === viewerId);
      if (ownSeat?.state === 'pending_entry') {
        if (!postedBigBlindRef.current) {
          postedBigBlindRef.current = true;
          sendRef.current({type: 'post_big_blind'});
        }
      } else {
        postedBigBlindRef.current = false;
      }
    }
    if (message.type === 'error') failPending(message.code || 'unknown');
    if (message.type === 'removed') setRemoved({code: message.code});
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
    shareCode,
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

  useEffect(() => {
    if (USE_MOCK || !id) return () => {
    };
    const interval = setInterval(() => {
      void doRefresh().then(result => {
        if (result) {
          setAccessToken(result.accessToken);
          setUsername(result.username);
        }
      });
    }, TOKEN_REFRESH_INTERVAL_MS);
    return () => clearInterval(interval);
  }, [id]);

  // A backgrounded tab (screen lock, app switch) can have its WS silently
  // killed by the OS without a clean close event — the client-side heartbeat
  // only notices once it wakes up, and by then the socket may have already
  // burned through its whole reconnect budget. Forcing a reconnect attempt
  // the moment the tab becomes visible again skips straight past any
  // exhausted backoff instead of waiting on it.
  useEffect(() => {
    if (USE_MOCK || typeof document === 'undefined') return () => {
    };
    const onVisibility = () => {
      if (document.visibilityState === 'visible' && status !== 'connected') retryNow();
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, [status, retryNow]);

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
    status,
    snapshot,
    snapshotAt,
    unlock,
    chat,
    pendingAction,
    actionError: lastActionError,
    reconnectAttempt,
    announcement,
    removed,
    clearActionError: () => setLastActionError(null),
    retryNow,
    readyPending,
    showCardsPending,
    ready: (ready = true) => {
      if (readyLockRef.current) return false;
      readyLockRef.current = true;
      setReadyPending(true);
      setTimeout(() => {
        readyLockRef.current = false;
        setReadyPending(false);
      }, 1000);
      return emit({type: 'ready', ready});
    },
    act,
    showCards: () => {
      if (showCardsLockRef.current) return false;
      showCardsLockRef.current = true;
      setShowCardsPending(true);
      setTimeout(() => {
        showCardsLockRef.current = false;
        setShowCardsPending(false);
      }, 1000);
      const ok = emit({type: 'show_cards'});
      if (ok) playSound('showing_card');
      return ok;
    },
    sendChat: (message: string) => emit({type: 'chat', message})
  };
}
