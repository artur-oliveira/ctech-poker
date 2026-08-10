'use client';
import {useId, useState} from 'react';
import {Wallet} from 'lucide-react';
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
import {formatBuyIn, midBuyIn} from './BuyInPanel';

const GENERIC_ERROR = 'Não foi possível comprar mais fichas agora. Tente novamente.';

/** Shown once a seated player's stack hits zero. The "Play" toggle is a dead
 * end at that point, so this offers the same buy-in ceremony as first sitting
 * down instead. Stays reachable via its own trigger if dismissed, since
 * busting doesn't force a decision. */
export function RebuyDialog({roomId, room, onRebuyAction}: {
  roomId: string;
  room: Room;
  onRebuyAction: () => void
}) {
  const sliderId = useId();
  const [open, setOpen] = useState(true);
  const [amount, setAmount] = useState<number | null>(null);
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState('');
  
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
  
  return <Dialog open={open} onOpenChangeAction={next => {
    setOpen(next);
    if (!next) setError('');
  }}>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Comprar mais fichas"/>}>
      <Wallet/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Você ficou sem fichas</DialogTitle>
        <DialogDescription>Compre mais {unit} para continuar jogando nesta mesa.</DialogDescription>
      </DialogHeader>
      {isReal && !!room.entry_fee_cents &&
          <p className="buyin-fee-notice">Taxa fixa de mesa: {formatBuyIn(room.entry_fee_cents, true)} (cobrada
              de novo a cada vez que você compra fichas).</p>}
      <div className="buyin-control">
        <label htmlFor={sliderId}>Recompra</label>
        <input id={sliderId} type="range" min={room.buy_in_min} max={room.buy_in_max} step={step} value={value}
               disabled={joining} onChange={event => setAmount(Number(event.target.value))}
               aria-valuetext={`${fmt(value)} ${unit}`}/>
        <output htmlFor={sliderId}>{fmt(value)} <span>{unit}</span></output>
        <small>mín. {fmt(room.buy_in_min)} · máx. {fmt(room.buy_in_max)}</small>
      </div>
      {error && <p className="buyin-error" role="alert">{error}</p>}
      <DialogFooter>
        <Button type="button" variant="ghost" disabled={joining} onClick={() => setOpen(false)}>Agora não</Button>
        <Button type="button" disabled={joining} onClick={confirm}>
          {joining ? 'Comprando…' : `Comprar ${fmt(value)}`}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
