'use client';
import {useCallback, useEffect, useRef, useState} from 'react';
import {
  getAccessToken,
  setAccessToken,
  setPlayerId,
  setUsername,
  subscribeAccessToken
} from '@/lib/api/client';
import {doRefresh} from '@/lib/auth/oauth';
import {cardLabel} from '@/lib/cards';
import {useWebSocket, type WSStatus} from '@aoctech/ws-client';
import type {MockTableService} from '@/dev/mockRuntime';
import {type MockScenario, USE_MOCK} from '@/lib/mockConfig';
import type {ActionPreselection, PokerAction, ServerMessage, TableSnapshot} from '@/lib/api/table';
import {playerName} from '@/lib/utils';
import {playSound} from '@/lib/sound';
import {decodeServerMessage, encodeClientMessage} from "@/lib/ws/utils";
import {isTableReaction, type TableReactionEvent, type TableReactionID} from '@/lib/reactions';

export type ConnectionStatus = WSStatus
export type ActionError = { code: string; message: string }

const ACTION_TIMEOUT_MS = 8000;
// A player parked at the table for hours (away, in a long hand, distracted)
// never issues another REST call, so nothing would otherwise notice the JWT
// is about to expire, so the socket would reconnect-loop with the same stale
// token until @aoctech/ws-client's retry budget runs out and gives up for
// good. Refreshing well inside any realistic access-token lifetime keeps
// the in-memory credential current before that ever happens. The active
// socket remains authenticated for its lifetime and a real reconnect reads
// the latest authToken prop. Subscribing the socket to token changes would
// deliberately close a healthy connection after every silent refresh.
const TOKEN_REFRESH_INTERVAL_MS = 4 * 60 * 1000;

const ERROR_MESSAGES: Record<string, string> = {
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
  message_too_long: 'A mensagem ultrapassa o limite de 500 caracteres.',
  not_connected: 'Sem conexão com a mesa. Reconecte antes de agir.',
  action_timeout: 'A mesa demorou para confirmar a ação. O estado será atualizado antes de uma nova tentativa.',
  bot_challenge_required: 'Conclua a verificação para continuar jogando.',
  bot_challenge_failed: 'A verificação não foi concluída. Tente novamente.',
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
  const nextHasPayouts = next.payouts && Object.keys(next.payouts).length > 0;
  const prevHasPayouts = previous.payouts && Object.keys(previous.payouts).length > 0;
  if (nextHasPayouts && !prevHasPayouts) {
    if (next.pot_results?.length) {
      for (const pot of next.pot_results) {
        if (pot.refund) {
          messages.push(...Object.entries(pot.payouts || {})
            .filter(([, amount]) => amount > 0)
            .map(([playerId, amount]) =>
              `${playerLabel(playerId)} recebeu ${amount.toLocaleString('pt-BR')} fichas devolvidas`));
        } else if (pot.winner_player_ids.length === 1) {
          const winner = pot.winner_player_ids[0];
          const amount = pot.payouts?.[winner] ?? pot.payout_amount;
          messages.push(`${playerLabel(winner)} ganhou ${amount.toLocaleString('pt-BR')} fichas`);
        } else if (pot.winner_player_ids.length > 1) {
          messages.push(`${pot.winner_player_ids.map(playerLabel).join(' e ')} dividiram um pote de ${pot.payout_amount.toLocaleString('pt-BR')} fichas`);
        }
      }
    } else {
      // Compatibility with protocol v1, which only published aggregate
      // credits and could not distinguish a win from a refund precisely.
      messages.push(...Object.entries(next.payouts || {})
        .filter(([playerId, amount]) => amount > 0 && next.winners?.includes(playerId))
        .map(([playerId, amount]) =>
          `${playerLabel(playerId)} recebeu ${amount.toLocaleString('pt-BR')} fichas`));
    }
  }
  return messages.join('. ');
}

