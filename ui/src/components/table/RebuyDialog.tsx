'use client';
import {useEffect, useId, useState} from 'react';
import {Wallet} from 'lucide-react';
import {useQuery} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
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
import {createPurchase, listSkus, type SandboxPurchase, type SandboxSKU} from '@/lib/api/wallet';
import {SkuGrid} from '@/components/store/SkuGrid';
import {PixPaymentView} from '@/components/store/PixPaymentView';
import {formatBuyIn, midBuyIn} from './BuyInPanel';

const GENERIC_ERROR = 'Não foi possível comprar mais fichas agora. Tente novamente.';
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
  const [open, setOpen] = useState(true);
  const [amount, setAmount] = useState<number | null>(null);
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState('');
  const [graceElapsed, setGraceElapsed] = useState(!autoRebuy);
  const [pixPurchase, setPixPurchase] = useState<SandboxPurchase | null>(null);

  useEffect(() => {
    if (!autoRebuy) return undefined;
    const id = window.setTimeout(() => setGraceElapsed(true), AUTO_REBUY_GRACE_MS);
    return () => window.clearTimeout(id);
  }, [autoRebuy]);

  const player = useQuery({queryKey: ['player', 'me'], queryFn: getMe, enabled: graceElapsed});
  const balanceIsZero = player.data?.sandbox_balance === 0;
  const skus = useQuery({
    queryKey: ['wallet', 'skus'], queryFn: listSkus, enabled: graceElapsed && balanceIsZero,
  });

  const step = room.big_blind > 0 ? room.big_blind : 1;
  const value = amount ?? midBuyIn(room.buy_in_min, room.buy_in_max, room.big_blind);
  const isReal = room.currency_mode === 'real';
  const unit = isReal ? 'reais' : 'fichas';
  const fmt = (n: number) => formatBuyIn(n, isReal);

  async function confirm() {
    setJoining(true);
    setError('');
    try {
      await joinRoom(roomId, value);
      setOpen(false);
      onRebuyAction();
    } catch {
      setError(GENERIC_ERROR);
      setJoining(false);
    }
  }

  async function selectSku(sku: SandboxSKU) {
    setError('');
    try {
      setPixPurchase(await createPurchase(sku.id));
    } catch {
      setError(GENERIC_ERROR);
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
        <DialogDescription>{balanceIsZero && !isReal
          ? 'Seu saldo sandbox zerou. Compre mais fichas com Pix para continuar nesta mesa.'
          : `Compre mais ${unit} para continuar jogando nesta mesa.`}</DialogDescription>
      </DialogHeader>
      {isReal && !!room.entry_fee_cents &&
          <p className="buyin-fee-notice">Taxa fixa de mesa: {formatBuyIn(room.entry_fee_cents, true)} (cobrada
              de novo a cada vez que você compra fichas).</p>}
      {error && <p className="buyin-error" role="alert">{error}</p>}
      {balanceIsZero && !isReal
        ? pixPurchase
          ? <PixPaymentView purchase={pixPurchase}/>
          : <SkuGrid skus={skus.data ?? []} isLoading={skus.isLoading} isError={skus.isError}
                     onRetryAction={() => void skus.refetch()} onSelectAction={sku => void selectSku(sku)}
                     pendingSku={null}/>
        : <>
          <div className="buyin-control">
            <label htmlFor={sliderId}>Recompra</label>
            <input id={sliderId} type="range" min={room.buy_in_min} max={room.buy_in_max} step={step} value={value}
                   disabled={joining} onChange={event => setAmount(Number(event.target.value))}
                   aria-valuetext={`${fmt(value)} ${unit}`}/>
            <output htmlFor={sliderId}>{fmt(value)} <span>{unit}</span></output>
            <small>mín. {fmt(room.buy_in_min)} · máx. {fmt(room.buy_in_max)}</small>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" disabled={joining} onClick={() => setOpen(false)}>Agora não</Button>
            <Button type="button" disabled={joining} onClick={confirm}>
              {joining ? 'Comprando…' : `Comprar ${fmt(value)}`}
            </Button>
          </DialogFooter>
        </>}
    </DialogContent>
  </Dialog>;
}
