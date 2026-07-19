import Link from 'next/link';
import {Club, Trophy} from 'lucide-react';
import {RoomList} from '@/components/lobby/RoomList';
import {CreateRoomDialog} from '@/components/lobby/CreateRoomDialog';import {TermsGate} from '@/components/TermsGate'

export default function Lobby() {
  return <TermsGate><main className="app-page">
    <nav className="app-nav shell"><Link href="/" className="brand"><span
      className="brand-mark"><Club/></span>CTech <b>Poker</b></Link><Link href="/leaderboard"><Trophy/> Ranking</Link>
    </nav>
    <section className="lobby shell">
      <header>
        <div><small>LOBBY SANDBOX</small><h1>Escolha sua mesa.</h1><p>Fichas virtuais, emoção de verdade. Nenhum símbolo
          monetário por aqui.</p></div>
        <CreateRoomDialog/></header>
      <RoomList/></section>
  </main></TermsGate>
}
