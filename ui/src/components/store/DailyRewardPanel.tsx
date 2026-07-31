'use client';
import {useState} from 'react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Coins, Gift, RotateCcw} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {getCooldown, spin} from '@/lib/api/dailyReward';
import {pushNotification} from '@/lib/notify';
import {formatDuration, useCountdownMs} from './useCountdown';

function deadlineFromSeconds(seconds: number): number | null {
  return seconds > 0 ? Date.now() + seconds * 1000 : null;
}

export function DailyRewardPanel() {
  const queryClient = useQueryClient();
  const cooldown = useQuery({queryKey: ['dailyReward', 'cooldown'], queryFn: getCooldown, retry: 1});
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
      queryClient.setQueryData(['dailyReward', 'cooldown'], {
        remaining_time_seconds: result.remaining_time_seconds,
      });
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

  if (cooldown.isError) {
    return <div className="store-reward store-reward-error">
      <span className="store-reward-icon"><Gift aria-hidden="true"/></span>
      <h2>A recompensa ficou fora da mesa</h2>
      <p>Não conseguimos consultar seu resgate agora. Suas fichas continuam seguras.</p>
      <Button type="button" variant="outline" onClick={() => void cooldown.refetch()}>
        <RotateCcw aria-hidden="true"/> Tentar novamente
      </Button>
    </div>;
  }

  return <div className={`store-reward${wonAmount !== null ? ' is-claimed' : ''}`}>
    <span className="store-reward-icon"><Gift aria-hidden="true"/></span>
    <div className="store-reward-copy">
      <h2>Fichas por conta da casa</h2>
      <p>Uma recompensa sandbox grátis por dia. O valor é revelado no resgate.</p>
    </div>
    {wonAmount !== null && <div className="store-reward-result" role="status" aria-live="polite">
      <Coins aria-hidden="true"/>
      <span><strong>+{wonAmount.toLocaleString('pt-BR')} fichas</strong>Seu saldo já foi atualizado.</span>
    </div>}
    {ready
      ? <Button type="button" disabled={spinning || cooldown.isLoading} onClick={() => void claim()}>
        {spinning ? 'Revelando…' : 'Resgatar fichas grátis'}
      </Button>
      : <>
        <p className="store-reward-countdown">Volte em <strong>{formatDuration(remainingMs)}</strong></p>
        <Button type="button" disabled>Recompensa já resgatada</Button>
      </>}
  </div>;
}
