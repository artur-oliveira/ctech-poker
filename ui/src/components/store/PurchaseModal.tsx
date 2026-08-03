'use client';
import {useEffect, useState} from 'react';
import Image from 'next/image';
import {Check, Copy, ShieldCheck} from 'lucide-react';
import {useQueryClient} from '@tanstack/react-query';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {getPurchase, type SandboxPurchase} from '@/lib/api/wallet';
import {formatDuration, useCountdownMs} from './useCountdown';

const POLL_MS = 5000;

export function PurchaseModal({purchase, onCloseAction, onUpdate}: {
  purchase: SandboxPurchase | null;
  onCloseAction: () => void;
  onUpdate: (purchase: SandboxPurchase) => void;
}) {
  const queryClient = useQueryClient();
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const open = purchase !== null;
  const expiresMs = purchase?.expires_at ? new Date(purchase.expires_at).getTime() : null;
  const qrImageType = purchase?.qr_code_base64?.startsWith('PHN2Zy') ? 'image/svg+xml' : 'image/png';
  const remainingMs = useCountdownMs(open ? expiresMs : null);
  const expired = expiresMs !== null && remainingMs <= 0;

  useEffect(() => {
    if (!copied) return undefined;
    const id = window.setTimeout(() => setCopied(false), 2000);
    return () => window.clearTimeout(id);
  }, [copied]);

  useEffect(() => {
    if (purchase?.status !== 'confirmed') return;
    void queryClient.invalidateQueries({queryKey: ['wallet', 'sandbox-purchases']});
    void queryClient.invalidateQueries({queryKey: ['wallet', 'balance']});
    void queryClient.invalidateQueries({queryKey: ['player', 'me']});
  }, [purchase?.status, queryClient]);

  // Websocket confirmation (useLobbyRealtime) is the primary path; this poll
  // is only a safety net for a missed/dropped ws frame while the modal is open.
  useEffect(() => {
    if (!open || !purchase || purchase.status !== 'pending') return undefined;
    const id = window.setInterval(() => {
      getPurchase(purchase.purchase_id).then(onUpdate).catch(() => undefined);
    }, POLL_MS);
    return () => window.clearInterval(id);
  }, [open, purchase, onUpdate]);

  async function copy() {
    if (!purchase?.pix_copia_e_cola) return;
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(purchase.pix_copia_e_cola);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  }

  return <Dialog open={open} onOpenChange={next => {
    if (!next) onCloseAction();
  }}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{purchase?.status === 'confirmed' ? 'Fichas adicionadas'
          : expired ? 'Código Pix expirado'
            : purchase?.status && purchase.status !== 'pending' ? 'Compra encerrada' : 'Pague com Pix para concluir'}</DialogTitle>
        <DialogDescription>{purchase?.status === 'confirmed'
          ? 'O pagamento foi confirmado e seu saldo sandbox está atualizado.'
          : expired ? 'Este código não pode mais ser usado. Feche esta janela e escolha um pacote novamente.'
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
        : purchase?.status && purchase.status !== 'pending'
          ? <p className="buyin-error" role="alert">Esta compra não está mais disponível ({STATUS_LABEL[purchase.status] || 'status desconhecido'}).</p>
          : <>
            {purchase?.qr_code_base64 && <div className="store-qr">
              <Image src={`data:${qrImageType};base64,${purchase.qr_code_base64}`} alt="QR code Pix para pagamento"
                     width={200} height={200} unoptimized/>
            </div>}
            <div className="buyin-control store-pix-control">
              <label htmlFor="pix-copia-e-cola">Pix copia e cola</label>
              <div className="store-pix-field">
                <input id="pix-copia-e-cola" value={purchase?.pix_copia_e_cola ?? ''} readOnly
                       onClick={event => event.currentTarget.select()}/>
                <Button type="button" variant="ghost" size="icon"
                        aria-label={copied ? 'Código Pix copiado' : expired ? 'Código Pix expirado' : 'Copiar código Pix'}
                        title={copied ? 'Copiado' : expired ? 'Código expirado' : 'Copiar código Pix'}
                        disabled={expired || !purchase?.pix_copia_e_cola} onClick={() => void copy()}>
                  {copied ? <Check aria-hidden="true"/> : <Copy aria-hidden="true"/>}
                </Button>
              </div>
              <span className="sr-only" aria-live="polite">{copied ? 'Código Pix copiado.' : ''}</span>
            </div>
            {copyFailed && <p className="buyin-error" role="alert">Não foi possível copiar automaticamente. Selecione o código acima e copie manualmente.</p>}
            {expiresMs !== null && <p className={`store-countdown${expired ? ' is-expiring' : ''}`}>
              {expired ? 'Código expirado' : `Expira em ${formatDuration(remainingMs)}`}
            </p>}
            <p className="store-payment-note"><ShieldCheck aria-hidden="true"/> As fichas são apenas do modo sandbox e não têm valor em dinheiro.</p>
          </>}
    </DialogContent>
  </Dialog>;
}

const STATUS_LABEL: Record<string, string> = {
  refunded: 'estornada',
  expired: 'expirada',
  failed: 'falhou',
};
