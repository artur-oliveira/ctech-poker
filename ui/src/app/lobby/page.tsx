'use client';
import Link from 'next/link';
import {Club, Gift, Trophy} from 'lucide-react';
import {RoomList} from '@/components/lobby/RoomList';
import {CreateRoomDialog} from '@/components/lobby/CreateRoomDialog';
import {TermsGate} from '@/components/TermsGate';
import {useEffect, useState} from 'react';
import {remainingTime, spin} from '@/lib/api/gamification';
import {pushNotification} from '@/lib/notify';
import {Button} from "@/components/ui/button";

function formatCooldown(seconds: number) {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}min`;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

export default function Lobby() {
  const [claiming, setClaiming] = useState(false);
  // null = cooldown still unknown (loading); 0 = claimable
  const [cooldown, setCooldown] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    remainingTime()
      .then(r => !cancelled && setCooldown(r.remaining_time_seconds))
      .catch(() => !cancelled && setCooldown(0));
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!cooldown) return () => {
    };
    const timer = setTimeout(() => setCooldown(cooldown - 1), 1000);
    return () => clearTimeout(timer);
  }, [cooldown]);

  async function claimReward() {
    setClaiming(true);
    try {
      const r = await spin();
      setCooldown(r.remaining_time_seconds);
      if (r.amount > 0) pushNotification(`Você ganhou +${r.amount} fichas sandbox!`, 'info');
      else pushNotification(`Recompensa disponível em ${formatCooldown(r.remaining_time_seconds)}.`, 'info');
    } catch {
      pushNotification('Não foi possível resgatar a recompensa agora.', 'error');
    } finally {
      setClaiming(false);
    }
  }

  const onCooldown = cooldown === null || cooldown > 0;
  
  return <TermsGate>
    <main className="app-page">
      <nav className="app-nav shell"><Link href="/" className="brand"><span
        className="brand-mark"><Club/></span>CTech <b>Poker</b></Link><Link href="/leaderboard"><Trophy/> Ranking</Link>
      </nav>
      <section className="lobby shell">
        <header>
          <div>
            <small>LOBBY SANDBOX</small>
            <h1>Escolha sua mesa.</h1>
            <p>
              Fichas virtuais, emoção de verdade.
            </p>
          </div>
          <div className="lobby-actions">
            <Button variant="outline" size="lg" disabled={claiming || onCooldown} onClick={claimReward}
                    className="btn-reward">
              <Gift size={18}/> {claiming ? 'Resgatando…'
                : cooldown ? <>Próxima recompensa <span className="reward-timer">{formatCooldown(cooldown)}</span></>
                  : 'Recompensa Diária'}
            </Button>
            <CreateRoomDialog/>
          </div>
        </header>
        <RoomList/></section>
    </main>
  </TermsGate>;
}
