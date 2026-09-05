'use client';
import {type RefObject, useEffect, useState} from 'react';
import {Check, LoaderCircle, RefreshCw} from 'lucide-react';
import {useQueryClient} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {getPurchase, sandboxPurchaseKey, type SandboxPurchase} from '@/lib/api/wallet';
import {BALANCE_QUERY_KEY} from '@/lib/api/player';
import {usePurchaseStatus} from '@/lib/hooks/usePurchaseStatus';
import {useCountdownMs} from './useCountdown';
import {PixPaymentView} from './PixPaymentView';

export function PurchaseModal({purchase, finalFocusRef, onCloseAction, onUpdateAction, onRegenerateAction}: {
  purchase: SandboxPurchase | null;
  // The modal is opened programmatically (no DialogTrigger), so base-ui has no
  // element of its own to hand focus back to on close. Passing the package
  // button that opened it lets base-ui restore focus as part of its own close
  // teardown, instead of the page racing that teardown with a rAF of its own.
  finalFocusRef?: RefObject<HTMLButtonElement | null>;
  onCloseAction: () => void;
  onUpdateAction: (purchase: SandboxPurchase) => void;
  onRegenerateAction: (sku: string) => Promise<void>;
}) {
  const queryClient = useQueryClient();
  const [regenerating, setRegenerating] = useState(false);
  const [regenerateFailed, setRegenerateFailed] = useState(false);
  const open = purchase !== null;
  const purchaseId = purchase?.purchase_id;
  const expiresMs = purchase?.expires_at ? new Date(purchase.expires_at).getTime() : null;
  const remainingMs = useCountdownMs(open ? expiresMs : null);
  const expired = expiresMs !== null && remainingMs <= 0;
  const recoverableExpired = expired || purchase?.status === 'expired';

  useEffect(() => {
    if (purchase?.status !== 'confirmed') return;
    void queryClient.invalidateQueries({queryKey: ['wallet', 'sandbox-purchases']});
    void queryClient.invalidateQueries({queryKey: BALANCE_QUERY_KEY});
  }, [purchase?.status, queryClient]);

  // Websocket confirmation (`sandbox_purchase_update` in useLobbyRealtime,
  // which invalidates the wallet root this key lives under) is the primary
  // path; the shared fallback poll is only a safety net for a missed or dropped
  // frame. It lives in the query cache, so it no longer keeps firing in a
  // hidden tab, backs off as the wait grows and stops when the Pix window
  // closes. See #227.
  const statusQuery = usePurchaseStatus<SandboxPurchase>({
    queryKey: sandboxPurchaseKey(purchaseId ?? ''),
    queryFn: () => getPurchase(purchaseId!),
    purchase: purchase ?? undefined,
    enabled: open && Boolean(purchaseId),
  });
  const pollFailed = statusQuery.isError;
  const pollChecking = statusQuery.isFetching;
  // The store page owns the purchase row (it also drives the history list), so
  // a fresher status is handed upwards rather than mirrored here.
  const polled = statusQuery.data;
  useEffect(() => {
    if (polled) onUpdateAction(polled);
  }, [polled, onUpdateAction]);

  async function regenerate() {
    if (!purchase?.sku || regenerating) return;
    setRegenerating(true);
    setRegenerateFailed(false);
    try {
      await onRegenerateAction(purchase.sku);
    } catch {
      setRegenerateFailed(true);
    } finally {
      setRegenerating(false);
    }
  }

  return <Dialog open={open} onOpenChange={next => {
    if (!next) onCloseAction();
  }}>
    <DialogContent finalFocus={finalFocusRef}>
      <DialogHeader>
        <DialogTitle>{purchase?.status === 'confirmed' ? 'Fichas adicionadas'
          : recoverableExpired ? 'Código Pix expirado'
            : purchase?.status && purchase.status !== 'pending' ? 'Compra encerrada' : 'Pague com Pix para concluir'}</DialogTitle>
        <DialogDescription>{purchase?.status === 'confirmed'
          ? 'O pagamento foi confirmado e seu saldo sandbox está atualizado.'
          : recoverableExpired ? 'Este código não pode mais ser usado. Gere um novo Pix para manter o mesmo pacote.'
            : purchase?.status && purchase.status !== 'pending'
            ? 'Este pagamento não pode mais ser concluído.'
            : 'Escaneie o QR code ou copie o código Pix no aplicativo do seu banco.'}</DialogDescription>
      </DialogHeader>
      {purchase?.status === 'confirmed'
        ? <div className="store-purchase-success" role="status">
          <span className="store-purchase-success-icon"><Check aria-hidden="true"/></span>
          <strong>Pagamento confirmado</strong>
          <p>{purchase.total_credits
            ? `${purchase.total_credits.toLocaleString('pt-BR')} fichas sandbox já estão no seu saldo.`
            : 'Suas fichas sandbox já estão no saldo.'}</p>
          <span className="store-purchase-chips" aria-hidden="true"><i/><i/><i/><i/><i/></span>
        </div>
        : purchase?.status && purchase.status !== 'pending' && !recoverableExpired
          ? <p className="buyin-error" role="alert">Esta compra não está mais disponível ({STATUS_LABEL[purchase.status] || 'status desconhecido'}).</p>
          : <PixPaymentView purchase={purchase!}
                            amountDetail={purchase?.total_credits
                              ? `${purchase.total_credits.toLocaleString('pt-BR')} fichas sandbox` : undefined}/>}
      {purchase?.status === 'pending' && pollFailed && <div className="store-poll-recovery" role="alert">
        <span>Não foi possível atualizar a confirmação. Seu pagamento não foi alterado.</span>
        <Button type="button" variant="ghost" disabled={pollChecking} onClick={() => void statusQuery.refetch()}>
          {pollChecking ? <LoaderCircle className="spin" aria-hidden="true"/> : <RefreshCw aria-hidden="true"/>}
          {pollChecking ? 'Verificando…' : 'Verificar pagamento'}
        </Button>
      </div>}
      {recoverableExpired && <>
        {regenerateFailed && <p className="buyin-error" role="alert">Não foi possível gerar um novo Pix. Tente novamente ou feche para voltar ao pacote.</p>}
        <DialogFooter>
          <Button type="button" variant="ghost" disabled={regenerating} onClick={onCloseAction}>Voltar aos pacotes</Button>
          <Button type="button" disabled={regenerating || !purchase?.sku} onClick={() => void regenerate()}>
            {regenerating ? 'Gerando novo Pix…' : 'Gerar novo Pix para este pacote'}
          </Button>
        </DialogFooter>
      </>}
    </DialogContent>
  </Dialog>;
}

const STATUS_LABEL: Record<string, string> = {
  refunded: 'estornada',
  expired: 'expirada',
  failed: 'falhou',
};
