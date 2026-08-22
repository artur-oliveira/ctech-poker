'use client';
import {useCallback, useEffect, useState} from 'react';
import Image from 'next/image';
import {Check, Coins, LoaderCircle, QrCode, ShieldCheck} from 'lucide-react';
import {useQueryClient} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle
} from '@/components/ui/dialog';
import {PixPaymentView} from '@/components/store/PixPaymentView';
import {ApiError} from '@/lib/api/client';
import {
  createCosmeticPurchase, getCosmeticPurchase, type CosmeticCatalogEntry, type CosmeticKind,
  type CosmeticPurchase, type CosmeticPurchaseMethod
} from '@/lib/api/cosmeticPurchases';
import {cardPath} from '@/lib/cards';
import {DECK_VARIANTS, type DeckVariantId} from '@/lib/cardVariants';
import {TABLE_THEMES, type TableThemeId} from '@/lib/tablePreferences';

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

function CosmeticPreview({kind, itemId}: { kind: CosmeticKind; itemId: string }) {
  if (kind === 'deck') return <Image src={cardPath('As', itemId as DeckVariantId)} alt="" width={48} height={68}/>;
  const theme = TABLE_THEMES[itemId as TableThemeId];
  return <span className="felt-swatch"
               style={{'--theme-a': theme?.colors[0], '--theme-b': theme?.colors[1]} as React.CSSProperties}/>;
}

export function CosmeticPurchaseDialog({kind, entry, initialPurchase, sandboxBalance, onCloseAction,
                                        onConfirmedAction}: {
  kind: CosmeticKind;
  entry: CosmeticCatalogEntry | null;
  initialPurchase?: CosmeticPurchase;
  sandboxBalance?: number;
  onCloseAction: () => void;
  onConfirmedAction?: (purchase: CosmeticPurchase) => void;
}) {
  const queryClient = useQueryClient();
  const [purchase, setPurchase] = useState<CosmeticPurchase | undefined>(initialPurchase);
  const [pendingMethod, setPendingMethod] = useState<CosmeticPurchaseMethod | null>(null);
  const [error, setError] = useState('');
  const [pollError, setPollError] = useState(false);
  const label = entry && labelFor(kind, entry.id);
  const confirmed = purchase?.status === 'confirmed';
  const active = purchase?.status === 'pending' || purchase?.status === 'processing';
  const expired = purchase?.status === 'expired';
  const purchaseId = purchase?.purchase_id;

  const refreshStatus = useCallback(async () => {
    if (!purchaseId) return;
    try {
      const next = await getCosmeticPurchase(kind, purchaseId);
      setPurchase(next);
      setPollError(false);
      if (next.status === 'confirmed') {
        void queryClient.invalidateQueries({queryKey: ['wallet', 'cosmetic-purchases', kind]});
        void queryClient.invalidateQueries({queryKey: ['wallet', 'cosmetic-catalog', kind]});
        void queryClient.invalidateQueries({queryKey: ['player', 'me']});
        onConfirmedAction?.(next);
      }
    } catch {
      setPollError(true);
    }
  }, [kind, purchaseId, queryClient, onConfirmedAction]);

  useEffect(() => {
    if (!active) return undefined;
    const timer = window.setInterval(() => void refreshStatus(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [active, refreshStatus]);

  if (!entry || !label) return null;

  const insufficientFichas = sandboxBalance !== undefined && sandboxBalance < (entry.price_fichas ?? 0);

  async function buy(method: CosmeticPurchaseMethod) {
    setPendingMethod(method);
    setError('');
    try {
      const next = await createCosmeticPurchase(kind, entry!.id, method);
      setPurchase(next);
      await Promise.all([
        queryClient.invalidateQueries({queryKey: ['wallet', 'cosmetic-purchases', kind]}),
        queryClient.invalidateQueries({queryKey: ['player', 'me']}),
      ]);
      if (next.status === 'confirmed') onConfirmedAction?.(next);
    } catch (caught) {
      setError(purchaseError(caught));
    } finally {
      setPendingMethod(null);
    }
  }

  return <Dialog open onOpenChange={open => {
    if (!open && !pendingMethod) onCloseAction();
  }}>
    <DialogContent className="reaction-purchase-dialog cosmetic-purchase-dialog">
      <DialogHeader>
        <span className="reaction-purchase-hero cosmetic-purchase-hero"><CosmeticPreview kind={kind} itemId={entry.id}/></span>
        <DialogTitle>{confirmed ? `${label} liberado` : `Liberar ${label}`}</DialogTitle>
        <DialogDescription>{confirmed
          ? 'O item é seu para sempre e já pode ser usado em qualquer mesa.'
          : 'Uma compra, uso permanente. Escolha como pagar; não há assinatura nem consumo por uso.'}</DialogDescription>
      </DialogHeader>

      {confirmed ? <div className="reaction-purchase-success" role="status" aria-live="polite">
        <span><Check aria-hidden="true"/></span>
        <div><strong>Pronto para a mesa</strong><p>Feche esta janela e selecione o item na sua próxima mão.</p></div>
      </div> : active && purchase?.method === 'pix' ? <>
        <PixPaymentView purchase={purchase}
          paymentNote="O pagamento libera apenas este item cosmético e não altera fichas nem saldo de jogo."/>
        <p className="reaction-purchase-wait"><LoaderCircle aria-hidden="true"/> Aguardando confirmação do Pix…</p>
      </> : active ? <div className="reaction-purchase-processing" role="status">
        <LoaderCircle aria-hidden="true"/><strong>Confirmando o débito de fichas…</strong>
        <p>Você pode fechar esta janela. A compra continuará segura e aparecerá no histórico.</p>
      </div> : <>
        <div className="reaction-payment-options" aria-label="Meios de pagamento">
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
      {active && pollError && <div className="store-poll-recovery" role="alert">
        <span>Não foi possível atualizar a confirmação.</span>
        <Button type="button" variant="ghost" onClick={() => void refreshStatus()}>Verificar novamente</Button>
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
