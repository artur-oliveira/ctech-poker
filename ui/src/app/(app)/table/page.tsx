'use client';
import Link from 'next/link';
import dynamic from 'next/dynamic';
import {Suspense, useEffect, useMemo, useState} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {ChevronLeft, MessageCircle, Pause, Play, RotateCw, SmilePlus, Wifi} from 'lucide-react';
import {getViewerId} from '@/lib/utils';
import {useTableRealtime} from '@/lib/hooks/useTableRealtime';
import {BuyInPanel} from '@/components/table/BuyInPanel';
import {STAGE_LABELS, TableStage} from '@/components/table/TableStage';
import {ActionBar} from '@/components/table/ActionBar';
import {Chat} from '@/components/table/Chat';
import {IdleWarning} from '@/components/table/IdleWarning';
import {InviteDialog} from '@/components/table/InviteDialog';
import {LeaveDialog} from '@/components/table/LeaveDialog';
import {LastWinners} from '@/components/table/LastWinners';
import {EquityTrainerPanel} from '@/components/table/EquityTrainerPanel';
import {TablePreferencesDialog} from '@/components/table/TablePreferencesDialog';
import {RealityCheck} from '@/components/table/RealityCheck';
import {TodayHighlight} from '@/components/table/TodayHighlight';
import {SessionRecap} from '@/components/table/SessionRecap';
import {TableReactions} from '@/components/table/TableReactions';
import {TableUtilityMenu} from '@/components/table/TableUtilityMenu';
import {BotChallenge} from '@/components/table/BotChallenge';
import {AchievementToast} from '@/components/AchievementToast';
import {TermsGate} from '@/components/TermsGate';
import {Button} from '@/components/ui/button';
import {pushNotification} from '@/lib/notify';
import {updateMe} from '@/lib/api/player';
import {currentReactionPurchase, type ReactionCatalogEntry} from '@/lib/api/reactionPurchases';
import {useTablePreferences} from '@/lib/tablePreferences';
import {useDealerVoice} from '@/lib/hooks/useDealerVoice';
import {useTableRemoval, useTableSession} from '@/lib/hooks/useTableSession';
import {useTableOutcome} from '@/lib/hooks/useTableOutcome';
import {useTableOverlays} from '@/lib/hooks/useTableOverlays';
import {actionState} from '@/lib/tableActions';
import type {PlayerNote} from '@/lib/api/playerNotes';
import {highlightPot, seatParticipated} from '@/lib/tableOutcome';
import {getRelationships} from '@/lib/api/social';
import {SOCIAL_KEYS, suppressedPlayerIds} from '@/lib/social';
import {useSocialActions} from '@/lib/hooks/useSocialActions';
import {PlayerActionsMenu} from '@/components/social/PlayerActionsMenu';
import {type MockScenario, USE_MOCK} from '@/lib/mockConfig';
import {MAX_RECONNECT_ATTEMPTS} from '@aoctech/ws-client';
import {DEFAULT_TURN_TIMEOUT_SECONDS} from '@/lib/gameTiming';
import {isTableReaction, TABLE_REACTIONS} from '@/lib/reactions';
import {WALLET_QUERY_ROOT} from '@/lib/api/wallet';
import {bucketFromParams} from '@/lib/lobbyBuckets';
import {setSoundEffectsEnabled} from '@/lib/sound';

const ROOM_ID = /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/;
const MockControls = USE_MOCK
  ? dynamic(() => import('@/components/table/MockControls').then(module => module.MockControls))
  : () => null;
const RebuyDialog = dynamic(() => import('@/components/table/RebuyDialog').then(module => module.RebuyDialog));
const HandRankingsDialog = dynamic(() => import('@/components/table/HandRankingsDialog')
  .then(module => module.HandRankingsDialog));
const PlayerNoteDialog = dynamic(() => import('@/components/table/PlayerNoteDialog')
  .then(module => module.PlayerNoteDialog));
const ReactionPurchaseDialog = dynamic(() => import('@/components/reactions/ReactionPurchaseDialog')
  .then(module => module.ReactionPurchaseDialog));
