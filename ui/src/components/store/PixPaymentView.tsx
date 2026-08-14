'use client';
import {useState} from 'react';
import Image from 'next/image';
import {Check, Copy, ShieldCheck} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {formatDuration, useCountdownMs} from './useCountdown';

interface PixPayable {
  pix_copia_e_cola?: string;
  qr_code_base64?: string;
  expires_at?: string;
}

export function PixPaymentView({purchase, paymentNote = 'As fichas são apenas do modo sandbox e não têm valor em dinheiro.'}: {
  purchase: PixPayable;
  paymentNote?: string;
}) {
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const expiresMs = purchase.expires_at ? new Date(purchase.expires_at).getTime() : null;
  const qrImageType = purchase.qr_code_base64?.startsWith('PHN2Zy') ? 'image/svg+xml' : 'image/png';
  const remainingMs = useCountdownMs(expiresMs);
  const expired = expiresMs !== null && remainingMs <= 0;

  async function copy() {
    if (!purchase.pix_copia_e_cola) return;
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(purchase.pix_copia_e_cola);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  }

  return <>
    {purchase.qr_code_base64 && <div className="store-qr">
      <Image src={`data:${qrImageType};base64,${purchase.qr_code_base64}`} alt="QR code Pix para pagamento"
             width={200} height={200} unoptimized/>
    </div>}
    <div className="buyin-control store-pix-control">
      <label htmlFor="pix-copia-e-cola">Pix copia e cola</label>
      <div className="store-pix-field">
        <input id="pix-copia-e-cola" value={purchase.pix_copia_e_cola ?? ''} readOnly
               onClick={event => event.currentTarget.select()}/>
        <Button type="button" variant="ghost" size="icon"
                aria-label={copied ? 'Código Pix copiado' : expired ? 'Código Pix expirado' : 'Copiar código Pix'}
                title={copied ? 'Copiado' : expired ? 'Código expirado' : 'Copiar código Pix'}
                disabled={expired || !purchase.pix_copia_e_cola} onClick={() => void copy()}>
          {copied ? <Check aria-hidden="true"/> : <Copy aria-hidden="true"/>}
        </Button>
      </div>
      <span className="sr-only" aria-live="polite">{copied ? 'Código Pix copiado.' : ''}</span>
    </div>
    {copyFailed && <p className="buyin-error" role="alert">Não foi possível copiar automaticamente. Selecione o código acima e copie manualmente.</p>}
    {expiresMs !== null && <p className={`store-countdown${expired ? ' is-expiring' : ''}`}>
      {expired ? 'Código expirado' : `Expira em ${formatDuration(remainingMs)}`}
    </p>}
    <p className="store-payment-note"><ShieldCheck aria-hidden="true"/> {paymentNote}</p>
  </>;
}
