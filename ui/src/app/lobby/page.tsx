'use client'
import Link from 'next/link';
import {Club, Gift, Trophy} from 'lucide-react';
import {RoomList} from '@/components/lobby/RoomList';
import {CreateRoomDialog} from '@/components/lobby/CreateRoomDialog';
import {TermsGate} from '@/components/TermsGate'
import {useState} from 'react';
import {spin} from '@/lib/api/gamification';
import {pushNotification} from '@/lib/notify';
import {Button} from "@/components/ui/button";

export default function Lobby() {
  const [claiming, setClaiming] = useState(false);
  
  async function claimReward() {
    setClaiming(true);
    try {
      const r = await spin();
      pushNotification(`Você ganhou +${r.amount} fichas sandbox!`, 'info');
    } catch (e) {
      pushNotification('Não foi possível resgatar a recompensa agora.', 'error');
    } finally {
      setClaiming(false);
    }
  }
  
  return <TermsGate>
    <main className="app-page">
      <nav className="app-nav shell"><Link href="/" className="brand"><span
        className="brand-mark"><Club/></span>CTech <b>Poker</b></Link><Link href="/leaderboard"><Trophy/> Ranking</Link>
      </nav>
      <section className="lobby shell">
        <header>
          <div><small>LOBBY SANDBOX</small><h1>Escolha sua mesa.</h1><p>Fichas virtuais, emoção de verdade. Nenhum
            símbolo
            monetário por aqui.</p></div>
          <div className="lobby-actions">
            <Button variant="outline" disabled={claiming} onClick={claimReward} className="btn-reward">
              <Gift size={18}/> {claiming ? 'Resgatando...' : 'Recompensa Diária'}
            </Button>
            <CreateRoomDialog/>
          </div>
        </header>
        <RoomList/></section>
    </main>
  </TermsGate>
}
