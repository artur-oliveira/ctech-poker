'use client';
import {useState} from 'react';
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

// Confirming no longer blocks on the current hand: request_exit pauses the
// player immediately and, if dealt in, the table's persistent exit status
// (ExitStatus) takes over once this dialog closes — see
// docs/plans/2026-08-26-exit-mid-hand-design.md.
export function LeaveDialog({stack, pending, onRequestExitAction}: {
  stack: number;
  pending?: boolean;
  onRequestExitAction: () => boolean;
}) {
  const [open, setOpen] = useState(false);

  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger render={<Button type="button" variant="ghost" size="icon" aria-label="Sair da mesa"/>}>
      <DoorOpen/>
    </DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Sair da mesa?</DialogTitle>
        <DialogDescription>Você será pago com {stack.toLocaleString('pt-BR')} fichas assim que
          estiver livre para sair — imediatamente, ou ao fim da mão atual se ainda estiver
          participando dela.</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button type="button" variant="ghost" disabled={pending} onClick={() => setOpen(false)}>Continuar
          jogando</Button>
        <Button type="button" variant="destructive" disabled={pending} onClick={() => {
          onRequestExitAction();
          setOpen(false);
        }}>
          {pending ? 'Saindo…' : 'Sair e sacar fichas'}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
