'use client';
import {ArrowRight, Gift, Sparkles} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {SkeletonList} from '@/components/ui/skeleton';
import {WELCOME_PACK_SKU, type SandboxSKU} from '@/lib/api/wallet';
import {moneyExact} from '@/lib/chips';


export function SkuGrid({skus, isLoading, isError, onRetryAction, onSelectAction, pendingSku, welcomePackClaimed}: {
  skus: SandboxSKU[];
  isLoading: boolean;
  isError: boolean;
  onRetryAction: () => void;
  onSelectAction: (sku: SandboxSKU, trigger: HTMLButtonElement) => void;
  pendingSku: string | null;
  // Best-effort: derived from the purchase history already loaded for this
  // page (#348 has no dedicated eligibility field). A false negative here
  // just shows the card as available; the purchase itself is the real guard
  // and rejects with a clear message if it was already claimed.
  welcomePackClaimed?: boolean;
}) {
  if (isLoading) {
    return <SkeletonList label="Carregando pacotes de créditos…" count={4} height={140} className="store-sku-grid"/>;
  }
  if (isError) {
    return <div className="lobby-empty">Não foi possível carregar os pacotes agora.
      <Button variant="outline" size="sm" onClick={onRetryAction}>Tentar novamente</Button>
    </div>;
  }
  if (skus.length === 0) {
    return <div className="lobby-empty">
      <Sparkles aria-hidden="true"/>
      <p>Nenhum pacote disponível no momento.</p>
    </div>;
  }

  const sortedSkus = [...skus].sort((left, right) =>
    left.price_cents - right.price_cents
    || left.total_credits - right.total_credits
    || left.id.localeCompare(right.id));

  return <div className="store-sku-grid" role="group" aria-label="Pacotes de fichas">
      {sortedSkus.map(sku => {
        const bonusCredits = Math.max(0, sku.total_credits - sku.base_credits);
        const totalLabel = sku.total_credits.toLocaleString('pt-BR');
        const baseLabel = sku.base_credits.toLocaleString('pt-BR');
        const bonusLabel = bonusCredits.toLocaleString('pt-BR');
        const isWelcomePack = sku.id === WELCOME_PACK_SKU;
        const claimed = isWelcomePack && welcomePackClaimed;
        return <button key={sku.id} type="button"
                       className={`store-sku-card${isWelcomePack ? ' store-sku-card-welcome' : ''}`}
                       aria-label={`${claimed ? 'Já resgatado: ' : 'Escolher '}${isWelcomePack ? 'pacote de boas-vindas, ' : ''}${totalLabel} fichas: ${baseLabel} base${bonusCredits > 0 ? ` mais ${bonusLabel} de bônus` : ', sem bônus'}, por ${moneyExact(sku.price_cents)}`}
                       disabled={pendingSku !== null || claimed} onClick={event => onSelectAction(sku, event.currentTarget)}>
          {isWelcomePack && <span className="store-sku-badge"><Gift aria-hidden="true"/> Pacote de boas-vindas</span>}
          <span className="store-sku-credits">{totalLabel} <small>fichas no total</small></span>
          <span className="store-sku-composition">
            <span>{baseLabel} base</span>
            {bonusCredits > 0
              ? <><span aria-hidden="true">+</span><strong>{bonusLabel} bônus</strong>
                <small>({sku.bonus_percent}%)</small></>
              : <small>sem bônus</small>}
          </span>
          <span className="store-sku-price">
            <span>{claimed ? 'Já resgatado' : pendingSku === sku.id ? 'Preparando Pix…' : moneyExact(sku.price_cents)}</span>
            {!claimed && <ArrowRight aria-hidden="true"/>}
          </span>
        </button>;
      })}
    </div>;
}