// Plays at most one sound per snapshot transition (never on every broadcast):
// each condition compares against the previous snapshot exactly like
// describeSnapshot does). Priority: a new board card beats an all-in beats a
// bet beats a fold-to-one reveal, since at most one usually fires per frame
// anyway.
function playSoundForTransition(previous: TableSnapshot | null, next: TableSnapshot, viewerId?: string) {
  const scheduled: number[] = [];
  if (!previous) return scheduled;
  // Table is busy with a lot going on at once. The turn ring alone is easy
  // to miss, so this fires independently of (and can co-occur with) whatever
  // else this transition triggers below (a bet, a fold-to-one reveal, etc).
  if (viewerId && next.current_player_id === viewerId && previous.current_player_id !== viewerId) {
    playSound('your_turn');
  }
  if (next.board.length > previous.board.length) {
    const added = next.board.length - previous.board.length;
    // Flop deals 3 cards one at a time (Board/PlayingCard stagger reveal via
    // --deal-index; see .board-card .card-reveal-inner in globals.css): one
    // reveal sound per card, timed to match. Keep this in sync with that
    // animation-delay with a little gap (currently 360ms/index). Turn/river add a single card
    // with no stagger.
    for (let i = 0; i < added; i++) {
      if (i === 0) playSound('reveal');
      else scheduled.push(window.setTimeout(() => playSound('reveal'), i * 360));
    }
    return scheduled;
  }
  const previousSeats = new Map(previous.seats.map(seat => [seat.player_id, seat]));
  const wentAllIn = next.seats.some(seat => seat.state === 'all_in' && previousSeats.get(seat.player_id)?.state !== 'all_in');
  if (wentAllIn) {
    playSound('all_in');
    return scheduled;
  }
  const pot = previous.seats.reduce((n, seat) => n + seat.contributed, 0);
  const bettor = next.seats.find(seat => seat.contributed > (previousSeats.get(seat.player_id)?.contributed || 0));
  if (bettor) {
    const added = bettor.contributed - (previousSeats.get(bettor.player_id)?.contributed || 0);
    playSound(pot > 0 && added >= pot / 2 ? 'half_pot' : 'bet');
    return scheduled;
  }
  if (next.stage === 'complete' && previous.stage !== 'complete' && !next.won_without_showdown) playSound('reveal');
  return scheduled;
}

