'use client';
import {useEffect, useState} from 'react';
import Image from 'next/image';
import {Check, Copy} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '@/components/ui/dialog';
import {getPurchase, type SandboxPurchase} from '@/lib/api/wallet';
import {formatDuration, useCountdownMs} from './useCountdown';

const POLL_MS = 5000;

export function PurchaseModal({purchase, onClose, onUpdate}: {
  purchase: SandboxPurchase | null;
  onClose: () => void;
  onUpdate: (purchase: SandboxPurchase) => void;
}) {
  const [copied, setCopied] = useState(false);
  const open = purchase !== null;
  const expiresMs = purchase?.expires_at ? new Date(purchase.expires_at).getTime() : null;
  const remainingMs = useCountdownMs(open ? expiresMs : null);
  const expired = expiresMs !== null && remainingMs <= 0;

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
      await navigator.clipboard.writeText(purchase.pix_copia_e_cola);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API unavailable/blocked. The code stays visible for a manual copy.
    }
  }

  return <Dialog open={open} onOpenChange={next => {
    if (!next) onClose();
  }}>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Pague com Pix para concluir</DialogTitle>
        <DialogDescription>Escaneie o QR code ou copie o código Pix no aplicativo do seu banco.</DialogDescription>
      </DialogHeader>
      {purchase?.status === 'confirmed'
        ? <p className="store-reward-result">Pagamento confirmado! Seus créditos já foram adicionados.</p>
        : purchase?.status && purchase.status !== 'pending'
          ? <p className="buyin-error" role="alert">Esta compra não está mais disponível ({STATUS_LABEL[purchase.status] || purchase.status}).</p>
          : <>
            {purchase?.qr_code_base64 && <div className="store-qr">
              <Image src={`data:image/png;base64,${purchase.qr_code_base64}`} alt="QR code Pix para pagamento"
                     width={200} height={200} unoptimized/>
            </div>}
            <div className="buyin-control">
              <label htmlFor="pix-copia-e-cola">Pix copia e cola</label>
              <output id="pix-copia-e-cola" className="store-pix-code">{purchase?.pix_copia_e_cola}</output>
            </div>
            <Button type="button" onClick={() => void copy()}>
              {copied ? <><Check/> Copiado</> : <><Copy/> Copiar código</>}
            </Button>
            {expiresMs !== null && <p className={`store-countdown${expired ? ' is-expiring' : ''}`}>
              {expired ? 'Código expirado' : `Expira em ${formatDuration(remainingMs)}`}
            </p>}
          </>}
    </DialogContent>
  </Dialog>;
}

const STATUS_LABEL: Record<string, string> = {
  refunded: 'estornada',
  expired: 'expirada',
  failed: 'falhou',
};
