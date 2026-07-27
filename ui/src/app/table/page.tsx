'use client';
import Link from 'next/link';
import {Suspense, useEffect, useRef, useState} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {ChevronLeft, ClockAlert, Pause, Play, RotateCw, Wifi} from 'lucide-react';
import {getViewerId} from '@/lib/utils';
import {useTableRealtime} from '@/lib/hooks/useTableRealtime';
import {getRoom, getSeated} from '@/lib/api/rooms';
import {isNotFound} from '@/lib/api/client';
import {BuyInPanel} from '@/components/table/BuyInPanel';
import {TableStage} from '@/components/table/TableStage';
import {PerimeterTimer} from '@/components/table/PerimeterTimer';
import type {ActionAvailability} from '@/components/table/ActionBar';
import {ActionBar} from '@/components/table/ActionBar';
import {Chat} from '@/components/table/Chat';
import {InviteDialog} from '@/components/table/InviteDialog';
import {LeaveDialog} from '@/components/table/LeaveDialog';
import {RebuyDialog} from '@/components/table/RebuyDialog';
import {MockControls} from '@/components/table/MockControls';
import type {HandOutcomeState} from '@/components/table/HandOutcome';
import {LastWinners} from '@/components/table/LastWinners';
import {HandRankingsDialog} from '@/components/table/HandRankingsDialog';
import {TablePreferencesDialog} from '@/components/table/TablePreferencesDialog';
import {RealityCheck} from '@/components/table/RealityCheck';
import {PlayerNoteDialog} from '@/components/table/PlayerNoteDialog';
import {TableReactions} from '@/components/table/TableReactions';
import {BotChallenge} from '@/components/table/BotChallenge';
import {AchievementToast} from '@/components/AchievementToast';
import {TermsGate} from '@/components/TermsGate';
import {Button} from '@/components/ui/button';
import {pushNotification} from '@/lib/notify';
import type {TableSnapshot} from '@/lib/api/table';
import {bestFiveCardHand} from '@/lib/pokerRules';
import {getHands, getSessions} from '@/lib/api/player';
import {useTablePreferences} from '@/lib/tablePreferences';
import {useDealerVoice} from '@/lib/hooks/useDealerVoice';
import {getPlayerNotes, type PlayerNote} from '@/lib/api/playerNotes';
import {
  playerPotBreakdown,
  relevantWinner,
  seatParticipated,
  shouldShowOutcome,
  tableOutcomeKind
} from '@/lib/tableOutcome';
import {type MockScenario, USE_MOCK} from '@/lib/mock';
import {MAX_RECONNECT_ATTEMPTS} from '@aoctech/ws-client';

const ROOM_ID = /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/;
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
function connectionCopyFor(status: keyof typeof CONNECTION_COPY, attempt: number) {
  if (status === 'disconnected' && attempt > MAX_RECONNECT_ATTEMPTS) return RECONNECT_GIVEN_UP_COPY;
  return CONNECTION_COPY[status];
}

const MOCK_SCENARIOS = new Set<MockScenario>([
  'full_hand', 'full_hand_loss', 'full_hand_tie', 'all_in', 'auto_fold',
  'waiting', 'pre_flop', 'flop', 'turn', 'river', 'showdown', 'side_pot',
  'complete', 'complete_loss', 'complete_tie', 'fold_win',
  'reconnecting', 'action_error', 'timeout'
]);

function actionState(snapshot: TableSnapshot, viewer?: string) {
  const seat = snapshot.seats.find(item => item.player_id === viewer);
  const serverActions = snapshot.legal_actions;
  const callAmount = Math.min(seat?.stack || 0, Math.max(0, serverActions?.call_amount || 0));
  // An empty current_player_id is a deliberate server state (street
  // transition, runout, showdown), never permission for the viewer to act.
  const isTurn = Boolean(viewer && snapshot.current_player_id === viewer);
  const actions = new Set(serverActions?.actions || []);
  const available: ActionAvailability = {
    fold: actions.has('fold'), check: actions.has('check'), call: actions.has('call'), raise: actions.has('raise')
  };
  const maxRaise = Math.max(0, serverActions?.max_raise_to || 0);
  const minRaise = Math.min(maxRaise, Math.max(0, serverActions?.min_raise_to || 0));
  return {
    available, callAmount, isTurn, minRaise, maxRaise, raiseStep: Math.max(1, serverActions?.step || 1),
    effectiveStack: Math.min(seat?.stack || 0, Math.max(0, ...snapshot.seats
      .filter(item => item.player_id !== viewer && item.state !== 'folded' && item.state !== 'sitting_out')
      .map(item => item.stack))),
    raisePresets: [
      {label: 'Mín', value: minRaise},
      {label: '⅓ pote', value: serverActions?.one_third_pot_raise_to || minRaise},
      {label: '½ pote', value: serverActions?.half_pot_raise_to || minRaise},
      {label: '⅔ pote', value: serverActions?.two_thirds_pot_raise_to || minRaise},
      {label: 'Pote', value: serverActions?.pot_raise_to || minRaise},
      {label: 'Máx', value: maxRaise}
    ]
  };
}

