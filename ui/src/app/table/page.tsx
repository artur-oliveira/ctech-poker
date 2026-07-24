'use client';
import Link from 'next/link';
import {Suspense, useEffect, useRef, useState} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {ChevronLeft, Pause, Play, RotateCw, Wifi} from 'lucide-react';
import {getViewerId} from '@/lib/utils';
import {useTableRealtime} from '@/lib/hooks/useTableRealtime';
import {getRoom, getSeated} from '@/lib/api/rooms';
import {isNotFound} from '@/lib/api/client';
import {BuyInPanel} from '@/components/table/BuyInPanel';
import {TableStage} from '@/components/table/TableStage';
import type {ActionAvailability} from '@/components/table/ActionBar';
import {ActionBar} from '@/components/table/ActionBar';
import {Chat} from '@/components/table/Chat';
import {InviteDialog} from '@/components/table/InviteDialog';
import {LeaveDialog} from '@/components/table/LeaveDialog';
import {MockControls} from '@/components/table/MockControls';
import type {HandOutcomeState} from '@/components/table/HandOutcome';
import {HandRankingsDialog} from '@/components/table/HandRankingsDialog';
import {AchievementToast} from '@/components/AchievementToast';
import {TermsGate} from '@/components/TermsGate';
import {Button} from '@/components/ui/button';
import {pushNotification} from '@/lib/notify';
import type {PokerAction, TableSnapshot} from '@/lib/api/table';
import {HAND_RANK_INDEX} from '@/lib/pokerRules';
import {type MockScenario, USE_MOCK} from '@/lib/mock';
import {MAX_RECONNECT_ATTEMPTS} from '@aoctech/ws-client';

const ROOM_ID = /^[a-f0-9]{32}$/i;
const CONNECTION_COPY = {
  connecting: 'Conectando à mesa…',
  reconnecting: 'Reconectando à mesa…',
  disconnected: 'Conexão interrompida. Tentando novamente…',
  error: 'A conexão oscilou. Suas fichas continuam seguras.'
} as const;
const REMOVED_REASON_COPY: Record<string, string> = {
  idle: 'Você foi removido da mesa por inatividade.',
  disconnected: 'Você foi removido da mesa após ficar desconectado por muito tempo.'
};
// @aoctech/ws-client gives up on its own retry loop after MAX_RECONNECT_ATTEMPTS
// and never schedules another one — only a fresh token (handled elsewhere) or
// this button's retryNow() tries again. Telling the player "tentando
// novamente" past that point would be a lie.
const RECONNECT_GIVEN_UP_COPY = 'Conexão perdida. Toque para tentar novamente.';
const STAGE_LABELS: Record<string, string> = {
  waiting_for_players: 'Aguardando jogadores', pre_flop: 'Pré-flop', flop: 'Flop', turn: 'Turn', river: 'River',
  showdown: 'Showdown', complete: 'Mão encerrada'
};
const BETTING_STAGES = new Set(['pre_flop', 'flop', 'turn', 'river']);

function connectionCopyFor(status: keyof typeof CONNECTION_COPY, attempt: number) {
  if (status === 'disconnected' && attempt > MAX_RECONNECT_ATTEMPTS) return RECONNECT_GIVEN_UP_COPY;
  return CONNECTION_COPY[status];
}

const MOCK_SCENARIOS = new Set<MockScenario>(['full_hand', 'waiting', 'pre_flop', 'flop', 'turn', 'river', 'showdown', 'side_pot', 'complete', 'reconnecting', 'action_error', 'timeout']);

function actionState(snapshot: TableSnapshot, viewer?: string) {
  const seat = snapshot.seats.find(item => item.player_id === viewer);
  const serverActions = snapshot.legal_actions;
  const currentContribution = Math.max(0, ...snapshot.seats.map(item => item.contributed));
  const callAmount = serverActions?.call_amount ?? Math.max(0, currentContribution - (seat?.contributed || 0));
  const isTurn = snapshot.current_player_id ? snapshot.current_player_id === viewer : Boolean(seat && seat.state === 'active' && BETTING_STAGES.has(snapshot.stage));
  const legacyActions: PokerAction[] = !seat || !BETTING_STAGES.has(snapshot.stage) || seat.state !== 'active' ? [] : [
    'fold', callAmount > 0 ? 'call' : 'check', ...(seat.stack > callAmount ? ['raise' as const] : [])
  ];
  const actions = new Set(serverActions?.actions || legacyActions);
  const available: ActionAvailability = {
    fold: actions.has('fold'), check: actions.has('check'), call: actions.has('call'), raise: actions.has('raise')
  };
  const legacyMax = Math.max(25, (seat?.stack || 0) + (seat?.contributed || 0));
  const maxRaise = Math.max(0, serverActions?.max_raise_to ?? legacyMax);
  const minRaise = Math.min(maxRaise, Math.max(0, serverActions?.min_raise_to ?? Math.min(100, maxRaise)));
  return {available, callAmount, isTurn, minRaise, maxRaise, raiseStep: serverActions?.step || 25};
}