const CONNECTION_COPY = {
  connecting: 'Conectando à mesa…',
  reconnecting: 'Reconectando à mesa…',
  disconnected: 'Conexão interrompida. Tentando novamente…',
  error: 'A conexão oscilou. Suas fichas continuam seguras.'
} as const;
// @aoctech/ws-client gives up on its own retry loop after MAX_RECONNECT_ATTEMPTS
// and never schedules another one. Only a fresh token (handled elsewhere) or
// this button's retryNow() tries again. Telling the player "tentando
// novamente" past that point would be a lie.
const RECONNECT_GIVEN_UP_COPY = 'Conexão perdida. Toque para tentar novamente.';

function connectionCopyFor(status: keyof typeof CONNECTION_COPY, attempt: number) {
  if (status === 'disconnected' && attempt > MAX_RECONNECT_ATTEMPTS) return RECONNECT_GIVEN_UP_COPY;
  return CONNECTION_COPY[status];
}

const MOCK_SCENARIOS = new Set<MockScenario>([
  'full_hand', 'heads_up', 'layout_3', 'layout_4', 'layout_5', 'six_max', 'layout_7', 'layout_8', 'nine_max',
  'full_hand_loss', 'full_hand_tie', 'all_in', 'auto_fold',
  'waiting', 'pre_flop', 'flop', 'turn', 'river', 'showdown', 'side_pot',
  'complete', 'complete_loss', 'complete_tie', 'fold_win', 'run_it_twice',
  'winner_cards', 'rabbit_hunt', 'rebuy', 'reality_check',
  'reconnecting', 'action_error', 'timeout'
]);

