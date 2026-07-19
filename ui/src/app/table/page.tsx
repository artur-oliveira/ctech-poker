'use client'
import Link from 'next/link';
import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {ChevronLeft, Wifi} from 'lucide-react';
import {decodeIdToken} from '@/lib/auth/oauth';
import {getAccessToken} from '@/lib/api/client';
import {useTableRealtime} from '@/lib/hooks/useTableRealtime';
import {Seat} from '@/components/table/Seat';
import {Board} from '@/components/table/Board';
import {ActionBar} from '@/components/table/ActionBar';
import {Chat} from '@/components/table/Chat';
import {AchievementToast} from '@/components/AchievementToast'
import {TermsGate} from '@/components/TermsGate'
import {Button} from '@/components/ui/button'

const ROOM_ID = /^[a-f0-9]{32}$/i

function TableContent() {
  const params = useSearchParams(), id = params.get('id') || '', valid = ROOM_ID.test(id);
  const rt = useTableRealtime(valid ? id : '');
  const token = getAccessToken();
  const viewer = token ? (decodeIdToken(token) as { sub?: string } | null)?.sub : undefined;
  if (!valid) return (
    <main className="game-loading">
      <h2>Mesa inválida</h2>
      <p>O identificador precisa ser um código de sala válido.</p>
      <Button render={<Link href="/lobby"/>}>Voltar ao lobby</Button>
    </main>
  );
  if (!rt.snapshot) return (
    <main className="game-loading"><span className="loader"/><h2>
      Aquecendo o seu lugar…
    </h2>
      <Button onClick={() => rt.ready()}>Estou pronto</Button>
    </main>
  )
  const s = rt.snapshot, pot = s.seats.reduce((n, x) => n + x.contributed, 0);
  return (
    <main className="game">
      <header>
        <Link
          href="/lobby"><ChevronLeft/> Lobby
        </Link>
        <span>{s.stage.replaceAll('_', ' ')}</span>
        <i>
          <Wifi/> {rt.status}
        </i>
      </header>
      <div className="game-table">
        <div className="game-rail"/>
        <div className="game-felt"><Board cards={s.board} pot={pot} rake={s.rake}/></div>
        {s.seats.map((seat, i) => <Seat key={seat.player_id} seat={seat} index={i}
                                        isViewer={seat.player_id === viewer}/>)}</div>
      <ActionBar onAct={rt.act}/><Chat items={rt.chat} onSend={rt.sendChat}/><AchievementToast unlock={rt.unlock}/>
    </main>
  )
}

export default function TablePage() {
  return <TermsGate><Suspense
    fallback={<main className="game-loading"><span className="loader"/></main>}><TableContent/></Suspense></TermsGate>
}