function TableContent() {
  const router = useRouter();
  const params = useSearchParams(), id = params.get('id') || '', valid = ROOM_ID.test(id);
  const inviteCode = params.get('invite') || undefined;
  const requestedScenario = params.get('scenario') as MockScenario | null;
  const scenario: MockScenario = requestedScenario && MOCK_SCENARIOS.has(requestedScenario) ? requestedScenario : 'full_hand';
  const requestedDelay = Number(params.get('delay') || 350);
  const delay = [0, 350, 1200, 9000].includes(requestedDelay) ? requestedDelay : 350;
  const viewer = getViewerId();
  const {data: room} = useQuery({
    queryKey: ['room', id], queryFn: () => getRoom(id), enabled: valid,
    retry: (count, err) => !isNotFound(err) && count < 3
  });
  const queryClient = useQueryClient();
  // Buy-in is an explicit ceremony: nothing is debited until the player
  // confirms an amount. The server (not local browser storage) is the
  // source of truth for "is this player already seated" — that is what
  // lets a player return via a new tab, a different browser, or a
  // different device without repeating the ceremony for a seat they
  // already have.
  const {data: seatedStatus, isLoading: seatedLoading} = useQuery({
    queryKey: ['seated', id], queryFn: () => getSeated(id), enabled: valid,
    retry: (count, err) => !isNotFound(err) && count < 3
  });
  const seated = seatedStatus?.seated ?? false;
  const rt = useTableRealtime(valid && seated ? id : '', viewer, inviteCode, USE_MOCK ? {scenario, delay} : undefined);
  // The server never closes a removed player's socket (it just stops
  // targeting it in future broadcasts) — without reacting to this message the
  // client would otherwise sit frozen on the last snapshot it received, or
  // silently reconnect into a seat it no longer holds.
  useEffect(() => {
    if (!rt.removed) return;
    pushNotification(REMOVED_REASON_COPY[rt.removed.code || ''] || 'Você foi removido da mesa.', 'info');
    queryClient.setQueryData(['seated', id], {seated: false, stack: 0});
    router.push('/lobby');
  }, [rt.removed, id, queryClient, router]);
  // The next-hand deadline is fixed server-side once armed, but a state
  // broadcast can still arrive mid-countdown (e.g. another player revealing
  // cards) and shift rt.snapshotAt forward. Recomputing animationDuration
  // against that later snapshotAt would shrink the CSS animation's total
  // duration while it's already running, snapping the ring to its end frame
  // long before the real 5s deadline. Freezing the duration at the first
  // snapshot that armed this deadline keeps the ring in sync with backend
  // time regardless of how many broadcasts land before it fires.
  const [nextHandArmed, setNextHandArmed] = useState<{ deadline: number; snapshotAt: number } | null>(null);
  // Fires the win/lose banner exactly once per resolved hand: payouts appear
  // once when a hand completes and stay put across every later broadcast of
  // that same `complete` snapshot (show_cards, pings, ...), so comparing
  // against the previous render's payouts (not the current one) is what
  // keeps this from re-firing on those repeats.
  const previousPayoutsRef = useRef<TableSnapshot['payouts']>(undefined);
  const outcomeKeyRef = useRef(0);
  const [handOutcome, setHandOutcome] = useState<HandOutcomeState | null>(null);
  useEffect(() => {
    const snap = rt.snapshot;
    const isFreshPayout = Boolean(snap?.payouts) && !previousPayoutsRef.current;
    previousPayoutsRef.current = snap?.payouts;
    if (!isFreshPayout || !snap?.payouts || !viewer) return;
    // Only a viewer who stayed in for the whole hand (never folded, never sat
    // out) gets a win/lose moment — folding is routine and already has its
    // own quiet "Desistiu" seat state; celebrating or consoling every single
    // fold would turn the delight into noise.
    const seat = snap.seats.find(item => item.player_id === viewer);
    if (seat?.state !== 'active' && seat?.state !== 'all_in') return;
    outcomeKeyRef.current += 1;
    const amount = snap.payouts[viewer] || 0;
    const kind = amount > 0 ? 'win' : 'lose';
    // The banner names one rival hand as the point of comparison: the
    // toughest hand it beat when the viewer won (proof it beat everyone),
    // or the hand that actually beat it when the viewer lost. Only seats
    // that reached showdown carry hand_category, so this stays undefined
    // (and the banner falls back to the plain category chip) whenever the
    // hand ended without one — e.g. everyone else folded.
    const opponentCategory = kind === 'win' ?
      snap.seats.filter(item => item.player_id !== viewer && item.hand_category)
        .sort((a, b) => HAND_RANK_INDEX[a.hand_category!] - HAND_RANK_INDEX[b.hand_category!])[0]?.hand_category :
      snap.seats.find(item => item.player_id !== viewer && (snap.payouts?.[item.player_id] || 0) > 0)?.hand_category;
    setHandOutcome({
      key: outcomeKeyRef.current, kind, amount, handCategory: seat.hand_category, opponentCategory
    });
  }, [rt.snapshot, viewer]);
  if (!valid) return (
    <main className="game-loading">
      <h1 className="sr-only">Mesa de poker</h1>
      <h2>Mesa inválida</h2>
      <p>O identificador precisa ser um código de sala válido.</p>
      <Button render={<Link href="/lobby"/>}>Voltar ao lobby</Button>
    </main>
  );
  if (seatedLoading) return (
    <main className="game-loading">
      <h1 className="sr-only">Mesa de poker</h1>
      <span className="loader"/>
    </main>
  );
  if (!seated) return <>
    <BuyInPanel roomId={id} shareCode={inviteCode} onSeatedAction={() => {
      queryClient.setQueryData(['seated', id], {seated: true, stack: 0});
    }}/>
    {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
  </>;
  if (!rt.snapshot) return <>
    <main className="game-loading">
      <h1 className="sr-only">Mesa de poker</h1>
      <span className="loader"/>
      <h2>{rt.status === 'connected' ? 'Aquecendo o seu lugar…' : 'Conectando à mesa…'}</h2>
      <p role="status"
         aria-live="polite">{rt.status === 'connected' ? 'Sincronizando o estado mais recente.' : connectionCopyFor(rt.status, rt.reconnectAttempt)}</p>
      {rt.status !== 'connected' &&
          <Button variant="outline" onClick={rt.retryNow}><RotateCw/> Tentar agora</Button>}
    </main>
    {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
  </>;
  const s = rt.snapshot, pot = s.seats.reduce((n, x) => n + x.contributed, 0);
  const bigBlind = room?.big_blind || 25;
  const connectionMessage = rt.status === 'connected' ? null : connectionCopyFor(rt.status, rt.reconnectAttempt);
  const actions = actionState(s, viewer);
  const viewerSeat = s.seats.find(seat => seat.player_id === viewer);
  const actionKey = [s.stage, s.current_player_id, s.board.join(','), viewerSeat?.stack, viewerSeat?.contributed,
    actions.minRaise, actions.maxRaise, actions.raiseStep].join(':');
  // A room's share_code is only ever present for its own creator (the server
  // strips it from every other viewer) — so its presence alone gates the
  // invite affordance for private tables; public tables need no code at all.
  if (s.next_hand_unix_ms && nextHandArmed?.deadline !== s.next_hand_unix_ms) {
    setNextHandArmed({deadline: s.next_hand_unix_ms, snapshotAt: rt.snapshotAt});
  }
  const nextHandDurationMs = s.next_hand_unix_ms && nextHandArmed?.deadline === s.next_hand_unix_ms ?
    Math.max(0, s.next_hand_unix_ms - nextHandArmed.snapshotAt) : 0;
  const canInvite = room && (room.visibility === 'public' || room.share_code);
  const canShowCards = s.stage === 'complete' && s.won_without_showdown && viewerSeat &&
    viewerSeat.state !== 'sitting_out' && viewerSeat.state !== 'pending_entry';
  const inviteUrl = typeof window !== 'undefined' ?
    `${window.location.origin}/table?id=${id}${room?.share_code ? `&invite=${room.share_code}` : ''}` : '';
  return (
    <main className="game">
      <h1 className="sr-only">Mesa de poker — {STAGE_LABELS[s.stage] || s.stage.replaceAll('_', ' ')}</h1>
      <div className="game-chrome">
        <header>
          <Link href="/lobby" aria-label="Voltar ao lobby"><ChevronLeft/> <span
            className="header-lobby-label">Lobby</span></Link>
          <span aria-hidden="true">{STAGE_LABELS[s.stage] || s.stage.replaceAll('_', ' ')}</span>
          <div className="header-right">
            <span className={`connection-state ${rt.status}`}>
              <Wifi aria-hidden="true"/>
              <span className="connection-label">{rt.status === 'connected' ? 'Ao vivo' : 'Reconectando'}</span>
            </span>
            <HandRankingsDialog/>
            {canInvite && <InviteDialog url={inviteUrl}/>}
            {viewerSeat && viewerSeat.state !== 'sitting_out' &&
                <Button type="button" variant="ghost" size="icon" aria-label="Sentar fora" disabled={rt.readyPending}
                        onClick={() => rt.ready(false)}><Pause/></Button>}
            {viewerSeat?.state === 'sitting_out' &&
                <Button type="button" variant="ghost" size="icon" aria-label="Voltar a jogar" disabled={rt.readyPending}
                        onClick={() => rt.ready(true)}><Play/></Button>}
            <LeaveDialog roomId={id} stack={viewerSeat?.stack || 0} onLeft={amount => {
              pushNotification(`Você saiu com ${amount.toLocaleString('pt-BR')} fichas.`, 'info');
              queryClient.setQueryData(['seated', id], {seated: false, stack: 0});
              router.push('/lobby');
            }}/>
          </div>
        </header>
        <div className="sr-only" role="status" aria-live="polite" aria-atomic="true">
          {[rt.announcement, rt.status === 'connected' ? 'Conexão com a mesa restaurada.' : connectionMessage]
            .filter(Boolean).join(' ')}
        </div>
        {connectionMessage && <div className={`reconnect-notice ${rt.status}`}>
            <span aria-hidden="true"/>
            <p>{connectionMessage}{rt.reconnectAttempt > 1 ? ` Tentativa ${rt.reconnectAttempt}.` : ''}</p>
            <Button type="button" variant="ghost" onClick={rt.retryNow}><RotateCw/> Tentar agora</Button>
        </div>}
        {/* The header's stage label already reads "aguardando jogadores" / "mão
            encerrada" — this box only earns its place when it has something
            the header doesn't: the next-hand countdown or the show-cards
            action. An empty-room wait with neither would otherwise float a
            duplicate label over the header. On phones, this notice renders
            in-flow right below the header (see .game-chrome / .reconnect-notice
            mobile rules) instead of floating fixed over it — a floating
            overlay can't reliably avoid the header once it wraps to two
            lines, which used to hide and block taps on Sentar fora/Sair da
            mesa for the whole time this notice was up. */}
        {!connectionMessage && (s.next_hand_unix_ms || canShowCards) && <div className="reconnect-notice">
            <p>{s.stage === 'complete' ? 'Mão encerrada.' : 'Aguardando jogadores.'}</p>
          {s.next_hand_unix_ms &&
              <span key={s.next_hand_unix_ms} className="next-hand-ring"
                    style={{
                      '--ring-duration': `${nextHandDurationMs}ms`,
                      '--urgent-delay': `${Math.max(0, nextHandDurationMs - 3000)}ms`,
                    } as React.CSSProperties}
                    aria-hidden="true"/>}
          {canShowCards &&
              <Button type="button" variant="ghost" disabled={rt.showCardsPending}
                      onClick={() => rt.showCards()}>Mostrar cartas</Button>}
        </div>}
      </div>
      <TableStage snapshot={s} viewer={viewer} pot={pot} bigBlind={bigBlind} nowMs={rt.snapshotAt}
                  outcome={handOutcome} holdOutcomeOpen={s.stage === 'complete'}/>
      <ActionBar
        onActAction={rt.act}
        {...actions}
        pot={pot}
        actionKey={actionKey}
        connected={rt.status === 'connected'}
        pending={rt.pendingAction}
        error={rt.actionError} onDismissErrorAction={rt.clearActionError}/>
      <Chat items={rt.chat} onSend={rt.sendChat} connected={rt.status === 'connected'} viewerId={viewer}
            seats={s.seats}/><AchievementToast
      unlock={rt.unlock}/>
      {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
    </main>
  );
}

export default function TablePage() {
  return <TermsGate><Suspense
    fallback={<main className="game-loading"><span className="loader"/></main>}><TableContent/></Suspense></TermsGate>;
}
