'use client';
import Link from 'next/link';
import {Suspense} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {ChevronLeft, RotateCw, Wifi} from 'lucide-react';
import {getViewerId, rotateSeats} from '@/lib/utils';
import {useTableRealtime} from '@/lib/hooks/useTableRealtime';
import {getRoom, getSeated} from '@/lib/api/rooms';
import {BuyInPanel} from '@/components/table/BuyInPanel';
import {Seat} from '@/components/table/Seat';
import {Board} from '@/components/table/Board';
import type {ActionAvailability} from '@/components/table/ActionBar';
import {ActionBar} from '@/components/table/ActionBar';
import {Chat} from '@/components/table/Chat';
import {InviteDialog} from '@/components/table/InviteDialog';
import {LeaveDialog} from '@/components/table/LeaveDialog';
import {MockControls} from '@/components/table/MockControls';
import {AchievementToast} from '@/components/AchievementToast';
import {TermsGate} from '@/components/TermsGate';
import {Button} from '@/components/ui/button';
import {pushNotification} from '@/lib/notify';
import type {PokerAction, TableSnapshot} from '@/lib/api/table';
import {type MockScenario, USE_MOCK} from '@/lib/mock';

const ROOM_ID = /^[a-f0-9]{32}$/i;
const CONNECTION_COPY = {
  connecting: 'Conectando à mesa…',
  reconnecting: 'Reconectando à mesa…',
  disconnected: 'Conexão interrompida. Tentando novamente…',
  error: 'A conexão oscilou. Suas fichas continuam seguras.'
} as const;
const STAGE_LABELS: Record<string, string> = {
  waiting_for_players: 'Aguardando jogadores', pre_flop: 'Pré-flop', flop: 'Flop', turn: 'Turn', river: 'River',
  showdown: 'Showdown', complete: 'Mão encerrada'
};
const BETTING_STAGES = new Set(['pre_flop', 'flop', 'turn', 'river']);
const MOCK_SCENARIOS = new Set<MockScenario>(['full_hand', 'waiting', 'pre_flop', 'flop', 'turn', 'river', 'showdown', 'reconnecting', 'action_error', 'timeout']);

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
  const {data: room} = useQuery({queryKey: ['room', id], queryFn: () => getRoom(id), enabled: valid});
  const queryClient = useQueryClient();
  // Buy-in is an explicit ceremony: nothing is debited until the player
  // confirms an amount. The server (not local browser storage) is the
  // source of truth for "is this player already seated" — that is what
  // lets a player return via a new tab, a different browser, or a
  // different device without repeating the ceremony for a seat they
  // already have.
  const {data: seatedStatus, isLoading: seatedLoading} = useQuery({
    queryKey: ['seated', id], queryFn: () => getSeated(id), enabled: valid
  });
  const seated = seatedStatus?.seated ?? false;
  const rt = useTableRealtime(valid && seated ? id : '', viewer, inviteCode, USE_MOCK ? {scenario, delay} : undefined);
  if (!valid) return (
    <main className="game-loading">
      <h2>Mesa inválida</h2>
      <p>O identificador precisa ser um código de sala válido.</p>
      <Button render={<Link href="/lobby"/>}>Voltar ao lobby</Button>
    </main>
  );
  if (seatedLoading) return <main className="game-loading"><span className="loader"/></main>;
  if (!seated) return <>
    <BuyInPanel roomId={id} shareCode={inviteCode} onSeatedAction={() => {
      queryClient.setQueryData(['seated', id], {seated: true, stack: 0});
    }}/>
    {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
  </>;
  if (!rt.snapshot) return <>
    <main className="game-loading"><span className="loader"/>
      <h2>{rt.status === 'connected' ? 'Aquecendo o seu lugar…' : 'Conectando à mesa…'}</h2>
      <p role="status"
         aria-live="polite">{rt.status === 'connected' ? 'Sincronizando o estado mais recente.' : CONNECTION_COPY[rt.status]}</p>
      {rt.status === 'connected' ? <Button onClick={() => rt.ready()}>Estou pronto</Button> :
        <Button variant="outline" onClick={rt.retryNow}><RotateCw/> Tentar agora</Button>}
    </main>
    {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
  </>;
  const s = rt.snapshot, pot = s.seats.reduce((n, x) => n + x.contributed, 0);
  const connectionMessage = rt.status === 'connected' ? null : CONNECTION_COPY[rt.status];
  const actions = actionState(s, viewer);
  const viewerSeat = s.seats.find(seat => seat.player_id === viewer);
  const actionKey = [s.stage, s.current_player_id, s.board.join(','), viewerSeat?.stack, viewerSeat?.contributed,
    actions.minRaise, actions.maxRaise, actions.raiseStep].join(':');
  // A room's share_code is only ever present for its own creator (the server
  // strips it from every other viewer) — so its presence alone gates the
  // invite affordance for private tables; public tables need no code at all.
  const canInvite = room && (room.visibility === 'public' || room.share_code);
  const inviteUrl = typeof window !== 'undefined' ?
    `${window.location.origin}/table?id=${id}${room?.share_code ? `&invite=${room.share_code}` : ''}` : '';
  return (
    <main className="game">
      <header>
        <Link
          href="/lobby"><ChevronLeft/> Lobby
        </Link>
        <span>{STAGE_LABELS[s.stage] || s.stage.replaceAll('_', ' ')}</span>
        <div className="header-right">
          <span className={`connection-state ${rt.status}`}>
            <Wifi aria-hidden="true"/> {rt.status === 'connected' ? 'Ao vivo' : 'Reconectando'}
          </span>
          {canInvite && <InviteDialog url={inviteUrl}/>}
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
      {!connectionMessage && (s.stage === 'waiting_for_players' || s.stage === 'complete') && <div className="reconnect-notice">
          <p>{s.stage === 'complete' ? 'Mão encerrada. Confirme quando estiver pronto para a próxima.' : 'Aguardando jogadores. Confirme quando estiver pronto.'}</p>
          <Button type="button" variant="ghost" onClick={() => rt.ready()}>Estou pronto</Button>
      </div>}
      <div className="game-table">
        <div className="game-rail"/>
        <div className="game-felt"><Board cards={s.board} pot={pot} rake={s.rake}/></div>
        {rotateSeats(s.seats, viewer).map((seat, i) => <Seat key={seat.player_id} seat={seat} index={i}
                                                             isTurn={s.current_player_id === seat.player_id}
                                                             payout={s.payouts?.[seat.player_id] || 0}
                                                             isViewer={seat.player_id === viewer}/>)}</div>
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
