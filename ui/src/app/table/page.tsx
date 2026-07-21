'use client'
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {ChevronLeft, RotateCw, Wifi} from 'lucide-react';
import {decodeIdToken} from '@/lib/auth/oauth';
import {getAccessToken} from '@/lib/api/client';
import {useTableRealtime} from '@/lib/hooks/useTableRealtime';
import {Seat} from '@/components/table/Seat';
import {Board} from '@/components/table/Board';
import type {ActionAvailability} from '@/components/table/ActionBar'
import {ActionBar} from '@/components/table/ActionBar';
import {Chat} from '@/components/table/Chat';
import {MockControls} from '@/components/table/MockControls';
import {AchievementToast} from '@/components/AchievementToast'
import {TermsGate} from '@/components/TermsGate'
import {Button} from '@/components/ui/button'
import type {PokerAction, TableSnapshot} from '@/lib/api/table'
import {MOCK_PLAYER_ID, type MockScenario, USE_MOCK} from '@/lib/mock'

const ROOM_ID = /^[a-f0-9]{32}$/i
const CONNECTION_COPY = {
  connecting: 'Conectando à mesa…',
  reconnecting: 'Reconectando à mesa…',
  disconnected: 'Conexão interrompida. Tentando novamente…',
  error: 'A conexão oscilou. Suas fichas continuam seguras.'
} as const
const STAGE_LABELS: Record<string, string> = {
  waiting_for_players: 'Aguardando jogadores', pre_flop: 'Pré-flop', flop: 'Flop', turn: 'Turn', river: 'River',
  showdown: 'Showdown', complete: 'Mão encerrada'
}
const BETTING_STAGES = new Set(['pre_flop', 'flop', 'turn', 'river'])
const MOCK_SCENARIOS = new Set<MockScenario>(['full_hand', 'waiting', 'pre_flop', 'flop', 'turn', 'river', 'showdown', 'reconnecting', 'action_error', 'timeout'])

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
  return {available, callAmount, isTurn, minRaise, maxRaise, raiseStep: serverActions?.step || 25}
}

function TableContent() {
  const params = useSearchParams(), id = params.get('id') || '', valid = ROOM_ID.test(id);
  const requestedScenario = params.get('scenario') as MockScenario | null;
  const scenario: MockScenario = requestedScenario && MOCK_SCENARIOS.has(requestedScenario) ? requestedScenario : 'full_hand';
  const requestedDelay = Number(params.get('delay') || 350);
  const delay = [0, 350, 1200, 9000].includes(requestedDelay) ? requestedDelay : 350;
  const token = getAccessToken();
  const viewer = USE_MOCK ? MOCK_PLAYER_ID : token ? (decodeIdToken(token) as { sub?: string } | null)?.sub : undefined;
  const rt = useTableRealtime(valid ? id : '', viewer, USE_MOCK ? {scenario, delay} : undefined);
  if (!valid) return (
    <main className="game-loading">
      <h2>Mesa inválida</h2>
      <p>O identificador precisa ser um código de sala válido.</p>
      <Button render={<Link href="/lobby"/>}>Voltar ao lobby</Button>
    </main>
  );
  if (!rt.snapshot) return <>
    <main className="game-loading"><span className="loader"/>
      <h2>{rt.status === 'connected' ? 'Aquecendo o seu lugar…' : 'Conectando à mesa…'}</h2>
      <p role="status"
         aria-live="polite">{rt.status === 'connected' ? 'Sincronizando o estado mais recente.' : CONNECTION_COPY[rt.status]}</p>
      {rt.status === 'connected' ? <Button onClick={() => rt.ready()}>Estou pronto</Button> :
        <Button variant="outline" onClick={rt.retryNow}><RotateCw/> Tentar agora</Button>}
    </main>
    {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
  </>
  const s = rt.snapshot, pot = s.seats.reduce((n, x) => n + x.contributed, 0);
  const connectionMessage = rt.status === 'connected' ? null : CONNECTION_COPY[rt.status];
  const actions = actionState(s, viewer);
  const viewerSeat = s.seats.find(seat => seat.player_id === viewer);
  const actionKey = [s.stage, s.current_player_id, s.board.join(','), viewerSeat?.stack, viewerSeat?.contributed,
    actions.minRaise, actions.maxRaise, actions.raiseStep].join(':');
  return (
    <main className="game">
      <header>
        <Link
          href="/lobby"><ChevronLeft/> Lobby
        </Link>
        <span>{STAGE_LABELS[s.stage] || s.stage.replaceAll('_', ' ')}</span>
        <span className={`connection-state ${rt.status}`}>
          <Wifi aria-hidden="true"/> {rt.status === 'connected' ? 'Ao vivo' : 'Reconectando'}
        </span>
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
      <div className="game-table">
        <div className="game-rail"/>
        <div className="game-felt"><Board cards={s.board} pot={pot} rake={s.rake}/></div>
        {s.seats.map((seat, i) => <Seat key={seat.player_id} seat={seat} index={i}
                                        isTurn={s.current_player_id === seat.player_id}
                                        payout={s.payouts?.[seat.player_id] || 0}
                                        isViewer={seat.player_id === viewer}/>)}</div>
      <ActionBar onAct={rt.act} {...actions} actionKey={actionKey} connected={rt.status === 'connected'}
                 pending={rt.pendingAction}
                 error={rt.actionError} onDismissError={rt.clearActionError}/>
      <Chat items={rt.chat} onSend={rt.sendChat} connected={rt.status === 'connected'}/><AchievementToast
      unlock={rt.unlock}/>
      {USE_MOCK && <MockControls scenario={scenario} delay={delay}/>}
    </main>
  )
}

export default function TablePage() {
  return <TermsGate><Suspense
    fallback={<main className="game-loading"><span className="loader"/></main>}><TableContent/></Suspense></TermsGate>
}
