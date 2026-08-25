'use client';
import type {RefObject} from 'react';
import {useState} from 'react';
import {Check, CircleAlert, Coins, LoaderCircle, RotateCcw} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle
} from '@/components/ui/dialog';
import {ApiError} from '@/lib/api/client';
import type {SandboxPurchase} from '@/lib/api/wallet';

type RefundState = 'confirm' | 'pending' | 'success' | 'error';

function formatBRL(cents?: number) {
  return ((cents ?? 0) / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
}

function refundError(error: unknown) {
  if (error instanceof ApiError && error.status === 409) {
    return 'O estorno não é mais elegível. As fichas desta compra podem ter sido usadas em uma mesa.';
  }
  return 'Não foi possível concluir o estorno agora. Seu histórico não foi alterado; tente novamente.';
}

export function RefundConfirmationDialog({purchase, sandboxBalance, finalFocusRef, onCloseAction, onConfirmAction}: {
  purchase: SandboxPurchase | null;
  sandboxBalance?: number;
  /** Restores keyboard focus to the control that opened this dialog. */
  finalFocusRef?: RefObject<HTMLButtonElement | null>;
  onCloseAction: () => void;
  onConfirmAction: (purchaseId: string) => Promise<void>;
}) {
  const [state, setState] = useState<RefundState>('confirm');
  const [message, setMessage] = useState('');

  if (!purchase) return null;

  const credits = purchase.total_credits ?? 0;
  const projectedBalance = sandboxBalance === undefined ? null : Math.max(0, sandboxBalance - credits);
  const pending = state === 'pending';

  async function confirm() {
    setState('pending');
    setMessage('');
    try {
      await onConfirmAction(purchase!.purchase_id);
      setState('success');
    } catch (error) {
      setMessage(refundError(error));
      setState('error');
    }
  }

  return <Dialog open onOpenChange={open => {
    if (!open && !pending) onCloseAction();
  }}>
    <DialogContent className="store-refund-dialog" finalFocus={finalFocusRef}>
      <DialogHeader>
        <DialogTitle>{state === 'success' ? 'Compra estornada' : 'Solicitar estorno desta compra?'}</DialogTitle>
        <DialogDescription>{state === 'success'
          ? `O estorno de ${formatBRL(purchase.price_cents)} foi confirmado e as fichas foram removidas.`
          : 'Confira o valor e o efeito no seu saldo antes de confirmar.'}</DialogDescription>
      </DialogHeader>

      {state === 'success' ? <div className="store-refund-result" role="status" aria-live="polite">
        <span className="store-refund-result-icon"><Check aria-hidden="true"/></span>
        <div><strong>{formatBRL(purchase.price_cents)} em estorno</strong>
          <p>{credits.toLocaleString('pt-BR')} fichas sandbox removidas do saldo.</p></div>
      </div> : <>
        <dl className="store-refund-summary">
          <div><dt>Valor pago via Pix</dt><dd>{formatBRL(purchase.price_cents)}</dd></div>
          <div><dt>Fichas desta compra</dt><dd>{credits.toLocaleString('pt-BR')} fichas</dd></div>
          <div><dt>Saldo após o estorno</dt><dd>{projectedBalance === null
            ? 'Atualizado após confirmar'
            : `${projectedBalance.toLocaleString('pt-BR')} fichas`}</dd></div>
        </dl>

        <div className="store-refund-eligibility">
          <Coins aria-hidden="true"/>
          <div><strong>Disponível somente antes de usar as fichas</strong>
            <p>Ao confirmar, o servidor verifica se houve algum débito sandbox depois deste crédito. Se as fichas já foram usadas em uma mesa, o estorno será recusado e seu saldo permanecerá igual.</p></div>
        </div>
        <p className="store-refund-boundary"><CircleAlert aria-hidden="true"/>
          Esta ação trata apenas desta compra de fichas sandbox. Não movimenta saldo de dinheiro real e não transforma fichas em dinheiro.</p>
        {state === 'error' && <p className="store-refund-error" role="alert">{message}</p>}
      </>}

      <DialogFooter>
        {state === 'success'
          ? <Button type="button" onClick={onCloseAction}>Concluir</Button>
          : <><Button type="button" variant="ghost" disabled={pending} onClick={onCloseAction}>Manter compra</Button>
            <Button type="button" variant="destructive" disabled={pending} onClick={() => void confirm()}>
              {pending ? <LoaderCircle className="store-refund-spinner" aria-hidden="true"/> : <RotateCcw aria-hidden="true"/>}
              {pending ? 'Verificando elegibilidade…' : `Solicitar estorno de ${formatBRL(purchase.price_cents)}`}
            </Button></>}
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
