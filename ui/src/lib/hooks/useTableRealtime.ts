'use client';
import {useCallback, useEffect, useRef, useState} from 'react';
import {getAccessToken, subscribeAccessToken} from '@/lib/api/client';
import {wsOrigin} from '@/lib/ws/origin';
import {recoverSession} from '@/lib/auth/session';
import {cardLabel} from '@/lib/cards';
import {MAX_RECONNECT_ATTEMPTS, useWebSocket, type WSStatus} from '@aoctech/ws-client';
import {checkApiLiveness} from '@/lib/network/liveness';
import {useApiLiveness} from '@/lib/network/NetworkProvider';
import type {MockTableService} from '@/dev/mockRuntime';
import {type MockScenario, USE_MOCK} from '@/lib/mockConfig';
import type {ActionPreselection, PokerAction, ServerMessage, TableSnapshot} from '@/lib/api/table';
import {playerName} from '@/lib/utils';
import {playSound} from '@/lib/sound';
import {decodeServerMessage, encodeClientMessage} from "@/lib/ws/utils";
import {isTableReaction, type TableReactionEvent, type TableReactionID} from '@/lib/reactions';
import {CHAT_HISTORY_LIMIT, CHAT_MESSAGE_MAX_LENGTH} from '@/lib/chat';

export type ConnectionStatus = WSStatus
export type ActionError = { code: string; message: string }

const ACTION_TIMEOUT_MS = 8000;
// A stale_state rejection means the actor's own snapshot/hand precondition
// didn't match the server's — the same action resubmitted against the fresh
// version the resync just fetched is legal again (see actor.go's
// validateActionPrecondition). Cap the auto-resubmits so a genuinely illegal
// action (or a table stuck racing another player) fails visibly instead of
// looping forever.
const MAX_ACTION_RETRIES = 3;
// Rejections that mean "your view of the table is not the server's view".
// invalid_action belongs here even though it is also the code for a genuinely
// illegal move: a resync costs one snapshot, while not resyncing leaves a
// player who hit a server-side desync stuck until they reload the page.
const RESYNC_ERROR_CODES = new Set(['stale_state', 'rate_limited', 'invalid_action', 'unavailable']);
const TERMINAL_ERROR_CODES = new Set(['forbidden', 'not_found']);
const RESYNC_TIMEOUT_MS = 2500;
// First resubmit delay for an auxiliary command rejected against stale state,
// doubled per retry. Deliberately longer than the resync backoff scheduled for
// the same action_id (<=450ms on a first rejection) so the resubmit is judged
// against the state that resync pulled, not the one that just rejected it.
const AUX_RETRY_BASE_MS = 700;

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
  message_too_long: `A mensagem ultrapassa o limite de ${CHAT_MESSAGE_MAX_LENGTH} caracteres.`,
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

/** `suppressedPlayerIds` are the players the viewer muted or blocked. Their
 * chat and reactions are filtered *before* entering state, so a suppressed
 * message never creates a bubble, an animation or a live-region announcement —
 * while their seat, bets and poker actions stay completely untouched. Pass a
 * memoized set: it is compared by identity. */