function IdleWarning({deadline, onKeepSeat}: {deadline?: number; onKeepSeat: () => boolean}) {
  const [now, setNow] = useState(0);
  useEffect(() => {
    if (!deadline) return undefined;
    const delay = Math.max(0, deadline - Date.now() - 60_000);
    let interval: ReturnType<typeof setInterval> | undefined;
    const timeout = setTimeout(() => {
      setNow(Date.now());
      interval = setInterval(() => setNow(Date.now()), 1000);
    }, delay);
    return () => {
      clearTimeout(timeout);
      if (interval) clearInterval(interval);
    };
  }, [deadline]);
  if (!deadline || !now) return null;
  const seconds = Math.max(0, Math.ceil((deadline - now) / 1000));
  if (seconds > 60) return null;
  return <div className="idle-warning" role="alert">
    <ClockAlert aria-hidden="true"/>
    <p>Você será removido por inatividade em <strong>{seconds}s</strong>.</p>
    <Button type="button" variant="outline" onClick={onKeepSeat}>Continuar na mesa</Button>
  </div>;
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
  const [tableOpenedAt] = useState(() => Math.floor(Date.now() / 1000));
  const {preferences} = useTablePreferences();
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
  // Last-winners strip: sourced from the player's own hand-history endpoint
  // (not live socket state) so it's populated from table load, not only
  // after the viewer sits through a fresh resolution. Re-fetched once per
  // resolved hand below.
  const {data: tableHands = []} = useQuery({
    queryKey: ['hands', id], queryFn: () => getHands({tableId: id}), enabled: valid
  });
  const {data: sessions = []} = useQuery({
    queryKey: ['sessions', 'me'], queryFn: () => getSessions(), enabled: valid && seated
  });
  const {data: playerNotes = []} = useQuery({
    queryKey: ['player-notes'], queryFn: getPlayerNotes, enabled: valid && seated
  });
  const [noteOpponent, setNoteOpponent] = useState<{player_id: string; name?: string} | null>(null);
  const rt = useTableRealtime(valid && seated ? id : '', viewer, inviteCode, USE_MOCK ? {scenario, delay} : undefined);
  useDealerVoice(rt.announcement, preferences.dealerVoice);
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
  const previousPayoutsRef = useRef<{tableID: string; payouts?: TableSnapshot['payouts']}>({tableID: ''});
  const outcomeKeyRef = useRef(0);
  const [rememberedStart, setRememberedStart] =
    useState<{tableID: string; handID: string; stack: number} | null>(null);
  const [scopedHandOutcome, setScopedHandOutcome] =
    useState<{tableID: string; value: HandOutcomeState} | null>(null);
  // Protocol v3 publishes the exact pre-blind stack. During a rolling deploy,
  // remember the earliest live snapshot as stack+contributed; unlike the old
  // stage-based state this is scoped to both table and hand and also works
  // when the first frame arrives on flop/turn after a reconnect.
  const liveSeat = rt.snapshot?.seats.find(item => item.player_id === viewer);
  if (rt.snapshot?.hand_id && liveSeat && seatParticipated(liveSeat) &&
    !Object.keys(rt.snapshot.payouts || {}).length &&
    (rememberedStart?.tableID !== id || rememberedStart.handID !== rt.snapshot.hand_id)) {
    setRememberedStart({
      tableID: id,
      handID: rt.snapshot.hand_id,
      stack: liveSeat.stack_at_hand_start ?? liveSeat.stack + liveSeat.contributed
    });
  }
  useEffect(() => {
    const snap = rt.snapshot;
    const hasPayouts = Boolean(snap?.payouts && Object.keys(snap.payouts).length > 0);
    const previousPayouts = previousPayoutsRef.current.tableID === id ?
      previousPayoutsRef.current.payouts : undefined;
    const isFreshPayout = hasPayouts && !previousPayouts;
    previousPayoutsRef.current = {tableID: id, payouts: hasPayouts ? snap?.payouts : undefined};
    if (isFreshPayout) void queryClient.invalidateQueries({queryKey: ['hands', id]});
    if (!isFreshPayout || !snap || !hasPayouts || !viewer) return;
    // Seat state does not prove participation: a player may become active
    // mid-hand after returning/rebuying, but they are only eligible for the
    // next deal. `dealt_in` is the server-authored membership of this hand.
    const seat = snap.seats.find(item => item.player_id === viewer);
    if (!shouldShowOutcome(seat)) return;
    outcomeKeyRef.current += 1;
    // Membership in `winners`, not a truthy payout, decides win/lose: an
    // uncalled all-in's excess or an orphaned side-pot refund also shows up
    // in `payouts` without being an actual win.
    const kind = tableOutcomeKind(snap, viewer);
    // The banner names one rival hand as the point of comparison when the
    // viewer lost at least one eligible pot. Only seats
    // that reached showdown carry hand_category, so this stays undefined
    // (and the banner falls back to the plain category chip) whenever the
    // hand ended without one — e.g. everyone else folded.
    const opponentCategory = (kind === 'lose' || kind === 'mixed') ?
      relevantWinner(snap, viewer)?.hand_category : undefined;
    // The hand that actually won this pot: the viewer's own cards on a win
    // (always known), or the first winning rival's cards on a loss — but only
    // when a showdown actually revealed them (a hand that ended with everyone
    // else folding never shows opponent cards). Combined with the board (once
    // the board is complete) and reduced to the actual best 5-card hand — a
    // bare pair of hole cards doesn't show what the player actually won with
    // when the winning combination uses the board too.
    const winnerSeat = kind === 'lose' ? relevantWinner(snap, viewer) : seat;
    const winnerHole = winnerSeat?.hole_cards?.length === 2 &&
    winnerSeat.hole_cards.every(card => card.toLowerCase() !== 'back') ? winnerSeat.hole_cards : undefined;
    const winningCards = winnerHole && snap.board.length === 5 ?
      bestFiveCardHand([...winnerHole, ...snap.board]) : winnerHole;
    const viewerHole = seat.hole_cards?.length === 2 &&
    seat.hole_cards.every(card => card.toLowerCase() !== 'back') ? seat.hole_cards : undefined;
    const viewerCards = viewerHole && snap.board.length === 5 ?
      bestFiveCardHand([...viewerHole, ...snap.board]) : viewerHole;
    const stackBefore = seat.stack_at_hand_start ??
      (rememberedStart?.tableID === id && rememberedStart.handID === snap.hand_id ? rememberedStart.stack : undefined);
    const breakdown = playerPotBreakdown(snap, viewer);
    setScopedHandOutcome({
      tableID: id,
      value: {
        key: outcomeKeyRef.current, kind, handCategory: seat.hand_category, opponentCategory,
        winningCards, winningHoleCards: winnerHole, viewerCards, viewerHoleCards: viewerHole,
        winnerName: winnerSeat?.name, stackBefore, stackAfter: seat.stack,
        wonAmount: breakdown.won, refundAmount: breakdown.refund
      }
    });
  }, [rt.snapshot, viewer, queryClient, id, rememberedStart]);
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
  const s = rt.snapshot, pot = s.pots?.reduce((n, x) => n + x.amount, 0) ??
    s.seats.reduce((n, x) => n + x.contributed, 0);
  const bigBlind = room?.big_blind || 25;
  const connectionMessage = rt.status === 'connected' ? null : connectionCopyFor(rt.status, rt.reconnectAttempt);
  const actions = actionState(s, viewer);
  const viewerSeat = s.seats.find(seat => seat.player_id === viewer);
  const viewerStackBefore = viewerSeat?.stack_at_hand_start ??
    (rememberedStart?.tableID === id && rememberedStart.handID === s.hand_id ? rememberedStart.stack : undefined);
  const handOutcome = scopedHandOutcome?.tableID === id ? scopedHandOutcome.value : null;
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
  const isPaused = viewerSeat?.ready === false || viewerSeat?.state === 'sitting_out';
  const canRevealCards = s.stage === 'complete' && seatParticipated(viewerSeat) &&
    Boolean(viewerSeat?.hole_cards?.some(card => card.toLowerCase() !== 'back')) &&
    !((s.protocol_version ?? 0) < 2 && !s.won_without_showdown && viewerSeat?.state !== 'folded') &&
    [0, 1].some(index => !(viewerSeat?.hole_cards_revealed?.[index] ?? false));
  const inviteUrl = typeof window !== 'undefined' ?
    `${window.location.origin}/table?id=${id}${room?.share_code ? `&invite=${room.share_code}` : ''}` : '';
  const openSession = sessions.find(session => session.table_id === id && session.ended_at === 0);
  const playerNotesByID = Object.fromEntries(playerNotes.map(note => [note.opponent_id, note]));
  return (
    <main className="game" data-table-theme={preferences.theme}>
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
            <TablePreferencesDialog/>
            {canInvite && <InviteDialog url={inviteUrl}/>}
            {viewerSeat && !isPaused &&
                <Button type="button" variant="ghost" size="icon" aria-label="Sentar fora" disabled={rt.readyPending}
                        onClick={() => rt.ready(false)}><Pause/></Button>}
            {viewerSeat && isPaused && viewerSeat.stack > 0 &&
                <Button type="button" variant="ghost" size="icon" aria-label="Voltar a jogar" disabled={rt.readyPending}
                        onClick={() => rt.ready(true)}><Play/></Button>}
            {viewerSeat && isPaused && viewerSeat.stack === 0 && room &&
              s.stage !== 'showdown' && s.stage !== 'complete' &&
                <RebuyDialog roomId={id} room={room} onRebuyAction={() => rt.ready(true)}/>}
            <LeaveDialog roomId={id} stack={viewerSeat?.stack || 0} onLeftAction={amount => {
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
        {!connectionMessage && s.next_hand_unix_ms && <div className="reconnect-notice">
            <p>{s.stage === 'complete' ? 'Mão encerrada.' : 'Aguardando jogadores.'}</p>
          {s.next_hand_unix_ms &&
              <PerimeterTimer className="next-hand-ring"
                              durationMs={nextHandDurationMs}
                              restartKey={s.next_hand_unix_ms}
                              radius={12}/>}
        </div>}
      </div>
      {/* `payouts`, not `stage === 'complete'`: a showdown hand shows payouts
          one broadcast earlier, at `stage === 'showdown'`, and only flips to
          `complete` a moment later. Gating on the stage name left a window
          where the outcome had already fired but holdOpen was still false,
          racing the banner's own exit timer into dismissing it before
          `complete` ever arrived. */}
      <TableStage snapshot={s} viewer={viewer} pot={pot} bigBlind={bigBlind} nowMs={rt.snapshotAt}
                  outcome={handOutcome} holdOutcomeOpen={Boolean(s.payouts && Object.keys(s.payouts).length > 0)}
                  viewerStackBefore={(s.payouts && Object.keys(s.payouts).length > 0) ? viewerStackBefore : undefined}
                  canRevealCards={canRevealCards} revealPending={rt.showCardsPending}
                  onRevealCard={index => rt.showCards(index)}
                  playerNotes={playerNotesByID}
                  onEditPlayerNote={seat => setNoteOpponent({player_id: seat.player_id, name: seat.name})}/>
      <ActionBar
        onActAction={rt.act}
        {...actions}
        actionKey={actionKey}
        selectionScope={`${s.hand_id || 'waiting'}:${s.stage}`}
        canPreselect={Boolean(viewerSeat?.dealt_in && viewerSeat.state === 'active' && !actions.isTurn &&
          s.stage !== 'showdown' && s.stage !== 'complete')}
        actionDeadlineMs={s.action_deadline_unix_ms}
        actionBaseDeadlineMs={s.action_base_deadline_unix_ms}
        timeBankMs={viewerSeat?.time_bank_ms ?? 0}
        voiceCommands={preferences.voiceCommands}
        connected={rt.status === 'connected'}
        pending={rt.pendingAction}
        error={rt.actionError} onDismissErrorAction={rt.clearActionError}/>
      <IdleWarning deadline={s.idle_removal_unix_ms} onKeepSeat={rt.keepSeat}/>
      {viewerSeat && <RealityCheck joinedAt={openSession?.joined_at || tableOpenedAt}
                                   buyIn={openSession?.buyin_amount || viewerSeat.stack_at_hand_start || viewerSeat.stack}
                                   currentStack={viewerSeat.stack} handId={s.hand_id}
                                   handComplete={s.stage === 'complete'} isTurn={actions.isTurn}/>}
      <Chat items={rt.chat}
            onSend={rt.sendChat}
            connected={rt.status === 'connected'}
            viewerId={viewer}
            seats={s.seats}/>
      <TableReactions items={rt.reactions} seats={s.seats} viewerId={viewer}
                      connected={rt.status === 'connected'} onSend={rt.sendReaction}/>
      <BotChallenge required={rt.botChallengeRequired} onTokenAction={rt.submitBotChallenge}/>
      <LastWinners items={tableHands}/>
      <PlayerNoteDialog key={noteOpponent?.player_id || 'closed'} opponent={noteOpponent}
                        existing={noteOpponent ? playerNotesByID[noteOpponent.player_id] : undefined}
                        open={Boolean(noteOpponent)}
                        onOpenChange={open => !open && setNoteOpponent(null)}
                        onSaved={(note: PlayerNote | null) => {
                          if (!noteOpponent) return;
                          queryClient.setQueryData<PlayerNote[]>(['player-notes'], current => {
                            const rest = (current || []).filter(item => item.opponent_id !== noteOpponent.player_id);
                            return note ? [...rest, note] : rest;
                          });
                        }}/>

      <AchievementToast unlock={rt.unlock}/>
      {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
    </main>
  );
}

export default function TablePage() {
  return <TermsGate><Suspense
    fallback={<main className="game-loading"><span className="loader"/></main>}><TableContent/></Suspense></TermsGate>;
}
