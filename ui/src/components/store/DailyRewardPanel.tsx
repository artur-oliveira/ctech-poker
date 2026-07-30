'use client';
import {useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Gift} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {getCooldown, spin} from '@/lib/api/dailyReward';
import {pushNotification} from '@/lib/notify';
import {formatDuration, useCountdownMs} from './useCountdown';

function deadlineFromSeconds(seconds: number): number | null {
  return seconds > 0 ? Date.now() + seconds * 1000 : null;
}

export function DailyRewardPanel() {
  const queryClient = useQueryClient();
  const cooldown = useQuery({queryKey: ['dailyReward', 'cooldown'], queryFn: getCooldown});
  const [deadline, setDeadline] = useState<number | null>(null);
  // Tracks which server-reported cooldown the local deadline was last derived
  // from, so a fresh GET result resets the countdown exactly once (during
  // render, not in an effect — avoids an extra cascading render per fetch).
  const [syncedFrom, setSyncedFrom] = useState<number | undefined>(undefined);
  const [spinning, setSpinning] = useState(false);
  const [wonAmount, setWonAmount] = useState<number | null>(null);

  if (cooldown.data && cooldown.data.remaining_time_seconds !== syncedFrom) {
    setSyncedFrom(cooldown.data.remaining_time_seconds);
    setDeadline(deadlineFromSeconds(cooldown.data.remaining_time_seconds));
  }

  const remainingMs = useCountdownMs(deadline);
  const ready = deadline === null || remainingMs <= 0;

  async function claim() {
    setSpinning(true);
    try {
      const result = await spin();
      setSyncedFrom(result.remaining_time_seconds);
      setDeadline(deadlineFromSeconds(result.remaining_time_seconds));
      void queryClient.invalidateQueries({queryKey: ['wallet', 'balance']});
      void queryClient.invalidateQueries({queryKey: ['player', 'me']});
      if (result.amount > 0) {
        setWonAmount(result.amount);
        pushNotification(`Você recebeu ${result.amount.toLocaleString('pt-BR')} fichas sandbox!`, 'info');
      } else {
        pushNotification('Recompensa diária ainda não disponível.', 'info');
      }
    } catch {
      pushNotification('Não foi possível resgatar sua recompensa agora. Tente novamente em instantes.');
    } finally {
      setSpinning(false);
    }
  }

  return <div className="store-reward">
    <span className="store-reward-icon"><Gift aria-hidden="true"/></span>
    <h2>Recompensa diária</h2>
    <p>Resgate fichas sandbox grátis uma vez por dia. O valor varia a cada resgate.</p>
    {wonAmount !== null && <p className="store-reward-result">+{wonAmount.toLocaleString('pt-BR')} fichas</p>}
    {ready
      ? <Button type="button" disabled={spinning || cooldown.isLoading} onClick={() => void claim()}>
        {spinning ? 'Resgatando…' : 'Resgatar recompensa'}
      </Button>
      : <>
        <p className="store-reward-countdown">Próxima recompensa em {formatDuration(remainingMs)}</p>
        <Button type="button" disabled>Resgatar recompensa</Button>
      </>}
  </div>;
}
