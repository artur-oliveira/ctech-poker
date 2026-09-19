'use client';
import {useState} from 'react';
import {Ticket} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Field} from '@/components/ui/field';
import {Input} from '@/components/ui/input';
import {createPurchase, promoRedeemErrorMessage, type SandboxPurchase} from '@/lib/api/wallet';

/**
 * Standalone promo-code redemption (#348) — deliberately separate from the
 * SKU grid, since a code resolves its own package server-side: the client
 * only ever sends the code, never a chosen sku (see wallet.ts's
 * createPurchase). Success opens the same PurchaseModal a catalog pick does.
 */
export function PromoCodeForm({onRedeemedAction}: {onRedeemedAction: (purchase: SandboxPurchase) => void}) {
  const [code, setCode] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [redeemed, setRedeemed] = useState(false);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const trimmed = code.trim();
    if (!trimmed || pending) return;
    setPending(true);
    setError(null);
    try {
      const purchase = await createPurchase('', trimmed);
      setRedeemed(true);
      setCode('');
      onRedeemedAction(purchase);
    } catch (err) {
      setError(promoRedeemErrorMessage(err));
    } finally {
      setPending(false);
    }
  }

  return <form className="store-promo-form" onSubmit={event => void submit(event)}>
    <Field label="Código promocional" description="Opcional. O pacote e o preço são definidos pelo código.">
      {control => <div className="store-promo-row">
        <Input {...control} value={code} placeholder="Ex.: BEMVINDO10"
               autoComplete="off" autoCapitalize="characters"
               onChange={event => {
                 setCode(event.target.value.toUpperCase());
                 setRedeemed(false);
               }}/>
        <Button type="submit" variant="outline" disabled={!code.trim() || pending} loading={pending}>
          <Ticket aria-hidden="true"/> {pending ? 'Aplicando…' : 'Aplicar código'}
        </Button>
      </div>}
    </Field>
    {error && <p className="form-error" role="alert">{error}</p>}
    {redeemed && !error && <p className="store-promo-success" role="status">Código aplicado! Conclua o pagamento para receber o pacote.</p>}
  </form>;
}
