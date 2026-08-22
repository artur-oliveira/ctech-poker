'use client';
import {useEffect, useId, useState} from 'react';
import Link from 'next/link';
import {Gift, RefreshCw, Wallet} from 'lucide-react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog';
import type {Room} from '@/lib/api/rooms';
import {joinRoom} from '@/lib/api/rooms';
import {getMe} from '@/lib/api/player';
import {getCooldown, spin} from '@/lib/api/dailyReward';
import {formatBuyIn, midBuyIn} from './BuyInPanel';

const GENERIC_ERROR = 'Não foi possível comprar mais fichas agora. Tente novamente.';
const REWARD_ERROR = 'Não foi possível resgatar a recompensa agora. Tente novamente.';
// ponytail: fixed grace window, not an explicit server "auto-rebuy failed"
// push — good enough for one local wallet HTTP round trip. If wallet latency
// ever gets unpredictable enough to make this flaky, replace with an
// explicit "rebuy_failed" push instead of guessing a timeout.
const AUTO_REBUY_GRACE_MS = 1500;

/** Shown once a seated player's stack hits zero. The "Play" toggle is a dead
 * end at that point, so this offers the same buy-in ceremony as first sitting
 * down instead. Stays reachable via its own trigger if dismissed, since
 * busting doesn't force a decision.
 *
 * There is deliberately no purchase here: busting is not a moment to sell
 * chips. The recovery paths are the balance the player already has and the
 * free daily reward; the store stays a separate, deliberate destination.
 *
 * When the seat opted into auto-rebuy at join time, the server's own
 * auto-rebuy attempt runs asynchronously right after this bust snapshot
 * arrives — so this dialog waits out a short grace window before deciding
 * what to show, instead of immediately flashing a "you're out of chips"
 * dialog that a silent auto-rebuy is about to make moot. */
