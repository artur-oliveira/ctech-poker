'use client';
import {useState} from 'react';
import {ArrowRight, ChevronDown, ChevronUp, Sparkles} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {SkeletonList} from '@/components/ui/skeleton';
import type {SandboxSKU} from '@/lib/api/wallet';

function formatBRL(cents: number) {
  return (cents / 100).toLocaleString('pt-BR', {style: 'currency', currency: 'BRL'});
}

export function SkuGrid({skus, isLoading, isError, onRetryAction, onSelectAction, pendingSku}: {
  skus: SandboxSKU[];
  isLoading: boolean;
  isError: boolean;
  onRetryAction: () => void;
  onSelectAction: (sku: SandboxSKU, trigger: HTMLButtonElement) => void;
  pendingSku: string | null;
}) {
  const [expanded, setExpanded] = useState(false);

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

  const visibleSkus = expanded ? skus : skus.slice(0, 4);
  const hiddenCount = Math.max(0, skus.length - 4);

  return <>
    <div className="store-sku-grid" aria-label="Pacotes de fichas sandbox">
      {visibleSkus.map(sku => {
        const bonusCredits = Math.max(0, sku.total_credits - sku.base_credits);
        const totalLabel = sku.total_credits.toLocaleString('pt-BR');
        const baseLabel = sku.base_credits.toLocaleString('pt-BR');
        const bonusLabel = bonusCredits.toLocaleString('pt-BR');
        return <button key={sku.id} type="button" className="store-sku-card"
                       aria-label={`Escolher ${totalLabel} fichas: ${baseLabel} base${bonusCredits > 0 ? ` mais ${bonusLabel} de bônus` : ', sem bônus'}, por ${formatBRL(sku.price_cents)}`}
                       disabled={pendingSku !== null} onClick={event => onSelectAction(sku, event.currentTarget)}>
          <span className="store-sku-credits">{totalLabel} <small>fichas no total</small></span>
          <span className="store-sku-composition">
            <span>{baseLabel} base</span>
            {bonusCredits > 0
              ? <><span aria-hidden="true">+</span><strong>{bonusLabel} bônus</strong>
                <small>({sku.bonus_percent}%)</small></>
              : <small>sem bônus</small>}
          </span>
          <span className="store-sku-price">
            <span>{pendingSku === sku.id ? 'Preparando Pix…' : formatBRL(sku.price_cents)}</span>
            <ArrowRight aria-hidden="true"/>
          </span>
        </button>;
      })}
    </div>
    {hiddenCount > 0 && <Button className="store-sku-toggle" variant="ghost" size="sm"
                                aria-expanded={expanded} onClick={() => setExpanded(value => !value)}>
      {expanded ? <><ChevronUp aria-hidden="true"/>Mostrar 4 opções</>
        : <><ChevronDown aria-hidden="true"/>Ver mais {hiddenCount} {hiddenCount === 1 ? 'pacote' : 'pacotes'}</>}
    </Button>}
  </>;
}
