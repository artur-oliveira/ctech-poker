'use client';
import type {RefObject} from 'react';
import {useEffect, useRef, useState} from 'react';
import Image from 'next/image';
import {Check, Coins, LoaderCircle, QrCode, ShieldCheck} from 'lucide-react';
import {useQuery, useQueryClient} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle
} from '@/components/ui/dialog';
import {PixPaymentView} from '@/components/store/PixPaymentView';
import {ApiError} from '@/lib/api/client';
import {
  cosmeticPurchaseKey, createCosmeticPurchase, getCosmeticPurchase, type CosmeticCatalogEntry,
  type CosmeticKind, type CosmeticPurchase, type CosmeticPurchaseMethod
} from '@/lib/api/cosmeticPurchases';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, type DeckVariantId} from '@/lib/cardVariants';
import {TABLE_THEMES, type TableThemeId} from '@/lib/tablePreferences';
import {WALLET_QUERY_ROOT} from '@/lib/api/wallet';

const POLL_MS = 4000;

function formatBRL(cents?: number) {
  return ((cents ?? 0) / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
}

function purchaseError(error: unknown) {
  if (error instanceof ApiError && error.status === 409) {
    return 'Este item já é seu ou existe uma compra dele em andamento. Atualize a compra para continuar.';
  }
  if (error instanceof ApiError && error.status === 400) {
    return 'Não foi possível usar este meio de pagamento para este item.';
  }
  return 'Não foi possível iniciar a compra agora. Tente novamente.';
}

function labelFor(kind: CosmeticKind, itemId: string) {
  return kind === 'deck' ? DECK_VARIANTS[itemId as DeckVariantId]?.label : TABLE_THEMES[itemId as TableThemeId]?.label;
}

const ACES = ['As', 'Ah', 'Ad', 'Ac'];

function CosmeticPreview({kind, itemId}: { kind: CosmeticKind; itemId: string }) {
  if (kind === 'deck') return <span className="cosmetic-store-deck-preview">
    {ACES.map(card => <Image key={card} src={cardPath(card, itemId as DeckVariantId)} alt="" width={36} height={50}/>)}
  </span>;
  const theme = TABLE_THEMES[itemId as TableThemeId];
  return <span className="felt-swatch"
               style={{'--theme-a': theme?.colors[0], '--theme-b': theme?.colors[1]} as React.CSSProperties}/>;
}

export function CosmeticPurchaseDialog({kind, entry, initialPurchase, sandboxBalance, finalFocusRef, onCloseAction,
                                        onConfirmedAction}: {
  kind: CosmeticKind;
  entry: CosmeticCatalogEntry | null;
  initialPurchase?: CosmeticPurchase;
  sandboxBalance?: number;
  /** Restores keyboard focus to the control that opened this dialog. */
  finalFocusRef?: RefObject<HTMLButtonElement | null>;
  onCloseAction: () => void;
  onConfirmedAction?: (purchase: CosmeticPurchase) => void;
}) {
  const queryClient = useQueryClient();
  const [started, setStarted] = useState<CosmeticPurchase | undefined>(initialPurchase);
  const [pendingMethod, setPendingMethod] = useState<CosmeticPurchaseMethod | null>(null);
  const [error, setError] = useState('');
  const label = entry && labelFor(kind, entry.id);
  const startedId = started?.purchase_id;

  // The purchase's live status is server state, so it lives in the query cache
  // under `cosmeticPurchaseKey`: `refetchInterval` is the 4s fallback poll, and
  // the `cosmetic_purchase_update` websocket frame (#144) invalidates this same
  // key, which resolves the dialog on the next tick instead of on the next poll.
  const statusQuery = useQuery({
    queryKey: cosmeticPurchaseKey(kind, startedId ?? ''),
    queryFn: () => getCosmeticPurchase(kind, startedId!),
    enabled: Boolean(startedId) && (started?.status === 'pending' || started?.status === 'processing'),
    refetchInterval: POLL_MS
  });

  const purchase = statusQuery.data ?? started;
  const confirmed = purchase?.status === 'confirmed';
  const active = purchase?.status === 'pending' || purchase?.status === 'processing';
  const expired = purchase?.status === 'expired';

  // Fires once per confirmed purchase id: the store's ownership refresh must
  // not re-run on every re-render that still reads a confirmed status.
  const announcedRef = useRef('');
  useEffect(() => {
    if (!confirmed || !purchase || announcedRef.current === purchase.purchase_id) return;
    announcedRef.current = purchase.purchase_id;
    void queryClient.invalidateQueries({queryKey: WALLET_QUERY_ROOT});
    void queryClient.invalidateQueries({queryKey: ['player', 'me']});
    onConfirmedAction?.(purchase);
  }, [confirmed, purchase, queryClient, onConfirmedAction]);

  if (!entry || !label) return null;

  const insufficientFichas = sandboxBalance !== undefined && sandboxBalance < (entry.price_fichas ?? 0);

  async function buy(method: CosmeticPurchaseMethod) {
    setPendingMethod(method);
    setError('');
    try {
      const next = await createCosmeticPurchase(kind, entry!.id, method);
      setStarted(next);
      queryClient.setQueryData(cosmeticPurchaseKey(kind, next.purchase_id), next);
    } catch (caught) {
      setError(purchaseError(caught));
    } finally {
      setPendingMethod(null);
    }
  }

  return <Dialog open onOpenChange={open => {
    if (!open && !pendingMethod) onCloseAction();
  }}>
    <DialogContent className="reaction-purchase-dialog cosmetic-purchase-dialog" finalFocus={finalFocusRef}>
      <DialogHeader>
        <span className="cosmetic-purchase-hero"><CosmeticPreview kind={kind} itemId={entry.id}/></span>
        <DialogTitle>{confirmed ? `${label} liberado` : `Liberar ${label}`}</DialogTitle>
        <DialogDescription>{confirmed
          ? 'O item é seu para sempre e já pode ser usado em qualquer mesa.'
          : 'Uma compra, uso permanente. Escolha como pagar; não há assinatura nem consumo por uso.'}</DialogDescription>
      </DialogHeader>

      {confirmed ? <div className="reaction-purchase-success" role="status" aria-live="polite">
        <span><Check aria-hidden="true"/></span>
        <div><strong>Pronto para a mesa</strong><p>Feche esta janela e selecione o item na sua próxima mão.</p></div>
      </div> : active && purchase?.method === 'pix' ? <>
        <PixPaymentView purchase={purchase} amountDetail={label}
          paymentNote="O pagamento libera apenas este item cosmético e não altera fichas nem saldo de jogo."/>
        <p className="reaction-purchase-wait"><LoaderCircle aria-hidden="true"/> Aguardando confirmação do Pix…</p>
      </> : active ? <div className="reaction-purchase-processing" role="status">
        <LoaderCircle aria-hidden="true"/><strong>Confirmando o débito de fichas…</strong>
        <p>Você pode fechar esta janela. A compra continuará segura e aparecerá no histórico.</p>
      </div> : <>
        <div className="reaction-payment-options" role="group" aria-label="Meios de pagamento">
          <button type="button" disabled={pendingMethod !== null || insufficientFichas}
                  onClick={() => void buy('fichas')}>
            <Coins aria-hidden="true"/><span><strong>{(entry.price_fichas ?? 0).toLocaleString('pt-BR')} fichas</strong>
              <small>{insufficientFichas ? 'Saldo sandbox insuficiente' : 'Confirmação imediata'}</small></span>
            {pendingMethod === 'fichas' && <LoaderCircle className="spin" aria-hidden="true"/>}
          </button>
          <button type="button" disabled={pendingMethod !== null} onClick={() => void buy('pix')}>
            <QrCode aria-hidden="true"/><span><strong>{formatBRL(entry.price_cents)} via Pix</strong>
              <small>QR code no próximo passo</small></span>
            {pendingMethod === 'pix' && <LoaderCircle className="spin" aria-hidden="true"/>}
          </button>
        </div>
        <p className="reaction-purchase-assurance"><ShieldCheck aria-hidden="true"/>
          O preço vem do catálogo do servidor e o item só é liberado após a confirmação.</p>
      </>}

      {expired && <p className="reaction-purchase-error" role="alert">Este Pix expirou. Gere uma nova compra para tentar novamente.</p>}
      {active && statusQuery.isError && <div className="store-poll-recovery" role="alert">
        <span>Não foi possível atualizar a confirmação.</span>
        <Button type="button" variant="ghost" onClick={() => void statusQuery.refetch()}>Verificar novamente</Button>
      </div>}
      {error && <p className="reaction-purchase-error" role="alert">{error}</p>}

      <DialogFooter>
        {confirmed ? <Button type="button" onClick={onCloseAction}>Usar na mesa</Button>
          : expired ? <><Button type="button" variant="ghost" onClick={onCloseAction}>Agora não</Button>
            <Button type="button" disabled={pendingMethod !== null} onClick={() => void buy('pix')}>Gerar novo Pix</Button></>
          : <Button type="button" variant="ghost" disabled={pendingMethod !== null} onClick={onCloseAction}>
            {active ? 'Fechar e acompanhar depois' : 'Agora não'}
          </Button>}
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
