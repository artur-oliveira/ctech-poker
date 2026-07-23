'use client';
import Link from 'next/link';
import {Club, Gift, LoaderCircle, Trophy} from 'lucide-react';
import {StakesGrid} from '@/components/lobby/StakesGrid';
import {CreateRoomDialog} from '@/components/lobby/CreateRoomDialog';
import {ProfileMenu} from '@/components/lobby/ProfileMenu';
import {MockControls} from '@/components/table/MockControls';
import {TermsGate} from '@/components/TermsGate';
import {useEffect, useState} from 'react';
import {useQueryClient} from '@tanstack/react-query';
import {remainingTime, spin} from '@/lib/api/gamification';
import {pushNotification} from '@/lib/notify';
import {USE_MOCK} from '@/lib/mock';
import {Button} from "@/components/ui/button";

function formatCooldown(seconds: number) {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}min`;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

export default function Lobby() {
  const queryClient = useQueryClient();
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
      if (r.amount > 0) {
        pushNotification(`Você ganhou +${r.amount.toLocaleString('pt-BR')} fichas sandbox!`, 'info');
        // Reward always credits the sandbox ledger (walletclient.Credit is
        // sandbox-only server-side) — refetch so the header's balance pill
        // picks up the new sandbox_balance instead of showing stale data.
        void queryClient.invalidateQueries({queryKey: ['player', 'me']});
      }
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
        className="brand-mark"><Club/></span>CTech <b>Poker</b></Link>
        <div className="header-right">
          <Link href="/leaderboard"><Trophy/> Ranking</Link>
          <ProfileMenu/>
        </div>
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
              {claiming ? <LoaderCircle size={18} className="action-spinner"/> : <Gift size={18}/>} {claiming ? 'Resgatando…'
                : cooldown ? <>Próxima recompensa <span className="reward-timer">{formatCooldown(cooldown)}</span></>
                  : 'Recompensa Diária'}
            </Button>
            <CreateRoomDialog/>
          </div>
        </header>
        <StakesGrid/></section>
      {USE_MOCK && <MockControls/>}
    </main>
  </TermsGate>;
}