export function useTableRealtime(id: string, viewerId?: string, shareCode?: string, mockOptions?: {
  scenario?: MockScenario;
  delay?: number
}) {
  const [socketAuthToken, setSocketAuthToken] = useState(() => getAccessToken());
  useEffect(() => subscribeAccessToken(setSocketAuthToken), []);
  const activeTableIDRef = useRef(id);
  useEffect(() => {
    activeTableIDRef.current = id;
    return () => {
      if (activeTableIDRef.current === id) activeTableIDRef.current = '';
    };
  }, [id]);
  const pendingTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const pendingActionRef = useRef<{
    id: string;
    action: PokerAction;
    snapshotVersion: number;
    handId: string;
  } | null>(null);
  const latestVersionRef = useRef(-1);
  const latestHandIDRef = useRef('');
  const latestProtocolVersionRef = useRef(0);
  const previousSnapshot = useRef<TableSnapshot | null>(null);
  const awaitingReconnectSnapshotRef = useRef(false);
  const resetOnOpenRef = useRef(true);
  const sendRef = useRef<(value: object) => boolean>(() => false);
  // A mid-hand joiner is seated as pending_entry and stays that way forever
  // unless the client opts them in (PostBigBlindCmd). The product intent is
  // an automatic buy-in for the next hand's big blind, no manual click, so
  // fire it once as soon as the viewer's own seat shows pending_entry. Reset
  // when they leave that state so a *later* pending_entry spell (re-joining
  // after leaving) posts again instead of being silently skipped.
  const postedBigBlindRef = useRef(false);
  const postBigBlindActionRef = useRef<string | null>(null);
  const postBigBlindTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  // ready() and showCards() go straight through emit() with no server
  // round-trip tracking (unlike act(), which pendingActionRef already
  // guards): a double-click/double-tap sends the frame twice. A short
  // synchronous ref lock (not state, so two clicks in the same tick can't
  // both read a stale "not pending" value) blocks the repeat; the mirrored
  // state only drives the button's disabled/pending visual.
  const readyLockRef = useRef(false);
  const showCardsLockRef = useRef(false);
  const readyActionRef = useRef<string | null>(null);
  const showCardsActionRef = useRef<string | null>(null);
  const readyTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const showCardsTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const authRecoveryRef = useRef(false);
  const [readyPending, setReadyPending] = useState(false);
  const [showCardsPending, setShowCardsPending] = useState(false);
  const [snapshot, setSnapshot] = useState<TableSnapshot | null>(null);
  const [snapshotTableID, setSnapshotTableID] = useState('');
  // Captured once per snapshot (in this event handler, never during render) so
  // Seat can compute its countdown ring's remaining time as a pure function of
  // props (deadlineMs - snapshotAt) instead of calling Date.now() itself.
  const [snapshotAt, setSnapshotAt] = useState(0);
  const [unlock, setUnlock] = useState<{ key: string; stars: number } | null>(null);
  const [chat, setChat] = useState<{ id: string; player: string; message: string; timestamp?: number }[]>([]);
  const [reactions, setReactions] = useState<TableReactionEvent[]>([]);
  const reactionTimersRef = useRef<Map<string, number>>(new Map());
  const soundTimersRef = useRef<Set<number>>(new Set());
  const [pendingAction, setPendingAction] = useState<PokerAction | null>(null);
  const [lastActionError, setLastActionError] = useState<ActionError | null>(null);
  const [announcement, setAnnouncement] = useState('');
  const [botChallengeRequired, setBotChallengeRequired] = useState(false);
  const [removed, setRemoved] = useState<{ code?: string } | null>(null);
  const [mockStatus, setMockStatus] = useState<WSStatus>('connecting');
  const [mockReconnectAttempt, setMockReconnectAttempt] = useState(0);
  const mockService = useRef<MockTableService | null>(null);

  const showReaction = useCallback((reaction: TableReactionEvent, expiresAt = Date.now() + 2400) => {
    if (!reactionTimersRef.current.has(reaction.id)) {
      setReactions(value => [...value.filter(item => item.id !== reaction.id).slice(-7), reaction]);
    }
    const existing = reactionTimersRef.current.get(reaction.id);
    if (existing) window.clearTimeout(existing);
    const remaining = Math.max(0, expiresAt - Date.now());
    const timer = window.setTimeout(() => {
      setReactions(value => value.filter(item => item.id !== reaction.id));
      reactionTimersRef.current.delete(reaction.id);
    }, remaining);
    reactionTimersRef.current.set(reaction.id, timer);
  }, []);

  const clearPending = useCallback((expectedId?: string) => {
    if (expectedId && pendingActionRef.current?.id !== expectedId) return;
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
    pendingTimer.current = undefined;
    pendingActionRef.current = null;
    setPendingAction(null);
  }, []);

  const failPending = useCallback((code: string, expectedId?: string) => {
    clearPending(expectedId);
    setLastActionError(actionError(code));
  }, [clearPending]);

  const finishAuxiliaryCommand = useCallback((actionId: string, failedCode?: string) => {
    if (readyActionRef.current === actionId) {
      if (readyTimerRef.current) clearTimeout(readyTimerRef.current);
      readyActionRef.current = null;
      readyLockRef.current = false;
      setReadyPending(false);
    }
    if (showCardsActionRef.current === actionId) {
      if (showCardsTimerRef.current) clearTimeout(showCardsTimerRef.current);
      showCardsActionRef.current = null;
      showCardsLockRef.current = false;
      setShowCardsPending(false);
    }
    if (postBigBlindActionRef.current === actionId) {
      if (postBigBlindTimerRef.current) clearTimeout(postBigBlindTimerRef.current);
      postBigBlindActionRef.current = null;
      if (failedCode) postedBigBlindRef.current = false;
    }
    if (failedCode) setLastActionError(actionError(failedCode));
  }, []);

  const recoverSession = useCallback(() => {
    if (USE_MOCK || authRecoveryRef.current) return;
    authRecoveryRef.current = true;
    void doRefresh().then(result => {
      if (result) {
        setAccessToken(result.accessToken);
        setUsername(result.username);
      } else {
        setAccessToken(null);
        setUsername(null);
        setPlayerId(null);
      }
    }).catch(() => {
      setAccessToken(null);
      setUsername(null);
      setPlayerId(null);
    }).finally(() => {
      authRecoveryRef.current = false;
    });
  }, []);

  const receive = useCallback((message: ServerMessage) => {
    if (message.type === 'state' && message.snapshot) {
      const legacyUnversioned = !message.snapshot.snapshot_version;
      const version = message.snapshot.snapshot_version ?? 0;
      if (latestVersionRef.current >= 0 && version < latestVersionRef.current) return;
      latestVersionRef.current = version;
      latestHandIDRef.current = message.snapshot.hand_id ?? '';
      latestProtocolVersionRef.current = message.snapshot.protocol_version ?? 0;
      const liveMessage = describeSnapshot(previousSnapshot.current, message.snapshot, viewerId);
      for (const timer of soundTimersRef.current) window.clearTimeout(timer);
      soundTimersRef.current.clear();
      for (const timer of playSoundForTransition(previousSnapshot.current, message.snapshot, viewerId)) {
        soundTimersRef.current.add(timer);
      }
      previousSnapshot.current = message.snapshot;
      if (liveMessage) setAnnouncement(liveMessage);
      setSnapshot(message.snapshot);
      setSnapshotTableID(activeTableIDRef.current);
      setSnapshotAt(Date.now());
      if ((message.snapshot.protocol_version ?? 0) >= 6) {
        setChat((message.snapshot.chat_messages ?? []).map(item => ({
          id: item.id, player: item.player_id, message: item.message, timestamp: item.timestamp
        })));
        const liveReactionIDs = new Set((message.snapshot.reactions ?? []).map(item => item.id));
        for (const [reactionID, timer] of reactionTimersRef.current) {
          if (liveReactionIDs.has(reactionID)) continue;
          window.clearTimeout(timer);
          reactionTimersRef.current.delete(reactionID);
        }
        setReactions(value => value.filter(item => liveReactionIDs.has(item.id)));
        for (const item of message.snapshot.reactions ?? []) {
          if (!isTableReaction(item.reaction_id) || item.expires_at <= Date.now()) continue;
          showReaction({
            id: item.id,
            playerId: item.player_id,
            reactionId: item.reaction_id,
            targetPlayerId: item.target_player_id || undefined
          }, item.expires_at);
        }
      }
      // ACK is authoritative. This version check is the recovery path for a
      // lost ACK: once a newer state arrives, the old decision cannot still
      // be pending against the snapshot it was sent from.
      if (pendingActionRef.current && version > pendingActionRef.current.snapshotVersion) {
        clearPending(pendingActionRef.current.id);
      }
      // A sync_state response is serialized after the action frame on the
      // same socket. Even at the same version it is authoritative proof that
      // the timed-out action was not committed.
      if (message.action_id && pendingActionRef.current?.id === message.action_id) {
        clearPending(message.action_id);
      }
      // A reconnect's initial snapshot is built with a forced store read.
      if (awaitingReconnectSnapshotRef.current) {
        awaitingReconnectSnapshotRef.current = false;
        clearPending();
      }
      // Compatibility while API instances are rolling from the unversioned
      // protocol: those servers have no ACK frames, so their next full state
      // remains the only available confirmation signal.
      if (legacyUnversioned) {
        clearPending();
        for (const id of [readyActionRef.current, showCardsActionRef.current, postBigBlindActionRef.current]) {
          if (id) finishAuxiliaryCommand(id);
        }
      }
      const ownSeat = message.snapshot.seats.find(seat => seat.player_id === viewerId);
      if (ownSeat?.state === 'pending_entry') {
        if (!postedBigBlindRef.current && !postBigBlindActionRef.current) {
          postedBigBlindRef.current = true;
          const actionId = crypto.randomUUID();
          postBigBlindActionRef.current = actionId;
          const retry = (remaining: number) => {
            if (postBigBlindActionRef.current !== actionId) return;
            if (!sendRef.current({type: 'post_big_blind', action_id: actionId})) {
              postBigBlindActionRef.current = null;
              postedBigBlindRef.current = false;
              return;
            }
            postBigBlindTimerRef.current = setTimeout(() => {
              if (remaining > 1) retry(remaining - 1);
              else finishAuxiliaryCommand(actionId, 'action_timeout');
            }, 2000);
          };
          retry(3);
        }
      } else {
        postedBigBlindRef.current = false;
      }
    }
    if (message.type === 'equity' && message.player_id && message.equity !== undefined &&
      message.snapshot_version === latestVersionRef.current) {
      setSnapshot(value => value ? {
        ...value,
        seats: value.seats.map(seat => seat.player_id === message.player_id ? {...seat, equity: message.equity} : seat)
      } : value);
    }
    if (message.type === 'action_ack' && message.action_id) {
      const revealedCard = showCardsActionRef.current === message.action_id;
      clearPending(message.action_id);
      finishAuxiliaryCommand(message.action_id);
      if (revealedCard) {
        // Protocol v1 only supports reveal-both and has no public reveal
        // mask. Mirror that acknowledged result locally until the API-first
        // rollout reaches protocol v2.
        if (latestProtocolVersionRef.current < 2) {
          setSnapshot(value => value ? {
            ...value,
            seats: value.seats.map(seat => seat.player_id === viewerId
              ? {...seat, hole_cards_revealed: [true, true]}
              : seat)
          } : value);
        }
        playSound('showing_card');
      }
    }
    if (message.type === 'error') {
      const code = message.code || 'unknown';
      if (code === 'unauthorized') recoverSession();
      if ((code === 'stale_state' || code === 'rate_limited') && message.action_id &&
        pendingActionRef.current?.id === message.action_id) {
        setLastActionError(actionError(code));
        const actionId = message.action_id;
        if (pendingTimer.current) clearTimeout(pendingTimer.current);
        const jitterMs = Math.floor(Math.random() * 400) + (code === 'rate_limited' ? 800 : 50);
        pendingTimer.current = setTimeout(() => {
          if (pendingActionRef.current?.id === actionId) {
            sendRef.current({type: 'sync_state', action_id: actionId});
          }
        }, jitterMs);
      } else if (message.action_id && pendingActionRef.current?.id === message.action_id) {
        failPending(code, message.action_id);
      }
      else if (message.action_id) finishAuxiliaryCommand(message.action_id, code);
      else setLastActionError(actionError(code));
    }
    if (message.type === 'connected') {
      awaitingReconnectSnapshotRef.current = true;
    }
    if (message.type === 'bot_challenge') setBotChallengeRequired(true);
    if (message.type === 'bot_challenge_passed') {
      setBotChallengeRequired(false);
      setLastActionError(null);
    }
    if (message.type === 'removed') setRemoved({code: message.code});
    if (message.type === 'achievement_unlocked' && message.key) setUnlock({
      key: message.key,
      stars: message.stars || 1
    });
    if (message.type === 'chat' && message.message) {
      const chatMessage = message.message;
      const id = message.action_id || `${Date.now()}-${message.player_id || '?'}-${chatMessage}`;
      setChat(value => value.some(item => item.id === id) ? value :
        [...value.slice(-39), {id, player: message.player_id || '?', message: chatMessage, timestamp: Date.now()}]);
    }
    if (message.type === 'reaction' && message.player_id && message.reaction_id &&
      isTableReaction(message.reaction_id)) {
      const reaction: TableReactionEvent = {
        id: message.action_id || `${Date.now()}-${message.player_id}-${message.reaction_id}-${Math.random()}`,
        playerId: message.player_id,
        reactionId: message.reaction_id,
        targetPlayerId: message.target_player_id || undefined
      };
      showReaction(reaction);
    }
  }, [clearPending, failPending, finishAuxiliaryCommand, recoverSession, showReaction, viewerId]);
  const receiveForTable = useCallback((message: ServerMessage) => {
    if (activeTableIDRef.current === id) receive(message);
  }, [id, receive]);

  const origin = (process.env.NEXT_PUBLIC_API_URL || (typeof window !== 'undefined' ? window.location.origin : '')).replace(/^http/, 'ws');
  const wsUrl = id ? `${origin}/v1.0/tables/${encodeURIComponent(id)}/ws` : null;
  const handleOpen = useCallback(() => {
    if (resetOnOpenRef.current) {
      resetOnOpenRef.current = false;
      setSnapshot(null);
      setSnapshotTableID('');
      setSnapshotAt(0);
      setPendingAction(null);
      setReadyPending(false);
      setShowCardsPending(false);
      setLastActionError(null);
      setAnnouncement('');
      setRemoved(null);
      setChat([]);
      setReactions([]);
    }
    sendRef.current({type: 'ping'});
  }, []);
  const {status: wsStatus, attempt: wsReconnectAttempt, send: wsSend, reconnect: wsRetryNow} = useWebSocket({
    url: wsUrl,
    binaryType: 'arraybuffer',
    encode: encodeClientMessage,
    decode: decodeServerMessage,
    onMessage: data => receiveForTable(data as ServerMessage),
    enabled: Boolean(wsUrl) && !USE_MOCK,
    authToken: socketAuthToken || undefined,
    shareCode,
    onOpen: handleOpen
  });
  const mockScenario = mockOptions?.scenario || 'flop';
  const mockDelay = Math.min(15000, Math.max(0, mockOptions?.delay ?? 650));
  useEffect(() => {
    if (!USE_MOCK || !id) return () => {

    };
    // A scenario change creates a brand-new in-memory server whose snapshot
    // versions start at one again. Reset every client-side value tied to the
    // previous service before connecting it; otherwise the monotonic-version
    // guard rejects the new scenario's snapshots as stale and leaves the
    // completed hand frozen on screen.
    latestVersionRef.current = -1;
    previousSnapshot.current = null;
    let service: MockTableService | null = null;
    let cancelled = false;
    void import('@/dev/mockRuntime').then(({MockTableService}) => {
      if (cancelled) return;
      service = new MockTableService(mockScenario, mockDelay, {
        onMessage: receiveForTable,
        onStatus: (next, attempt) => {
          if (next === 'connecting') {
            setSnapshot(null);
            setSnapshotTableID('');
            setSnapshotAt(0);
            setAnnouncement('');
            setLastActionError(null);
            setRemoved(null);
            clearPending();
            for (const actionId of [readyActionRef.current, showCardsActionRef.current, postBigBlindActionRef.current]) {
              if (actionId) finishAuxiliaryCommand(actionId);
            }
            postedBigBlindRef.current = false;
          }
          setMockStatus(next);
          setMockReconnectAttempt(attempt);
        }
      });
      mockService.current = service;
      service.connect();
    });
    return () => {
      cancelled = true;
      service?.close();
      if (mockService.current === service) mockService.current = null;
    };
  }, [clearPending, finishAuxiliaryCommand, id, mockDelay, mockScenario, receiveForTable]);

  const send = useCallback((value: object) => USE_MOCK ? Boolean(mockService.current?.send(value as Record<string, unknown>)) : wsSend(value), [wsSend]);
  const retryNow = useCallback(() => USE_MOCK ? mockService.current?.reconnect() : wsRetryNow(), [wsRetryNow]);
  const status = USE_MOCK ? mockStatus : wsStatus;
  const reconnectAttempt = USE_MOCK ? mockReconnectAttempt : wsReconnectAttempt;
  useEffect(() => {
    sendRef.current = send;
  }, [send]);

  // Query-param navigation reuses this hook instance. Every realtime ref is
  // table-scoped; carrying version 100 from table A into table B version 10
  // would otherwise reject B forever as "stale".
  useEffect(() => {
    latestVersionRef.current = -1;
    latestHandIDRef.current = '';
    latestProtocolVersionRef.current = 0;
    previousSnapshot.current = null;
    awaitingReconnectSnapshotRef.current = false;
    postedBigBlindRef.current = false;
    postBigBlindActionRef.current = null;
    readyActionRef.current = null;
    showCardsActionRef.current = null;
    readyLockRef.current = false;
    showCardsLockRef.current = false;
    for (const timer of [pendingTimer.current, readyTimerRef.current, showCardsTimerRef.current,
      postBigBlindTimerRef.current]) {
      if (timer) clearTimeout(timer);
    }
    pendingTimer.current = undefined;
    readyTimerRef.current = undefined;
    showCardsTimerRef.current = undefined;
    postBigBlindTimerRef.current = undefined;
    pendingActionRef.current = null;
    for (const timer of reactionTimersRef.current.values()) window.clearTimeout(timer);
    reactionTimersRef.current.clear();
    for (const timer of soundTimersRef.current) window.clearTimeout(timer);
    soundTimersRef.current.clear();
    resetOnOpenRef.current = true;
  }, [id]);

  useEffect(() => () => {
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
    if (readyTimerRef.current) clearTimeout(readyTimerRef.current);
    if (showCardsTimerRef.current) clearTimeout(showCardsTimerRef.current);
    if (postBigBlindTimerRef.current) clearTimeout(postBigBlindTimerRef.current);
    for (const timer of reactionTimersRef.current.values()) window.clearTimeout(timer);
    reactionTimersRef.current.clear();
    for (const timer of soundTimersRef.current) window.clearTimeout(timer);
    soundTimersRef.current.clear();
  }, []);

  useEffect(() => {
    if (USE_MOCK || !id) return () => {
    };
    const interval = setInterval(() => {
      void doRefresh().then(result => {
        if (result) {
          setAccessToken(result.accessToken);
          setUsername(result.username);
        } else {
          setAccessToken(null);
          setUsername(null);
          setPlayerId(null);
        }
      }).catch(() => {
        // A periodic refresh can fail because the device is temporarily
        // offline. Keep the current session until the server explicitly
        // rejects it; that path invokes recoverSession and clears invalid
        // credentials when refresh is no longer possible.
      });
    }, TOKEN_REFRESH_INTERVAL_MS);
    return () => clearInterval(interval);
  }, [id]);

  // A backgrounded tab (screen lock, app switch) can have its WS silently
  // killed by the OS without a clean close event. The client-side heartbeat
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

  const submitBotChallenge = useCallback((token: string) =>
    emit({type: 'bot_challenge', turnstile_token: token, action_id: crypto.randomUUID()}), [emit]);

  const act = useCallback((action: PokerAction, amount = 0) => {
    if (pendingActionRef.current) return false;
    setLastActionError(null);
    const snapshotVersion = latestVersionRef.current;
    const handId = latestHandIDRef.current;
    if (snapshotVersion <= 0 || !handId) {
      setLastActionError(actionError('missing_precondition'));
      return false;
    }
    const actionId = crypto.randomUUID();
    pendingActionRef.current = {id: actionId, action, snapshotVersion, handId};
    setPendingAction(action);
    if (!emit({
      type: 'act',
      action,
      amount,
      action_id: actionId,
      expected_snapshot_version: snapshotVersion,
      expected_hand_id: handId
    })) {
      clearPending(actionId);
      return false;
    }
    pendingTimer.current = setTimeout(() => {
      if (pendingActionRef.current?.id !== actionId) return;
      setLastActionError(actionError('action_timeout'));
      if (!send({type: 'sync_state', action_id: actionId})) {
        setLastActionError(actionError('connection_lost'));
        awaitingReconnectSnapshotRef.current = true;
        retryNow();
      }
    }, ACTION_TIMEOUT_MS);
    return true;
  }, [clearPending, emit, retryNow, send]);

  return {
    status,
    snapshot: snapshotTableID === id ? snapshot : null,
    snapshotAt,
    unlock,
    chat,
    reactions,
    pendingAction,
    actionError: lastActionError,
    reconnectAttempt,
    announcement,
    botChallengeRequired,
    removed,
    clearActionError: () => setLastActionError(null),
    retryNow,
    readyPending,
    showCardsPending,
    ready: (ready = true) => {
      if (readyLockRef.current) return false;
      const actionId = crypto.randomUUID();
      readyLockRef.current = true;
      readyActionRef.current = actionId;
      setReadyPending(true);
      if (!emit({type: 'ready', ready, action_id: actionId})) {
        finishAuxiliaryCommand(actionId);
        return false;
      }
      readyTimerRef.current = setTimeout(() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS);
      return true;
    },
    act,
    showCards: (cardIndex?: number) => {
      if (showCardsLockRef.current) return false;
      const actionId = crypto.randomUUID();
      showCardsLockRef.current = true;
      showCardsActionRef.current = actionId;
      setShowCardsPending(true);
      const ok = emit({type: 'show_cards', action_id: actionId, card_index: cardIndex});
      if (!ok) {
        finishAuxiliaryCommand(actionId);
        return false;
      }
      showCardsTimerRef.current = setTimeout(() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS);
      return ok;
    },
    keepSeat: () => emit({type: 'keep_seat', action_id: crypto.randomUUID()}),
    sendChat: (message: string) => emit({type: 'chat', message, action_id: crypto.randomUUID()}),
    sendReaction: (reactionId: TableReactionID, targetPlayerId?: string) =>
      emit({type: 'reaction', reaction_id: reactionId, target_player_id: targetPlayerId || '',
        action_id: crypto.randomUUID()}),
    preselectAction: (selection: ActionPreselection | null, amount = 0) => emit({
      type: 'preselect_action', action: selection || '', action_id: crypto.randomUUID(),
      amount,
      expected_snapshot_version: latestVersionRef.current,
      expected_hand_id: latestHandIDRef.current
    }),
    submitBotChallenge
  };
}
