'use client';
import {useState} from 'react';
import Image from 'next/image';
import {Check, CircleAlert, LoaderCircle, RotateCcw} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {ApiError} from '@/lib/api/client';
import type {CosmeticKind, CosmeticPurchase} from '@/lib/api/cosmeticPurchases';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, type DeckVariantId} from '@/lib/cardVariants';
import {TABLE_THEMES, type TableThemeId} from '@/lib/tablePreferences';

function labelFor(kind: CosmeticKind, itemId: string) {
  return kind === 'deck' ? DECK_VARIANTS[itemId as DeckVariantId]?.label : TABLE_THEMES[itemId as TableThemeId]?.label;
}

function CosmeticPreview({kind, itemId}: { kind: CosmeticKind; itemId: string }) {
  if (kind === 'deck') return <Image src={cardPath('As', itemId as DeckVariantId)} alt="" width={32} height={45}/>;
  const theme = TABLE_THEMES[itemId as TableThemeId];
  return <span className="felt-swatch"
               style={{'--theme-a': theme?.colors[0], '--theme-b': theme?.colors[1]} as React.CSSProperties}/>;
}

function valueLabel(purchase: CosmeticPurchase) {
  return purchase.method === 'pix'
    ? ((purchase.price_cents ?? 0) / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'})
    : `${(purchase.price_fichas ?? 0).toLocaleString('pt-BR')} fichas`;
}

export function CosmeticRefundDialog({kind, purchase, onCloseAction, onConfirmAction}: {
  kind: CosmeticKind;
  purchase: CosmeticPurchase | null;
  onCloseAction: () => void;
  onConfirmAction: (purchaseId: string) => Promise<void>;
}) {
  const [state, setState] = useState<'confirm' | 'pending' | 'success' | 'error'>('confirm');
  const [error, setError] = useState('');
  if (!purchase) return null;
  const label = labelFor(kind, purchase.item_id);

  async function confirm() {
    setState('pending');
    setError('');
    try {
      await onConfirmAction(purchase!.purchase_id);
      setState('success');
    } catch (caught) {
      setError(caught instanceof ApiError && caught.status === 409
        ? 'Este item já foi selecionado e não pode mais ser estornado.'
        : 'Não foi possível concluir o estorno. Sua compra não foi alterada; tente novamente.');
      setState('error');
    }
  }

  return <Dialog open onOpenChange={open => !open && state !== 'pending' && onCloseAction()}>
    <DialogContent className="reaction-refund-dialog cosmetic-refund-dialog">
      <DialogHeader>
        <DialogTitle>{state === 'success' ? 'Item estornado' : `Estornar ${label || 'item'}?`}</DialogTitle>
        <DialogDescription>{state === 'success'
          ? `${valueLabel(purchase)} foi devolvido pelo mesmo meio da compra.`
          : 'O servidor confirma que o item nunca foi selecionado antes de autorizar o estorno.'}</DialogDescription>
      </DialogHeader>
      {state === 'success' ? <div className="reaction-refund-success" role="status">
        <Check aria-hidden="true"/><div><strong>Estorno concluído</strong><p>O item voltou a ficar bloqueado.</p></div>
      </div> : <>
        <dl className="reaction-refund-summary">
          <div><dt>Item</dt><dd><CosmeticPreview kind={kind} itemId={purchase.item_id}/> {label}</dd></div>
          <div><dt>Valor devolvido</dt><dd>{valueLabel(purchase)}</dd></div>
          <div><dt>Destino</dt><dd>{purchase.method === 'pix' ? 'Mesma compra Pix' : 'Saldo de fichas sandbox'}</dd></div>
        </dl>
        <p className="reaction-refund-rule"><CircleAlert aria-hidden="true"/>
          Depois de selecionado pela primeira vez, o item continua seu para sempre, mas perde a elegibilidade de estorno.</p>
        {error && <p className="reaction-purchase-error" role="alert">{error}</p>}
      </>}
      <DialogFooter>
        {state === 'success' ? <Button onClick={onCloseAction}>Concluir</Button> : <>
          <Button variant="ghost" disabled={state === 'pending'} onClick={onCloseAction}>Manter item</Button>
          <Button variant="destructive" disabled={state === 'pending'} onClick={() => void confirm()}>
            {state === 'pending' ? <LoaderCircle className="spin" aria-hidden="true"/> : <RotateCcw aria-hidden="true"/>}
            {state === 'pending' ? 'Verificando uso…' : `Estornar ${valueLabel(purchase)}`}
          </Button>
        </>}
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