export function useTableRealtime(id: string, viewerId?: string, shareCode?: string, mockOptions?: {
  scenario?: MockScenario;
  delay?: number
}, suppressedPlayerIds?: ReadonlySet<string>) {
  const [socketAuthToken, setSocketAuthToken] = useState(() => getAccessToken());
  const apiLiveness = useApiLiveness();
  useEffect(() => subscribeAccessToken(setSocketAuthToken), []);
  // Read through a ref: receive() must see the current suppression set without
  // being rebuilt (and re-subscribing the socket) every time it changes.
  const suppressedRef = useRef(suppressedPlayerIds);
  useEffect(() => {
    suppressedRef.current = suppressedPlayerIds;
  }, [suppressedPlayerIds]);
  const isSuppressed = useCallback((playerId?: string) =>
    Boolean(playerId && suppressedRef.current?.has(playerId)), []);
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
    amount: number;
    snapshotVersion: number;
    handId: string;
    // Auto-retry bookkeeping for stale_state: retries counts resubmits already
    // used, awaitingRetry marks "the next authoritative snapshot should
    // resubmit this action" rather than clear it as abandoned.
    retries: number;
    awaitingRetry: boolean;
  } | null>(null);
  const latestVersionRef = useRef(-1);
  const latestHandIDRef = useRef('');
  const latestProtocolVersionRef = useRef(0);
  const previousSnapshot = useRef<TableSnapshot | null>(null);
  const awaitingReconnectSnapshotRef = useRef(false);
  const resetOnOpenRef = useRef(true);
  const sendRef = useRef<(value: object) => boolean>(() => false);
  const retryNowRef = useRef<() => void>(() => {
  });
  // Armed whenever a rejection makes us ask for the authoritative snapshot.
  // If that snapshot never lands, the socket itself is the broken part (the
  // server can be answering frames while serving a table it cannot load), so
  // the only recovery is a fresh connection — which is exactly what a manual
  // F5 used to do for the player.
  // Keyed by action_id (or '__global__' for an action-less resyncable
  // error) rather than a single shared ref: two different in-flight
  // commands (e.g. an act() and a requestRabbitHunt()) can each get their
  // own resyncable error within the same jitter window, and a shared ref
  // let the second overwrite/disarm the first's watchdog, stranding it
  // pending forever with no recovery (see armResyncWatchdog/clearResyncFor).
  const resyncWatchdogs = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const resyncTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
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
  const requestRabbitHuntLockRef = useRef(false);
	const requestWinnerCardsLockRef = useRef(false);
  const requestExitLockRef = useRef(false);
  const readyActionRef = useRef<string | null>(null);
  const showCardsActionRef = useRef<string | null>(null);
  const requestRabbitHuntActionRef = useRef<string | null>(null);
	const requestWinnerCardsActionRef = useRef<string | null>(null);
  const requestExitActionRef = useRef<string | null>(null);
  const readyTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const showCardsTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const requestRabbitHuntTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
	const requestWinnerCardsTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const requestExitTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  // Auxiliary commands (show_cards, request_rabbit_hunt, request_winner_cards,
  // ...) carry no expected_snapshot_version, so the server can only answer
  // them with a flat rejection — there is no stale_state path for them the way
  // there is for act(). Keeping the exact frame here is what lets a
  // resync-class rejection be resubmitted instead of surfaced, keyed by the
  // action_id the reply correlates on. Entries live only while the command is
  // in flight; finishAuxiliaryCommand drops them.
  const auxFramesRef = useRef<Map<string, { frame: object; retries: number; timer?: ReturnType<typeof setTimeout> }>>(new Map());
  const [readyPending, setReadyPending] = useState(false);
  const [showCardsPending, setShowCardsPending] = useState(false);
  const [requestRabbitHuntPending, setRequestRabbitHuntPending] = useState(false);
  const [requestRabbitHuntFailCount, setRequestRabbitHuntFailCount] = useState(0);
	const [requestWinnerCardsPending, setRequestWinnerCardsPending] = useState(false);
  const [requestExitPending, setRequestExitPending] = useState(false);
  const [snapshot, setSnapshot] = useState<TableSnapshot | null>(null);
  const [snapshotTableID, setSnapshotTableID] = useState('');
  // Captured once per snapshot (in this event handler, never during render) so
  // Seat can compute its countdown ring's remaining time as a pure function of
  // props (deadlineMs - snapshotAt) instead of calling Date.now() itself.
  const [snapshotAt, setSnapshotAt] = useState(0);
  const [unlock, setUnlock] = useState<{ key: string; stars: number } | null>(null);
  const [chat, setChat] = useState<{ id: string; player: string; message: string; timestamp?: number }[]>([]);
  // The seat speech-bubble is only for messages actually delivered while this
  // hook is mounted, never for the chat history a fresh snapshot hydrates on
  // connect/reconnect (that would burst every seat with a bubble on join).
  // null means "no snapshot has hydrated chat yet"; the first hydration seeds
  // this set silently, and only ids that show up afterward earn a bubble.
  const seenChatIdsRef = useRef<Set<string> | null>(null);
  const [chatBubbles, setChatBubbles] = useState<Record<string, { id: string; message: string }>>({});
  const noteFreshChatArrivals = useCallback((items: { id: string; player: string; message: string }[]) => {
    const seen = seenChatIdsRef.current;
    if (seen === null) {
      seenChatIdsRef.current = new Set(items.map(item => item.id));
      return;
    }
    const fresh = items.filter(item => !seen.has(item.id));
    if (!fresh.length) return;
    for (const item of fresh) seen.add(item.id);
    setChatBubbles(value => {
      const next = {...value};
      for (const item of fresh) next[item.player] = {id: item.id, message: item.message};
      return next;
    });
  }, []);
  const [reactions, setReactions] = useState<TableReactionEvent[]>([]);
  const reactionTimersRef = useRef<Map<string, number>>(new Map());
  const soundTimersRef = useRef<Set<number>>(new Set());
  const [pendingAction, setPendingAction] = useState<PokerAction | null>(null);
  const [lastActionError, setLastActionError] = useState<ActionError | null>(null);
  const [announcement, setAnnouncement] = useState('');
  const [botChallengeRequired, setBotChallengeRequired] = useState(false);
  const [removed, setRemoved] = useState<{ code?: string; amount?: number } | null>(null);
  const [terminalFailure, setTerminalFailure] = useState<{ tableID: string; code: string } | null>(null);
  const terminalError = terminalFailure?.tableID === id ? terminalFailure.code : null;
  const [mockStatus, setMockStatus] = useState<WSStatus>('connecting');
  const [mockReconnectAttempt, setMockReconnectAttempt] = useState(0);
  const mockService = useRef<MockTableService | null>(null);
  
  const showReaction = useCallback((reaction: TableReactionEvent, expiresAt = Date.now() + 3400) => {
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
    const aux = auxFramesRef.current.get(actionId);
    if (aux) {
      if (aux.timer) clearTimeout(aux.timer);
      auxFramesRef.current.delete(actionId);
    }
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
    if (requestRabbitHuntActionRef.current === actionId) {
      if (requestRabbitHuntTimerRef.current) clearTimeout(requestRabbitHuntTimerRef.current);
      requestRabbitHuntActionRef.current = null;
      requestRabbitHuntLockRef.current = false;
      setRequestRabbitHuntPending(false);
      if (failedCode) setRequestRabbitHuntFailCount(value => value + 1);
    }
		if (requestWinnerCardsActionRef.current === actionId) {
			if (requestWinnerCardsTimerRef.current) clearTimeout(requestWinnerCardsTimerRef.current);
			requestWinnerCardsActionRef.current = null;
			requestWinnerCardsLockRef.current = false;
			setRequestWinnerCardsPending(false);
		}
    if (requestExitActionRef.current === actionId) {
      if (requestExitTimerRef.current) clearTimeout(requestExitTimerRef.current);
      requestExitActionRef.current = null;
      requestExitLockRef.current = false;
      setRequestExitPending(false);
    }
    if (postBigBlindActionRef.current === actionId) {
      if (postBigBlindTimerRef.current) clearTimeout(postBigBlindTimerRef.current);
      postBigBlindActionRef.current = null;
      if (failedCode) postedBigBlindRef.current = false;
    }
    if (failedCode) setLastActionError(actionError(failedCode));
  }, []);

  // A resync-class rejection of an auxiliary command means the server judged
  // it against a state this client does not have — the exact situation the
  // resync scheduled alongside it is about to fix. The lease-holding table
  // actor can serve a cache another fleet instance has already moved past, and
  // these commands are rejected by an engine precondition ("hand is not
  // complete yet") *before* any commit, so nothing on the server forces that
  // reload either: without a resubmit, a rabbit hunt or a card reveal fails on
  // every single attempt while the client is looking at the very snapshot the
  // fleet broadcast. Resends the identical frame under the SAME action_id —
  // the rejection happened before commit, so no idempotency guard was written
  // for it — capped at the same MAX_ACTION_RETRIES act() uses, and always
  // later than the resync's own backoff so the resubmit lands on fresh state.
  // The original ACTION_TIMEOUT_MS timer stays armed as the backstop.
  const retryAuxiliaryCommand = useCallback((actionId: string, code: string) => {
    if (!RESYNC_ERROR_CODES.has(code)) return false;
    const aux = auxFramesRef.current.get(actionId);
    if (!aux || aux.retries >= MAX_ACTION_RETRIES) return false;
    aux.retries += 1;
    if (aux.timer) clearTimeout(aux.timer);
    aux.timer = setTimeout(() => {
      aux.timer = undefined;
      if (!auxFramesRef.current.has(actionId)) return;
      if (!sendRef.current(aux.frame)) finishAuxiliaryCommand(actionId, 'not_connected');
    }, AUX_RETRY_BASE_MS * 2 ** (aux.retries - 1) + Math.floor(Math.random() * 200));
    return true;
  }, [finishAuxiliaryCommand]);

  // Sends (or resends) the 'act' frame and (re)arms its own timeout. Used both
  // for the first submit and for a stale_state auto-retry, so it goes through
  // sendRef/retryNowRef (not send/emit/retryNow) to stay callable from
  // receive(), which is declared before those exist.
  const sendActFrame = useCallback((actionId: string, action: PokerAction, amount: number,
    snapshotVersion: number, handId: string) => {
    if (pendingTimer.current) clearTimeout(pendingTimer.current);
    if (!sendRef.current({
      type: 'act', action, amount, action_id: actionId,
      expected_snapshot_version: snapshotVersion, expected_hand_id: handId
    })) {
      setLastActionError(actionError('not_connected'));
      clearPending(actionId);
      return false;
    }
    pendingTimer.current = setTimeout(() => {
      if (pendingActionRef.current?.id !== actionId) return;
      setLastActionError(actionError('action_timeout'));
      if (!sendRef.current({type: 'sync_state', action_id: actionId})) {
        setLastActionError(actionError('connection_lost'));
        awaitingReconnectSnapshotRef.current = true;
        retryNowRef.current();
      }
    }, ACTION_TIMEOUT_MS);
    return true;
  }, [clearPending]);

  const armResyncWatchdog = useCallback((key: string) => {
    const existing = resyncWatchdogs.current.get(key);
    if (existing) clearTimeout(existing);
    resyncWatchdogs.current.set(key, setTimeout(() => {
      resyncWatchdogs.current.delete(key);
      awaitingReconnectSnapshotRef.current = true;
      retryNowRef.current();
    }, RESYNC_TIMEOUT_MS));
  }, []);

  const receive = useCallback((message: ServerMessage) => {
    if (message.type === 'state' && message.snapshot) {
      // Only disarm the watchdog(s) this message actually resolves: a
      // correlated reply to the resync it belongs to (matched by
      // action_id), or — once a reconnect is underway — every watchdog at
      // once, since a fresh connection makes them all moot. An unrelated
      // broadcast (no matching action_id) must not disarm anything; that
      // was the bug (see the Map comment above).
      if (awaitingReconnectSnapshotRef.current) {
        for (const timer of resyncWatchdogs.current.values()) clearTimeout(timer);
        resyncWatchdogs.current.clear();
      } else if (message.action_id && resyncWatchdogs.current.has(message.action_id)) {
        clearTimeout(resyncWatchdogs.current.get(message.action_id)!);
        resyncWatchdogs.current.delete(message.action_id);
      }
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
        const nextChat = (message.snapshot.chat_messages ?? [])
          .filter(item => !isSuppressed(item.player_id))
          .map(item => ({
            id: item.id, player: item.player_id, message: item.message, timestamp: item.timestamp
          })).slice(-CHAT_HISTORY_LIMIT);
        noteFreshChatArrivals(nextChat);
        setChat(nextChat);
        const liveReactionIDs = new Set((message.snapshot.reactions ?? []).map(item => item.id));
        for (const [reactionID, timer] of reactionTimersRef.current) {
          if (liveReactionIDs.has(reactionID)) continue;
          window.clearTimeout(timer);
          reactionTimersRef.current.delete(reactionID);
        }
        setReactions(value => value.filter(item => liveReactionIDs.has(item.id)));
        for (const item of message.snapshot.reactions ?? []) {
          if (!isTableReaction(item.reaction_id) || item.expires_at <= Date.now()) continue;
          if (isSuppressed(item.player_id)) continue;
          showReaction({
            id: item.id,
            playerId: item.player_id,
            reactionId: item.reaction_id,
            targetPlayerId: item.target_player_id || undefined
          }, item.expires_at);
        }
      }
      // ACK is authoritative — a plain unrelated broadcast (no action_id)
      // must NOT clear the pending action, even at a newer version: the
      // direct reply to our own request (ack, or the stale_state error that
      // arms a retry below) can legitimately arrive on the socket *after*
      // that broadcast, and clearing here first meant the retry-arm block
      // below found pendingActionRef already null and silently dropped the
      // action instead of resubmitting it (seen live: a "check" rejected as
      // stale_state right after an unrelated broadcast bumped the version).
      // A genuinely lost ACK is still covered by sendActFrame's own
      // ACTION_TIMEOUT_MS backstop.
      // A sync_state response is serialized after the action frame on the
      // same socket. Even at the same version it is authoritative proof that
      // the timed-out action was not committed. If it was armed for a
      // stale_state retry, resubmit against this fresh version/hand instead
      // of giving up — same action_id, since the rejected attempt never
      // reached the idempotency guard (validateActionPrecondition rejects
      // before commit), so resubmitting it is safe.
      if (message.action_id && pendingActionRef.current?.id === message.action_id) {
        const pending = pendingActionRef.current;
        if (pending.awaitingRetry && pending.retries < MAX_ACTION_RETRIES) {
          pending.retries += 1;
          pending.awaitingRetry = false;
          pending.snapshotVersion = version;
          pending.handId = message.snapshot.hand_id ?? '';
          sendActFrame(pending.id, pending.action, pending.amount, pending.snapshotVersion, pending.handId);
        } else {
          clearPending(message.action_id);
        }
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
        for (const id of [readyActionRef.current, showCardsActionRef.current, postBigBlindActionRef.current,
			  requestRabbitHuntActionRef.current, requestWinnerCardsActionRef.current, requestExitActionRef.current]) {
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
        // Otherwise a stale id here blocks the next hand's auto-post (the
        // `!postBigBlindActionRef.current` guard above) until its own ack
        // or the internal 2s x3 retry loop eventually times it out.
        postBigBlindActionRef.current = null;
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
      if (TERMINAL_ERROR_CODES.has(code)) setTerminalFailure({tableID: id, code});
      // stale_state is retryable (not just resync-and-give-up) while under
      // MAX_ACTION_RETRIES; once exhausted it falls through to the normal
      // failPending path below like any other rejection.
      const retriesUsed = pendingActionRef.current?.retries ?? 0;
      const keepsPending = code === 'stale_state' && message.action_id &&
        pendingActionRef.current?.id === message.action_id && retriesUsed < MAX_ACTION_RETRIES;
      if (keepsPending && pendingActionRef.current) pendingActionRef.current.awaitingRetry = true;
      if (RESYNC_ERROR_CODES.has(code)) {
        // The server rejected against a state this client does not have.
        // Pull the authoritative snapshot instead of leaving the player
        // guessing; armResyncWatchdog escalates to a reconnect if even that
        // gets no answer.
        const actionId = message.action_id;
        // The action timeout is redundant once we know the server answered.
        // The resync runs on its own timer so failPending's clearPending
        // cannot cancel it.
        if (pendingTimer.current) clearTimeout(pendingTimer.current);
        // Keyed by this error's own action_id (falling back to a shared
        // '__global__' bucket for the rare action-less error) so scheduling
        // a resync for one command can never cancel or starve another's —
        // see the Map comment on resyncTimers/resyncWatchdogs above.
        const resyncKey = actionId || '__global__';
        const existingResync = resyncTimers.current.get(resyncKey);
        if (existingResync) clearTimeout(existingResync);
        // Backoff grows with each stale_state retry already spent on this
        // action (50ms, 100ms, 200ms, ...), plus up to 400ms of jitter so
        // simultaneous clients resyncing off the same broadcast don't all
        // hammer the table actor in lockstep.
        const backoffMs = code === 'rate_limited' ? 800 : Math.min(1600, 50 * 2 ** retriesUsed);
        const jitterMs = backoffMs + Math.floor(Math.random() * 400);
        resyncTimers.current.set(resyncKey, setTimeout(() => {
          resyncTimers.current.delete(resyncKey);
          if (keepsPending && pendingActionRef.current?.id !== actionId) return;
          // The id sent here is also the correlation key the reply is
          // matched against above, so the watchdog must be armed under
          // this exact same id, not a separately generated one.
          const syncActionId = actionId || crypto.randomUUID();
          sendRef.current({type: 'sync_state', action_id: syncActionId});
          armResyncWatchdog(syncActionId);
        }, jitterMs));
      }
      if (keepsPending) {
        // Auto-retry is already in flight (armed above via awaitingRetry) —
        // don't surface the "sincronizando o estado" alert for a retry that
        // will very likely resolve on its own within a couple hundred ms.
        // failPending (below) still surfaces it once MAX_ACTION_RETRIES is
        // actually exhausted.
      } else if (message.action_id && pendingActionRef.current?.id === message.action_id) {
        failPending(code, message.action_id);
      } else if (message.action_id) {
        if (!retryAuxiliaryCommand(message.action_id, code)) finishAuxiliaryCommand(message.action_id, code);
      } else setLastActionError(actionError(code));
    }
    if (message.type === 'connected') {
      awaitingReconnectSnapshotRef.current = true;
    }
    if (message.type === 'bot_challenge') setBotChallengeRequired(true);
    if (message.type === 'bot_challenge_passed') {
      setBotChallengeRequired(false);
      setLastActionError(null);
    }
    if (message.type === 'removed') setRemoved({code: message.code, amount: message.amount});
    if (message.type === 'achievement_unlocked' && message.key) setUnlock({
      key: message.key,
      stars: message.stars || 1
    });
    if (message.type === 'chat' && message.message && !isSuppressed(message.player_id)) {
      const chatMessage = message.message;
      const player = message.player_id || '?';
      const id = message.action_id || `${Date.now()}-${player}-${chatMessage}`;
      noteFreshChatArrivals([{id, player, message: chatMessage}]);
      setChat(value => value.some(item => item.id === id) ? value :
        [...value.slice(-(CHAT_HISTORY_LIMIT - 1)), {id, player, message: chatMessage, timestamp: Date.now()}]);
    }
    if (message.type === 'reaction' && message.player_id && message.reaction_id &&
      isTableReaction(message.reaction_id) && !isSuppressed(message.player_id)) {
      const reaction: TableReactionEvent = {
        id: message.action_id || `${Date.now()}-${message.player_id}-${message.reaction_id}-${Math.random()}`,
        playerId: message.player_id,
        reactionId: message.reaction_id,
        targetPlayerId: message.target_player_id || undefined
      };
      showReaction(reaction);
    }
  }, [armResyncWatchdog, clearPending, failPending, finishAuxiliaryCommand, id, isSuppressed,
    noteFreshChatArrivals, retryAuxiliaryCommand, sendActFrame, showReaction, viewerId]);

  const receiveForTable = useCallback((message: ServerMessage) => {
    if (activeTableIDRef.current === id) receive(message);
  }, [id, receive]);
  
  const origin = wsOrigin();
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
      setRequestRabbitHuntPending(false);
			setRequestWinnerCardsPending(false);
      setLastActionError(null);
      setAnnouncement('');
      setRemoved(null);
      setTerminalFailure(null);
      setChat([]);
      seenChatIdsRef.current = null;
      setChatBubbles({});
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
    // Without a token the server answers unauthorized and closes right after
    // the upgrade, which resets ws-client's backoff — an endless loop rather
    // than a bounded retry. Wait for a session instead.
    enabled: Boolean(wsUrl) && !USE_MOCK && Boolean(socketAuthToken) &&
      apiLiveness.status === 'available' && !terminalError,
    authToken: socketAuthToken || undefined,
    shareCode,
    onOpen: handleOpen
  });
  useEffect(() => {
    if (!USE_MOCK && socketAuthToken && (wsStatus === 'error' || wsStatus === 'disconnected')) {
      void checkApiLiveness();
    }
  }, [socketAuthToken, wsStatus]);
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
            for (const actionId of [readyActionRef.current, showCardsActionRef.current, postBigBlindActionRef.current,
			  requestRabbitHuntActionRef.current, requestWinnerCardsActionRef.current, requestExitActionRef.current]) {
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
  const reconnectAttempt = USE_MOCK ? mockReconnectAttempt : wsReconnectAttempt;
  const rawStatus = USE_MOCK ? mockStatus : apiLiveness.status === 'unavailable' ? 'disconnected' : wsStatus;
  // @aoctech/ws-client reports 'error'/'disconnected' the instant a socket
  // drops, then only flips to 'reconnecting' once its retry timer actually
  // fires — so every drop flashes the red "connection lost" state even
  // though a retry is already guaranteed. reconnectAttempt <= MAX means the
  // library hasn't given up (see connectionCopyFor's identical threshold in
  // table/page.tsx): show 'reconnecting' for that whole window instead, and
  // only surface the raw status once retries are truly exhausted.
  const status = (rawStatus === 'error' || rawStatus === 'disconnected') && reconnectAttempt <= MAX_RECONNECT_ATTEMPTS
    ? 'reconnecting'
    : rawStatus;
  useEffect(() => {
    sendRef.current = send;
    retryNowRef.current = retryNow;
  }, [retryNow, send]);
  
  useEffect(() => () => {
    for (const timer of resyncWatchdogs.current.values()) clearTimeout(timer);
    resyncWatchdogs.current.clear();
    for (const timer of resyncTimers.current.values()) clearTimeout(timer);
    resyncTimers.current.clear();
  }, []);
  
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
    requestRabbitHuntActionRef.current = null;
		requestWinnerCardsActionRef.current = null;
    requestExitActionRef.current = null;
    readyLockRef.current = false;
    showCardsLockRef.current = false;
    requestRabbitHuntLockRef.current = false;
		requestWinnerCardsLockRef.current = false;
    requestExitLockRef.current = false;
    for (const timer of [pendingTimer.current, readyTimerRef.current, showCardsTimerRef.current,
		  postBigBlindTimerRef.current, requestRabbitHuntTimerRef.current, requestWinnerCardsTimerRef.current,
		  requestExitTimerRef.current]) {
      if (timer) clearTimeout(timer);
    }
    pendingTimer.current = undefined;
    readyTimerRef.current = undefined;
    showCardsTimerRef.current = undefined;
    postBigBlindTimerRef.current = undefined;
    requestRabbitHuntTimerRef.current = undefined;
		requestWinnerCardsTimerRef.current = undefined;
    requestExitTimerRef.current = undefined;
    pendingActionRef.current = null;
    for (const aux of auxFramesRef.current.values()) if (aux.timer) clearTimeout(aux.timer);
    auxFramesRef.current.clear();
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
    if (requestRabbitHuntTimerRef.current) clearTimeout(requestRabbitHuntTimerRef.current);
		if (requestWinnerCardsTimerRef.current) clearTimeout(requestWinnerCardsTimerRef.current);
    if (requestExitTimerRef.current) clearTimeout(requestExitTimerRef.current);
    for (const aux of auxFramesRef.current.values()) if (aux.timer) clearTimeout(aux.timer);
    auxFramesRef.current.clear();
    for (const timer of reactionTimersRef.current.values()) window.clearTimeout(timer);
    reactionTimersRef.current.clear();
    for (const timer of soundTimersRef.current) window.clearTimeout(timer);
    soundTimersRef.current.clear();
  }, []);
  
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
  
  // Records the frame under its action_id before sending, so a resync-class
  // rejection can resubmit this exact command instead of failing it outright
  // (see retryAuxiliaryCommand). Only for commands that carry an action_id the
  // server echoes back — fire-and-forget ones have nothing to correlate on.
  const emitAux = useCallback((actionId: string, frame: object) => {
    auxFramesRef.current.set(actionId, {frame, retries: 0});
    if (emit(frame)) return true;
    auxFramesRef.current.delete(actionId);
    return false;
  }, [emit]);

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
    pendingActionRef.current = {id: actionId, action, amount, snapshotVersion, handId, retries: 0, awaitingRetry: false};
    setPendingAction(action);
    return sendActFrame(actionId, action, amount, snapshotVersion, handId);
  }, [sendActFrame]);
  
  // Suppression is applied twice on purpose: incoming frames are dropped before
  // they reach state (no bubble, no animation, no announcement), and what is
  // already on screen disappears the moment a mute or block is confirmed.
  const visibleChat = suppressedPlayerIds?.size
    ? chat.filter(item => !suppressedPlayerIds.has(item.player)) : chat;
  const visibleBubbles = suppressedPlayerIds?.size
    ? Object.fromEntries(Object.entries(chatBubbles).filter(([playerId]) => !suppressedPlayerIds.has(playerId)))
    : chatBubbles;
  const visibleReactions = suppressedPlayerIds?.size
    ? reactions.filter(item => !suppressedPlayerIds.has(item.playerId)) : reactions;

  return {
    status,
    snapshot: snapshotTableID === id ? snapshot : null,
    snapshotAt,
    unlock,
    chat: visibleChat,
    chatBubbles: visibleBubbles,
    reactions: visibleReactions,
    pendingAction,
    actionError: lastActionError,
    reconnectAttempt,
    announcement,
    botChallengeRequired,
    removed,
    terminalError,
    clearActionError: () => setLastActionError(null),
    retryNow,
    readyPending,
    showCardsPending,
    requestRabbitHuntPending,
    requestRabbitHuntFailCount,
		requestWinnerCardsPending,
    requestExitPending,
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
      const ok = emitAux(actionId, {type: 'show_cards', action_id: actionId, card_index: cardIndex});
      if (!ok) {
        finishAuxiliaryCommand(actionId);
        return false;
      }
      showCardsTimerRef.current = setTimeout(() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS);
      return ok;
    },
    requestRabbitHunt: () => {
      if (requestRabbitHuntLockRef.current) return false;
      const actionId = crypto.randomUUID();
      requestRabbitHuntLockRef.current = true;
      requestRabbitHuntActionRef.current = actionId;
      setRequestRabbitHuntPending(true);
      const ok = emitAux(actionId, {type: 'request_rabbit_hunt', action_id: actionId});
      if (!ok) {
        finishAuxiliaryCommand(actionId);
        return false;
      }
      requestRabbitHuntTimerRef.current = setTimeout(() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS);
      return ok;
    },
    requestExit: () => {
      if (requestExitLockRef.current) return false;
      const actionId = crypto.randomUUID();
      requestExitLockRef.current = true;
      requestExitActionRef.current = actionId;
      setRequestExitPending(true);
      const ok = emitAux(actionId, {type: 'request_exit', action_id: actionId});
      if (!ok) {
        finishAuxiliaryCommand(actionId);
        return false;
      }
      requestExitTimerRef.current = setTimeout(() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS);
      return ok;
    },
    // Fire-and-forget like reportRabbitHuntVerifyFailed below: cancellation
    // is reflected in the next snapshot's pending_exit field, no local
    // pending/lock state needed.
    cancelExit: () => emit({type: 'cancel_exit', action_id: crypto.randomUUID()}),
		requestWinnerCards: () => {
			if (requestWinnerCardsLockRef.current) return false;
			const actionId = crypto.randomUUID();
			requestWinnerCardsLockRef.current = true;
			requestWinnerCardsActionRef.current = actionId;
			setRequestWinnerCardsPending(true);
			const ok = emitAux(actionId, {type: 'request_winner_cards', action_id: actionId});
			if (!ok) {
				finishAuxiliaryCommand(actionId);
				return false;
			}
			requestWinnerCardsTimerRef.current = setTimeout(
				() => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS
			);
			return ok;
		},
    // The winner answering the pending request. It shares requestWinnerCards'
    // in-flight slot on purpose: a client is either the requester or the winner
    // of a given request, never both, so one slot is all a session can need —
    // and WinnerCards.tsx disables both answers off that pending flag.
    answerWinnerCards: (accept: boolean) => {
      if (requestWinnerCardsLockRef.current) return false;
      const actionId = crypto.randomUUID();
      requestWinnerCardsLockRef.current = true;
      requestWinnerCardsActionRef.current = actionId;
      setRequestWinnerCardsPending(true);
      const ok = emitAux(actionId, {type: accept ? 'accept_winner_cards' : 'decline_winner_cards', action_id: actionId});
      if (!ok) {
        finishAuxiliaryCommand(actionId);
        return false;
      }
      requestWinnerCardsTimerRef.current = setTimeout(
        () => finishAuxiliaryCommand(actionId, 'action_timeout'), ACTION_TIMEOUT_MS
      );
      return ok;
    },
    // Fire-and-forget: RabbitHunt.tsx already shows its own "taxa devolvida"
    // message locally the moment verification fails, independent of the
    // server's response, so nothing in the UI waits on this ack.
    reportRabbitHuntVerifyFailed: () => emit({type: 'rabbit_hunt_verify_failed', action_id: crypto.randomUUID()}),
    keepSeat: () => emit({type: 'keep_seat', action_id: crypto.randomUUID()}),
    // Fire-and-forget, once per hand: lets achievements.Service tell a
    // genuinely blind all-in/win from one the client just never reported.
    // No pending/lock state — nothing in the UI waits on the server's ack.
    peekCards: () => emit({type: 'peek_cards', action_id: crypto.randomUUID()}),
    setRunItTwice: (enabled: boolean) => emit({type: 'set_run_it_twice', run_it_twice: enabled}),
    sendChat: (message: string) => emit({type: 'chat', message, action_id: crypto.randomUUID()}),
    sendReaction: (reactionId: TableReactionID, targetPlayerId?: string) =>
      emit({
        type: 'reaction', reaction_id: reactionId, target_player_id: targetPlayerId || '',
        action_id: crypto.randomUUID()
      }),
    preselectAction: (selection: ActionPreselection | null, amount = 0) => emit({
      type: 'preselect_action', action: selection || '', action_id: crypto.randomUUID(),
      amount,
      expected_snapshot_version: latestVersionRef.current,
      expected_hand_id: latestHandIDRef.current,
      expected_stage: snapshot?.stage || ''
    }),
    submitBotChallenge
  };
}