export function RebuyDialog({roomId, room, autoRebuy = false, onRebuyAction}: {
  roomId: string;
  room: Room;
  autoRebuy?: boolean;
  onRebuyAction: () => void
}) {
  const sliderId = useId();
  const autoRebuyId = useId();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(true);
  const [amount, setAmount] = useState<number | null>(null);
  const [keepAutoRebuy, setKeepAutoRebuy] = useState(autoRebuy);
  const [joining, setJoining] = useState(false);
  const [claiming, setClaiming] = useState(false);
  const [claimedAmount, setClaimedAmount] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [graceElapsed, setGraceElapsed] = useState(!autoRebuy);

  useEffect(() => {
    if (!autoRebuy) return undefined;
    const id = window.setTimeout(() => setGraceElapsed(true), AUTO_REBUY_GRACE_MS);
    return () => window.clearTimeout(id);
  }, [autoRebuy]);

  const isReal = room.currency_mode === 'real';
  const player = useQuery({queryKey: ['player', 'me'], queryFn: getMe, enabled: graceElapsed});
  // The balance that matters is the one this room can be re-entered with, not
  // "is it exactly zero": a player with 40 chips and a 100-chip minimum cannot
  // rebuy either.
  const balance = (isReal ? player.data?.game_balance : player.data?.sandbox_balance) ?? 0;
  const canAffordBuyIn = balance >= room.buy_in_min;
  const reward = useQuery({
    queryKey: ['dailyReward', 'cooldown'], queryFn: getCooldown,
    enabled: graceElapsed && !isReal && !canAffordBuyIn
  });
  const rewardReady = reward.data?.remaining_time_seconds === 0;

  const step = room.big_blind > 0 ? room.big_blind : 1;
  const max = Math.min(room.buy_in_max, Math.max(room.buy_in_min, balance));
  const value = Math.min(max, amount ?? midBuyIn(room.buy_in_min, room.buy_in_max, room.big_blind));
  const unit = isReal ? 'reais' : 'fichas';
  const fmt = (n: number) => formatBuyIn(n, isReal);

  async function confirm() {
    setJoining(true);
    setError('');
    try {
      await joinRoom(roomId, value, undefined, keepAutoRebuy);
      setOpen(false);
      onRebuyAction();
    } catch {
      setError(GENERIC_ERROR);
      setJoining(false);
    }
  }

  async function claimReward() {
    setClaiming(true);
    setError('');
    try {
      const result = await spin();
      setClaimedAmount(result.amount);
      queryClient.setQueryData(['dailyReward', 'cooldown'],
        {remaining_time_seconds: result.remaining_time_seconds});
      // The new balance decides whether a rebuy is now possible at all, so the
      // profile is re-read instead of being patched locally.
      await queryClient.invalidateQueries({queryKey: ['player', 'me']});
    } catch {
      setError(REWARD_ERROR);
    } finally {
      setClaiming(false);
    }
  }

  if (!graceElapsed) return null;

  return <Dialog open={open} onOpenChange={next => {
    setOpen(next);
    if (!next) setError('');
  }}>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Comprar mais fichas"/>}>
      <Wallet/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Você ficou sem fichas</DialogTitle>
        <DialogDescription>{canAffordBuyIn
          ? `Compre mais ${unit} para continuar jogando nesta mesa.`
          : isReal
            ? 'Seu saldo disponível não cobre o buy-in mínimo desta mesa.'
            : 'Seu saldo sandbox não cobre o buy-in mínimo desta mesa.'}</DialogDescription>
      </DialogHeader>
      {isReal && !!room.entry_fee_cents &&
          <p className="buyin-fee-notice">Taxa fixa de mesa: {formatBuyIn(room.entry_fee_cents, true)} (cobrada
              de novo a cada vez que você compra fichas).</p>}
      {error && <p className="buyin-error" role="alert">{error}</p>}
      {claimedAmount !== null && <p className="buyin-reward-result" role="status">
        Você resgatou {claimedAmount.toLocaleString('pt-BR')} fichas grátis.
      </p>}
      {canAffordBuyIn ? <>
        <div className="buyin-control">
          <label htmlFor={sliderId}>Recompra</label>
          <input id={sliderId} type="range" min={room.buy_in_min} max={max} step={step} value={value}
                 disabled={joining} onChange={event => setAmount(Number(event.target.value))}
                 aria-valuetext={`${fmt(value)} ${unit}`}/>
          <output htmlFor={sliderId}>{fmt(value)} <span>{unit}</span></output>
          <small>mín. {fmt(room.buy_in_min)} · máx. {fmt(max)}</small>
        </div>
        {!isReal &&
            <div className="buyin-control table-preference-toggle">
                <span><RefreshCw aria-hidden="true"/><span>
              <Label id={`${autoRebuyId}-label`} htmlFor={autoRebuyId}>Auto rebuy</Label>
            <small>Se suas fichas acabarem de novo, compramos automaticamente o mesmo valor para você continuar
                jogando sem esperar.</small>
          </span>
          </span>
                <Switch id={autoRebuyId} aria-labelledby={`${autoRebuyId}-label`} checked={keepAutoRebuy}
                        disabled={joining} onCheckedChange={setKeepAutoRebuy}/>
            </div>}
        <DialogFooter>
          <Button type="button" variant="ghost" disabled={joining} onClick={() => setOpen(false)}>Agora não</Button>
          <Button type="button" disabled={joining} onClick={confirm}>
            {joining ? 'Comprando…' : `Comprar ${fmt(value)}`}
          </Button>
        </DialogFooter>
      </> : <>
        <p className="buyin-hint">{isReal
          ? 'Você pode voltar ao lobby e escolher uma mesa com buy-in menor. A carteira fica em uma rota separada, fora do jogo.'
          : rewardReady
            ? 'Resgate suas fichas grátis do dia para tentar continuar nesta mesa.'
            : 'A recompensa diária ainda está em contagem. Sem pressa: você pode voltar ao lobby e escolher uma mesa com buy-in menor.'}</p>
        <DialogFooter>
          <Button variant="ghost" render={<Link href="/lobby"/>}>Voltar ao lobby</Button>
          {!isReal && rewardReady && <Button type="button" disabled={claiming} onClick={claimReward}>
            <Gift aria-hidden="true"/> {claiming ? 'Resgatando…' : 'Resgatar fichas grátis'}
          </Button>}
        </DialogFooter>
      </>}
    </DialogContent>
  </Dialog>;
}
