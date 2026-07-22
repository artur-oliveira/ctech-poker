'use client';
import {useState} from 'react';
import {isAxiosError} from 'axios';
import {DoorOpen} from 'lucide-react';
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
import {leaveRoom} from '@/lib/api/rooms';

// The engine rejects a cash-out while the player is still dealt into the
// current hand (active/all-in) — fold or wait for showdown first.
const DEALT_IN_MESSAGE = 'Você está na mão atual. Desista ou aguarde o fim da rodada para sair.';
const GENERIC_MESSAGE = 'Não foi possível sair da mesa agora. Tente novamente.';

export function LeaveDialog({roomId, stack, onLeft}: { roomId: string; stack: number; onLeft: (amount: number) => void }) {
  const [open, setOpen] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [error, setError] = useState('');

  async function confirm() {
    setLeaving(true);
    setError('');
    try {
      const {amount} = await leaveRoom(roomId);
      setOpen(false);
      onLeft(amount);
    } catch (e) {
      setError(isAxiosError(e) && e.response?.status === 409 ? DEALT_IN_MESSAGE : GENERIC_MESSAGE);
      setLeaving(false);
    }
  }

  return <Dialog open={open} onOpenChange={next => {
    setOpen(next);
    if (!next) setError('');
  }}>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Sair da mesa"/>}>
      <DoorOpen/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Sair da mesa?</DialogTitle>
        <DialogDescription>Você será pago com {stack.toLocaleString('pt-BR')} fichas.</DialogDescription>
      </DialogHeader>
      {error && <p className="buyin-error" role="alert">{error}</p>}
      <DialogFooter>
        <Button type="button" variant="ghost" disabled={leaving} onClick={() => setOpen(false)}>Cancelar</Button>
        <Button type="button" variant="destructive" disabled={leaving} onClick={confirm}>
          {leaving ? 'Saindo…' : 'Sair e sacar fichas'}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