function TableContent() {
  const router = useRouter();
  const params = useSearchParams(), id = params.get('id') || '', valid = ROOM_ID.test(id);
  // A lobby pick arrives as a bucket instead of a room id: the buy-in
  // ceremony below confirms it with join-or-create, which is what decides
  // the table (#205). Everything past the ceremony still needs a real id.
  const bucket = valid ? null : bucketFromParams(params);
  const inviteCode = params.get('invite') || undefined;
  const requestedScenario = params.get('scenario') as MockScenario | null;
  const scenario: MockScenario = requestedScenario && MOCK_SCENARIOS.has(requestedScenario) ? requestedScenario : 'full_hand';
  const requestedDelay = Number(params.get('delay') || 350);
  const delay = [0, 350, 1200, 9000].includes(requestedDelay) ? requestedDelay : 350;
  const viewer = getViewerId();
  const [tableOpenedAt] = useState(() => Math.floor(Date.now() / 1000));
  const {preferences} = useTablePreferences();
  useEffect(() => {
    setSoundEffectsEnabled(preferences.soundEffects);
    return () => setSoundEffectsEnabled(false);
  }, [preferences.soundEffects]);
  const queryClient = useQueryClient();
  const session = useTableSession(id, valid);
  const {room, seated, profile, playerNotes, reactionCatalog, reactionPurchases, tableHands} = session;
  const [noteOpponent, setNoteOpponent] = useState<{ player_id: string; name?: string } | null>(null);
  const [reactionPurchaseTarget, setReactionPurchaseTarget] = useState<ReactionCatalogEntry | null>(null);
  const [favoritesSaving, setFavoritesSaving] = useState(false);
  const socialActions = useSocialActions();
  // Seated opponents, kept in state (not derived mid-render) because the
  // suppression set has to exist before useTableRealtime is called, while the
  // seat list only exists after its first snapshot.
  const [opponentIds, setOpponentIds] = useState<string[]>([]);
  const {data: relationships = []} = useQuery({
    queryKey: SOCIAL_KEYS.relationships(opponentIds), queryFn: () => getRelationships(opponentIds),
    enabled: opponentIds.length > 0
  });
  // Blocking removes the target's content before the round-trip; a rejected
  // block takes the id back out again.
  const [pendingSuppressed, setPendingSuppressed] = useState<string[]>([]);
  const suppressedKey = [...new Set([...suppressedPlayerIds(relationships), ...pendingSuppressed])].sort().join(',');
  const suppressed = useMemo(() => new Set(suppressedKey ? suppressedKey.split(',') : []), [suppressedKey]);
  const rt = useTableRealtime(valid && seated ? id : '', viewer, inviteCode,
    USE_MOCK ? {scenario, delay} : undefined, suppressed);
  useEffect(() => {
    const ids = (rt.snapshot?.seats ?? []).map(seat => seat.player_id)
      .filter(playerId => playerId && playerId !== viewer).sort();
    // Seat membership is only knowable from the authoritative snapshot; this
    // mirrors it into the query key instead of re-deriving it during render.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setOpponentIds(previous => previous.join(',') === ids.join(',') ? previous : ids);
  }, [rt.snapshot?.seats, viewer]);
  useDealerVoice(rt.announcement, preferences.dealerVoice);
  const {sessionRecap, closeRecap} = useTableRemoval({
    id, removed: rt.removed, terminalError: rt.terminalError,
    sessions: session.sessions, sessionsLoading: session.sessionsLoading
  });
  const {handOutcome, viewerStackBefore, nextHandDurationMs} = useTableOutcome({
    id, viewer, snapshot: rt.snapshot, snapshotAt: rt.snapshotAt
  });
  const overlays = useTableOverlays({connected: rt.status === 'connected', sendReaction: rt.sendReaction});
  const {activeTablePanel, setActiveTablePanel, panelOpenChange, pendingReaction} = overlays;
  if (bucket) return <>
    <BuyInPanel bucket={bucket} onSeatedAction={roomId => {
      queryClient.setQueryData(['seated', roomId], {seated: true, stack: 0});
      router.replace(`/table?id=${encodeURIComponent(roomId)}`);
    }}/>
    {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
  </>;
  if (!valid) return (
    <main className="game-loading">
      <h1 className="sr-only">Mesa de poker</h1>
      <h2>Mesa inválida</h2>
      <p>O identificador precisa ser um código de sala válido.</p>
      <Button render={<Link href="/lobby"/>}>Voltar ao lobby</Button>
    </main>
  );
  if (session.seatedLoading) return (
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
  const actionKey = [s.stage, s.current_player_id, s.board.join(','), viewerSeat?.stack, viewerSeat?.contributed,
    actions.minRaise, actions.maxRaise, actions.raiseStep].join(':');
  // A room's share_code is only ever present for its own creator (the server
  // strips it from every other viewer), so its presence alone gates the
  // invite affordance for private tables; public tables need no code at all.
  const canInvite = room && (room.visibility === 'public' || room.share_code);
  const layoutCapacity = scenario === 'heads_up' ? 2 : scenario === 'six_max' ? 6 :
    scenario === 'nine_max' ? 9 : room?.max_seats;
  const isPaused = viewerSeat?.ready === false || viewerSeat?.state === 'sitting_out';
  const canRevealCards = s.stage === 'complete' && seatParticipated(viewerSeat) &&
    Boolean(viewerSeat?.hole_cards?.some(card => card.toLowerCase() !== 'back')) &&
    !((s.protocol_version ?? 0) < 2 && !s.won_without_showdown && viewerSeat?.state !== 'folded') &&
    [0, 1].some(index => !(viewerSeat?.hole_cards_revealed?.[index] ?? false));
  const inviteUrl = typeof window !== 'undefined' ?
    `${window.location.origin}/table?id=${id}${room?.share_code ? `&invite=${room.share_code}` : ''}` : '';
  const openSession = session.openSession;
  const playerNotesByID = Object.fromEntries(playerNotes.map(note => [note.opponent_id, note]));
  const relationshipsByID = Object.fromEntries(relationships.map(item => [item.player_id, item]));
  return (
    <main className="game" data-table-theme={profile?.table_theme || 'classic'}>
      <h1 className="sr-only">Mesa de poker: {STAGE_LABELS[s.stage] || s.stage.replaceAll('_', ' ')}</h1>
      <div className="game-chrome">
        <header>
          <Link href="/lobby" aria-label="Voltar ao lobby"><ChevronLeft/> <span
            className="header-lobby-label">Lobby</span></Link>
          <div className="header-right">
            <span className={`connection-state ${rt.status}`}>
              <Wifi aria-hidden="true"/>
              <span className="connection-label">{rt.status === 'connected' ? 'Ao vivo' : 'Reconectando'}</span>
            </span>
            <TodayHighlight tableId={id} handId={s.hand_id} handComplete={s.stage === 'complete'}
                            handPot={highlightPot(s)}/>
            <Button type="button" variant="ghost" size="icon" className="table-mobile-quick-action"
                    aria-label="Abrir reações" aria-keyshortcuts="e" aria-pressed={activeTablePanel === 'reactions'}
                    onClick={() => setActiveTablePanel(activeTablePanel === 'reactions' ? null : 'reactions')}>
              <SmilePlus aria-hidden="true"/>
            </Button>
            <Button type="button" variant="ghost" size="icon" className="table-mobile-quick-action"
                    aria-label="Abrir chat" aria-keyshortcuts="t" aria-pressed={activeTablePanel === 'chat'}
                    onClick={() => setActiveTablePanel(activeTablePanel === 'chat' ? null : 'chat')}>
              <MessageCircle aria-hidden="true"/>
            </Button>
            <span className="table-utility-menu-slot">
              <TableUtilityMenu active={activeTablePanel}
                                winnersAvailable={tableHands.length > 0}
                                equityTrainerVisible={room?.currency_mode === 'sandbox' && preferences.equityTrainer}
                                equityTrainerAvailable={!actions.isTurn}
                                inviteAvailable={Boolean(canInvite)}
                                onSelectAction={overlays.selectTableUtility}/>
            </span>
            <span className="table-rankings-standalone"><HandRankingsDialog open={activeTablePanel === 'rankings'}
                                                                            onOpenChangeAction={panelOpenChange('rankings')}/>
            </span>
            <span className="table-preferences-standalone"><TablePreferencesDialog
              open={overlays.preferencesOpen} onOpenChangeAction={overlays.setPreferencesOpen}
              runItTwiceAvailable={Boolean(room?.run_it_twice_enabled)}
                                    runItTwice={Boolean(viewerSeat?.run_it_twice)}
                                    onRunItTwiceChange={rt.setRunItTwice}
                                    onLockedFeltAction={() => router.push('/store#felt')}/></span>
            {canInvite && <span className="table-invite-standalone"><InviteDialog
              url={inviteUrl} roomId={id} open={overlays.inviteOpen}
              onOpenChangeAction={overlays.setInviteOpen}/></span>}
            {viewerSeat && !isPaused &&
                <Button type="button" variant="ghost" size="icon" className="table-pause-action"
                        aria-label="Sentar fora" disabled={rt.readyPending}
                        onClick={() => rt.ready(false)}><Pause/></Button>}
            {viewerSeat && isPaused && viewerSeat.stack > 0 &&
                <Button type="button" variant="ghost" size="icon" className="table-pause-action"
                        aria-label="Voltar a jogar" disabled={rt.readyPending}
                        onClick={() => rt.ready(true)}><Play/></Button>}
            {viewerSeat && isPaused && viewerSeat.stack === 0 && room &&
              s.stage !== 'showdown' && s.stage !== 'complete' &&
                <RebuyDialog roomId={id} room={room} autoRebuy={Boolean(viewerSeat.auto_rebuy)}
                             onRebuyAction={() => rt.ready(true)}/>}
            {/* The actual "left" handling (recap + notification) always
                arrives via the removed-frame effect in useTableRemoval,
                whether this resolves instantly (not dealt in) or only once the
                current hand releases the player — LeaveDialog itself only ever
                fires the request. */}
            <span className="table-exit-slot"><LeaveDialog stack={viewerSeat?.stack || 0}
                         pending={rt.requestExitPending}
                         onRequestExitAction={rt.requestExit}/></span>
          </div>
        </header>
        <div className="sr-only" role="status" aria-live="polite" aria-atomic="true">
          {[rt.announcement, rt.status === 'connected' ? 'Conexão com a mesa restaurada.' : connectionMessage]
            .filter(Boolean).join(' ')}
        </div>
        {/* On phones, this notice renders in-flow right below the header (see
            .game-chrome / .reconnect-notice mobile rules) instead of floating
            fixed over it, since a floating overlay can't reliably avoid the
            header once it wraps to two lines, which used to hide and block
            taps on Sentar fora/Sair da mesa for the whole time this notice
            was up. The next-hand countdown that used to share this slot now
            lives on the felt with the stage label (see StreetProgress in
            TableStage.tsx), since it applies to every viewer whether or not
            this particular connection notice is showing. */}
        {connectionMessage && <div className={`reconnect-notice ${rt.status}`}>
            <span aria-hidden="true"/>
            <p>{connectionMessage}{rt.reconnectAttempt > 1 ? ` Tentativa ${rt.reconnectAttempt}.` : ''}</p>
            <Button type="button" variant="ghost" onClick={rt.retryNow}><RotateCw/> Tentar agora</Button>
        </div>}
      </div>
      {/* `payouts`, not `stage === 'complete'`: a showdown hand shows payouts
          one broadcast earlier, at `stage === 'showdown'`, and only flips to
          `complete` a moment later. Gating on the stage name left a window
          where the outcome had already fired but holdOpen was still false,
          racing the banner's own exit timer into dismissing it before
          `complete` ever arrived. */}
      <TableStage snapshot={s} viewer={viewer} pot={pot} bigBlind={bigBlind} nowMs={rt.snapshotAt}
                  maxSeats={layoutCapacity} seatLayoutKey={id}
                  turnTimeoutMs={(room?.turn_timeout_seconds || DEFAULT_TURN_TIMEOUT_SECONDS) * 1000}
                  outcome={handOutcome} holdOutcomeOpen={Boolean(s.payouts && Object.keys(s.payouts).length > 0)}
                  nextHandDeadlineMs={!connectionMessage ? s.next_hand_unix_ms : undefined}
                  nextHandDurationMs={nextHandDurationMs}
                  viewerStackBefore={(s.payouts && Object.keys(s.payouts).length > 0) ? viewerStackBefore : undefined}
                  canRevealCards={canRevealCards} revealPending={rt.showCardsPending}
                  onRevealCardAction={index => rt.showCards(index)}
                  onPeekCardsAction={rt.peekCards}
                  rabbitHuntPending={rt.requestRabbitHuntPending}
                  rabbitHuntFailCount={rt.requestRabbitHuntFailCount}
                  onRequestRabbitHuntAction={rt.requestRabbitHunt}
                  onRabbitHuntVerifyFailedAction={rt.reportRabbitHuntVerifyFailed}
                  viewerPendingExit={Boolean(viewerSeat?.pending_exit)}
                  onCancelExitAction={rt.cancelExit}
                  winnerCardsPending={rt.requestWinnerCardsPending}
                  onRequestWinnerCardsAction={rt.requestWinnerCards}
                  onAnswerWinnerCardsAction={rt.answerWinnerCards}
                  playerNotes={playerNotesByID}
                  onEditPlayerNoteAction={seat => setNoteOpponent({player_id: seat.player_id, name: seat.name})}
                  renderPlayerActionsAction={seat => <PlayerActionsMenu
                    target={{
                      player_id: seat.player_id, name: seat.name,
                      relationship: relationshipsByID[seat.player_id]?.relationship,
                      muted: relationshipsByID[seat.player_id]?.muted,
                      blocked: relationshipsByID[seat.player_id]?.blocked
                    }}
                    actions={socialActions} surface="table_behavior" tableId={id} handId={s.hand_id}
                    onEditNoteAction={() => setNoteOpponent({player_id: seat.player_id, name: seat.name})}
                    onBlockedAction={blocked => {
                      setPendingSuppressed(previous => blocked
                        ? [...previous, seat.player_id]
                        : previous.filter(playerId => playerId !== seat.player_id));
                      return true;
                    }}/>}
                  targetedReactionLabel={pendingReaction ? TABLE_REACTIONS[pendingReaction].label : undefined}
                  onTargetPlayerAction={pendingReaction ? overlays.sendTargetedReaction : undefined}
                  announcement={rt.announcement}
                  chatBubbles={rt.chatBubbles}/>
      <ActionBar
        onActAction={rt.act}
        {...actions}
        actionKey={actionKey}
        selectionScope={`${s.hand_id || 'waiting'}:${s.stage}`}
        preselection={s.action_preselection || null}
        preselectionAmount={s.action_preselection_amount || 0}
        prospectiveCallAmount={s.prospective_call_amount || 0}
        onPreselectAction={rt.preselectAction}
        supportsCallPreselection={(s.protocol_version ?? 0) >= 8}
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
      {viewerSeat && <RealityCheck joinedAt={USE_MOCK && scenario === 'reality_check'
        ? tableOpenedAt - 2 * 60 * 60
        : openSession?.joined_at || tableOpenedAt}
                                   buyIn={openSession?.buyin_amount || viewerSeat.stack_at_hand_start || viewerSeat.stack}
                                   currentStack={viewerSeat.stack} handId={s.hand_id}
                                   handComplete={s.stage === 'complete'} isTurn={actions.isTurn}/>}
      {sessionRecap && <SessionRecap joinedAt={sessionRecap.joinedAt} buyIn={sessionRecap.buyIn}
                                     finalStack={sessionRecap.finalStack} tableId={id}
                                     mode={room?.currency_mode === 'real' ? 'real' : 'sandbox'}
                                     onCloseAction={closeRecap}/>}
      <Chat items={rt.chat}
            onSendAction={rt.sendChat}
            connected={rt.status === 'connected'}
            viewerId={viewer}
            seats={s.seats}
            open={activeTablePanel === 'chat'}
            onOpenChangeAction={panelOpenChange('chat')}/>
      <TableReactions items={rt.reactions} seats={s.seats} viewerId={viewer}
                      connected={rt.status === 'connected'} coolingDown={overlays.reactionCoolingDown}
                      pendingReaction={pendingReaction} onQuickSendAction={overlays.sendQuickReaction}
                      onPendingReactionChangeAction={overlays.setPendingReaction}
                      premiumEnabled premiumLoading={session.reactionCatalogLoading || session.reactionPurchasesLoading}
                      catalog={reactionCatalog} purchases={reactionPurchases}
                      favorites={(profile?.favorite_reactions || []).filter(isTableReaction)}
                      favoritesSaving={favoritesSaving}
                      onLockedReactionAction={entry => {
                        setActiveTablePanel(null);
                        setReactionPurchaseTarget(entry);
                      }}
                      onFavoriteReactionsChangeAction={async favorites => {
                        setFavoritesSaving(true);
                        try {
                          const updated = await updateMe({favorite_reactions: favorites});
                          queryClient.setQueryData(['player', 'me'], updated);
                          pushNotification('Atalhos de reação atualizados.', 'info');
                        } finally {
                          setFavoritesSaving(false);
                        }
                      }}
                      open={activeTablePanel === 'reactions'}
                      onOpenChangeAction={panelOpenChange('reactions')}/>
      <BotChallenge required={rt.botChallengeRequired} onTokenAction={rt.submitBotChallenge}/>
      <LastWinners items={tableHands} tableId={id} open={activeTablePanel === 'winners'}
                   onOpenChangeAction={panelOpenChange('winners')}/>
      {viewerSeat && room?.currency_mode === 'sandbox' && preferences.equityTrainer &&
          <EquityTrainerPanel seat={viewerSeat} isViewer board={s.board} stage={s.stage} handId={s.hand_id}
                              handComplete={s.stage === 'complete'} isTurn={actions.isTurn}
                              currencyMode={room?.currency_mode} open={activeTablePanel === 'equity'}
                              onOpenChangeAction={panelOpenChange('equity')}/>}
      <PlayerNoteDialog key={noteOpponent?.player_id || 'closed'} opponent={noteOpponent}
                        existing={noteOpponent ? playerNotesByID[noteOpponent.player_id] : undefined}
                        open={Boolean(noteOpponent)}
                        onOpenChangeAction={open => !open && setNoteOpponent(null)}
                        onSaved={(note: PlayerNote | null) => {
                          if (!noteOpponent) return;
                          queryClient.setQueryData<PlayerNote[]>(['player-notes'], current => {
                            const rest = (current || []).filter(item => item.opponent_id !== noteOpponent.player_id);
                            return note ? [...rest, note] : rest;
                          });
                        }}/>
      <ReactionPurchaseDialog key={reactionPurchaseTarget?.id || 'closed-reaction-purchase'}
                              entry={reactionPurchaseTarget}
                              initialPurchase={reactionPurchaseTarget
                                ? currentReactionPurchase(reactionPurchases, reactionPurchaseTarget.id)
                                : undefined}
                              sandboxBalance={profile?.sandbox_balance}
                              onCloseAction={() => setReactionPurchaseTarget(null)}
                              onConfirmedAction={() => {
                                void queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT});
                              }}/>

      <AchievementToast unlock={rt.unlock} blocked={Boolean(handOutcome)} onConsumed={rt.clearUnlock}/>
      {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
    </main>
  );
}

export default function TablePage() {
  return (
    <TermsGate>
      <Suspense
        fallback={<main className="game-loading"><span className="loader"/></main>}>
        <TableContent/>
      </Suspense>
    </TermsGate>
  );
}
