'use client';
import {useState} from 'react';
import {Check, CircleAlert, LoaderCircle, RotateCcw} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {EmojiGlyph} from '@/components/ui/EmojiGlyph';
import {ApiError} from '@/lib/api/client';
import type {ReactionPurchase} from '@/lib/api/reactionPurchases';
import {TABLE_REACTIONS, type TableReactionID} from '@/lib/reactions';

function valueLabel(purchase: ReactionPurchase) {
  return purchase.method === 'pix'
    ? ((purchase.price_cents ?? 0) / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'})
    : `${(purchase.price_fichas ?? 0).toLocaleString('pt-BR')} fichas`;
}

export function ReactionRefundDialog({purchase, onCloseAction, onConfirmAction}: {
  purchase: ReactionPurchase | null;
  onCloseAction: () => void;
  onConfirmAction: (purchaseId: string) => Promise<void>;
}) {
  const [state, setState] = useState<'confirm' | 'pending' | 'success' | 'error'>('confirm');
  const [error, setError] = useState('');
  if (!purchase) return null;
  const definition = TABLE_REACTIONS[purchase.reaction_id as TableReactionID];

  async function confirm() {
    setState('pending');
    setError('');
    try {
      await onConfirmAction(purchase!.purchase_id);
      setState('success');
    } catch (caught) {
      setError(caught instanceof ApiError && caught.status === 409
        ? 'Esta reação já foi usada e não pode mais ser estornada.'
        : 'Não foi possível concluir o estorno. Sua compra não foi alterada; tente novamente.');
      setState('error');
    }
  }

  return <Dialog open onOpenChange={open => !open && state !== 'pending' && onCloseAction()}>
    <DialogContent className="reaction-refund-dialog">
      <DialogHeader>
        <DialogTitle>{state === 'success' ? 'Reação estornada' : `Estornar ${definition?.label || 'reação'}?`}</DialogTitle>
        <DialogDescription>{state === 'success'
          ? `${valueLabel(purchase)} foi devolvido pelo mesmo meio da compra.`
          : 'O servidor confirma que a reação nunca foi enviada antes de autorizar o estorno.'}</DialogDescription>
      </DialogHeader>
      {state === 'success' ? <div className="reaction-refund-success" role="status">
        <Check aria-hidden="true"/><div><strong>Estorno concluído</strong><p>A reação voltou a ficar bloqueada.</p></div>
      </div> : <>
        <dl className="reaction-refund-summary">
          <div><dt>Reação</dt><dd>{definition?.glyph && <EmojiGlyph glyph={definition.glyph}/>} {definition?.label}</dd></div>
          <div><dt>Valor devolvido</dt><dd>{valueLabel(purchase)}</dd></div>
          <div><dt>Destino</dt><dd>{purchase.method === 'pix' ? 'Mesma compra Pix' : 'Saldo de fichas sandbox'}</dd></div>
        </dl>
        <p className="reaction-refund-rule"><CircleAlert aria-hidden="true"/>
          Depois do primeiro envio, a reação continua sua para sempre, mas perde a elegibilidade de estorno.</p>
        {error && <p className="reaction-purchase-error" role="alert">{error}</p>}
      </>}
      <DialogFooter>
        {state === 'success' ? <Button onClick={onCloseAction}>Concluir</Button> : <>
          <Button variant="ghost" disabled={state === 'pending'} onClick={onCloseAction}>Manter reação</Button>
          <Button variant="destructive" disabled={state === 'pending'} onClick={() => void confirm()}>
            {state === 'pending' ? <LoaderCircle className="spin" aria-hidden="true"/> : <RotateCcw aria-hidden="true"/>}
            {state === 'pending' ? 'Verificando uso…' : `Estornar ${valueLabel(purchase)}`}
          </Button>
        </>}
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
